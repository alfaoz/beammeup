package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alfaoz/beammeup/internal/hangar"
	"github.com/alfaoz/beammeup/internal/ships"
	"golang.org/x/term"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

type Runner struct {
	Store  *ships.Store
	Hangar *hangar.Service
}

func PrintHelp() {
	fmt.Print(`beammeup: manage HTTP/SOCKS5 proxy setups on a VPS via SSH.

Usage:
  beammeup [options]

Options:
  --host <ip-or-hostname>       VPS host or IP
  --ship <name>                 Use saved ship profile from ~/.beammeup/ships
  --list-ships                  List saved ship profiles and exit
  --ssh-port <port>             SSH port (default: 22)
  --ssh-user <username>         SSH user (default: root)
  --ssh-password <password>     SSH password
  --protocol <http|socks5>      Target protocol for show/configure actions
  --proxy-port <port>           Proxy port for configure/preflight
  --action <show|configure|rotate|destroy>
  --show-inventory              List detected beammeup setups and exit
  --preflight-only              Run checks only, make no remote changes
  --no-firewall-change          Do not add firewall rules on the VPS
  --self-update                 Update local beammeup binary and exit
  --auto-update                 Update local beammeup before running requested action
  --base-url <https-url>        Override release base URL
  --version                     Print beammeup version and exit
  --yes                         Skip confirmation prompts
  -h, --help                    Show this help

Environment:
  BEAMMEUP_AUTO_UPDATE=1        Auto-run self-update on startup
  BEAMMEUP_SHIPS_DIR            Override ship profile directory
`)
}

func RequiresNonInteractive(opts Options, isTTY bool) bool {
	if !isTTY {
		return true
	}
	return opts.Host != "" || opts.ShipName != "" || opts.Action != "" || opts.ShowInventory || opts.PreflightOnly ||
		opts.NoFirewallChange || opts.Protocol != "" || opts.ProxyPort > 0 || opts.Yes
}

func (r *Runner) Run(opts Options) (int, error) {
	if opts.ListShips {
		return r.listShips()
	}

	protocol, ok := NormalizeProtocol(strings.ToLower(strings.TrimSpace(opts.Protocol)))
	if !ok {
		return ExitUsage, errors.New("invalid --protocol. use http or socks5")
	}
	action, ok := NormalizeAction(strings.ToLower(strings.TrimSpace(opts.Action)))
	if !ok {
		return ExitUsage, errors.New("invalid --action. use show, configure, rotate, or destroy")
	}

	if opts.PreflightOnly && action != "" {
		return ExitUsage, errors.New("use either --preflight-only or --action, not both")
	}

	var ship ships.Ship
	if opts.ShipName != "" {
		loaded, err := r.Store.Load(opts.ShipName)
		if err != nil {
			return ExitFailure, err
		}
		ship = loaded
	}

	if opts.Host != "" {
		ship.Host = opts.Host
	}
	if opts.SSHPort > 0 {
		ship.SSHPort = opts.SSHPort
	}
	if opts.SSHUser != "" {
		ship.SSHUser = opts.SSHUser
	}
	if protocol != "" {
		ship.Protocol = protocol
	}
	if opts.ProxyPort > 0 {
		ship.ProxyPort = opts.ProxyPort
	}
	if opts.NoFirewallChange {
		ship.NoFirewallChange = true
	}
	if ship.SSHPort == 0 {
		ship.SSHPort = 22
	}
	if ship.SSHUser == "" {
		ship.SSHUser = "root"
	}

	if strings.TrimSpace(ship.Host) == "" {
		return ExitUsage, errors.New("no host provided. use --host or --ship")
	}

	password := opts.SSHPassword
	if strings.TrimSpace(password) == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return ExitUsage, errors.New("ssh password is required")
		}
		fmt.Printf("SSH password for %s@%s: ", ship.SSHUser, ship.Host)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return ExitFailure, fmt.Errorf("read password: %w", err)
		}
		password = string(b)
	}
	if strings.TrimSpace(password) == "" {
		return ExitUsage, errors.New("ssh password is required")
	}

	inv, err := r.Hangar.Inventory(ship, password)
	if err != nil {
		return ExitFailure, err
	}
	printInventorySummary(inv)

	if opts.ShowInventory {
		return ExitSuccess, nil
	}

	if opts.PreflightOnly {
		action = "configure"
	}
	if action == "" {
		action = "configure"
	}

	rotate := false
	if action == "rotate" {
		rotate = true
		action = "configure"
	}

	if action != "destroy" {
		if ship.Protocol == "" {
			if inv.HTTP.Exists {
				ship.Protocol = "http"
			} else if inv.Socks5.Exists {
				ship.Protocol = "socks5"
			} else {
				ship.Protocol = "http"
			}
		}
	}

	in := hangar.ActionInput{}
	switch {
	case action == "show":
		in.Mode = "show"
		in.Protocol = ship.Protocol
	case action == "destroy":
		if !opts.Yes {
			if !confirm("Destroy hangar on "+ship.Host+"?", false) {
				return ExitFailure, errors.New("cancelled")
			}
			fmt.Print("Type DESTROY to confirm: ")
			t := strings.TrimSpace(readLine())
			if t != "DESTROY" {
				return ExitFailure, errors.New("cancelled")
			}
		}
		in.Mode = "destroy"
	case opts.PreflightOnly:
		in.Mode = "preflight"
		in.Protocol = ship.Protocol
		in.ProxyPort = resolveProxyPort(ship, inv)
	default:
		in.Mode = "apply"
		in.Protocol = ship.Protocol
		in.RotateCredentials = rotate
		in.ProxyPort = resolveProxyPort(ship, inv)
		in.NoFirewallChange = ship.NoFirewallChange
	}

	res, err := r.Hangar.Execute(ship, password, in)
	if err != nil {
		return ExitFailure, err
	}

	if in.Mode == "preflight" {
		if res.Values.Get("BM_PREFLIGHT") != "OK" {
			return ExitFailure, errors.New("preflight failed")
		}
		fmt.Println("\nPreflight passed. No changes were made.")
		fmt.Printf("Protocol: %s\n", res.Values.Get("BM_PREFLIGHT_PROTOCOL"))
		fmt.Printf("Port: %s\n", res.Values.Get("BM_PREFLIGHT_PORT"))
		fmt.Println("Status: ready for launch.")
		return ExitSuccess, nil
	}

	if res.Protocol == "DESTROY" {
		fmt.Println("\n[beammeup] destroy hangar complete.")
		fmt.Printf("  Target: %s\n", res.Host)
		if res.Note != "" {
			fmt.Printf("  Result: %s\n", res.Note)
		}
		fmt.Println("\n[beammeup] jump successful.")
		return ExitSuccess, nil
	}

	fmt.Printf("\nbeammeup %s complete (%s).\n", res.Action, res.Protocol)
	fmt.Println("Connection details:")
	fmt.Printf("  Host: %s\n", res.Host)
	fmt.Printf("  Port: %s\n", res.Port)
	fmt.Printf("  Username: %s\n", fallback(res.User, "<not available>"))
	fmt.Printf("  Password: %s\n", fallback(res.Pass, "<not retrievable>"))

	if res.FirewallNote != "" {
		fmt.Printf("\nFirewall note: %s\n", res.FirewallNote)
	}
	if res.Note != "" {
		fmt.Printf("Note: %s\n", res.Note)
	}

	fmt.Println("\n[beammeup] jump successful.")
	fmt.Println("\nChrome extension setup:")
	if strings.EqualFold(res.Protocol, "HTTP") {
		fmt.Printf("  Type: HTTP proxy\n  Server: %s\n  Port: %s\n", res.Host, res.Port)
		fmt.Println("  Enter username/password when prompted")
		if res.Pass != "" {
			fmt.Printf("\nQuick test:\n  curl -x 'http://%s:%s@%s:%s' https://api.ipify.org\n", res.User, res.Pass, res.Host, res.Port)
		}
	} else {
		fmt.Printf("  Type: SOCKS5\n  Server: %s\n  Port: %s\n", res.Host, res.Port)
		fmt.Println("  Username/Password: use values above")
		if res.Pass != "" {
			fmt.Printf("\nQuick test:\n  curl -x 'socks5h://%s:%s@%s:%s' https://api.ipify.org\n", res.User, res.Pass, res.Host, res.Port)
		}
	}

	return ExitSuccess, nil
}

func (r *Runner) listShips() (int, error) {
	shipsList, err := r.Store.List()
	if err != nil {
		return ExitFailure, err
	}
	if len(shipsList) == 0 {
		fmt.Printf("No ships saved yet in %s\n", r.Store.Dir)
		return ExitSuccess, nil
	}
	fmt.Printf("Saved ships (%s):\n", r.Store.Dir)
	for _, ship := range shipsList {
		fmt.Printf("  - %s\n", ship)
	}
	return ExitSuccess, nil
}

func resolveProxyPort(ship ships.Ship, inv hangar.Inventory) int {
	if ship.ProxyPort > 0 {
		return ship.ProxyPort
	}
	if ship.Protocol == "socks5" {
		if inv.Socks5.Port != "" {
			if p, _ := strconv.Atoi(inv.Socks5.Port); p > 0 {
				return p
			}
		}
		return 1080
	}
	if inv.HTTP.Port != "" {
		if p, _ := strconv.Atoi(inv.HTTP.Port); p > 0 {
			return p
		}
	}
	return 18181
}

func printInventorySummary(inv hangar.Inventory) {
	fmt.Println("\n[ship-scan] detected beammeup setups on target:")
	if inv.Socks5.Exists {
		state := "inactive"
		if inv.Socks5.Active {
			state = "active"
		}
		fmt.Printf("  SOCKS5: %s, port=%s, user=%s\n", state, fallback(inv.Socks5.Port, "unknown"), fallback(inv.Socks5.User, "unknown"))
	} else {
		fmt.Println("  SOCKS5: not configured")
	}
	if inv.HTTP.Exists {
		state := "inactive"
		if inv.HTTP.Active {
			state = "active"
		}
		legacy := ""
		if inv.HTTP.Legacy {
			legacy = " (legacy config)"
		}
		fmt.Printf("  HTTP:   %s, port=%s, user=%s%s\n", state, fallback(inv.HTTP.Port, "unknown"), fallback(inv.HTTP.User, "unknown"), legacy)
	} else {
		fmt.Println("  HTTP:   not configured")
	}
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func confirm(prompt string, defYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	if defYes {
		fmt.Printf("%s [Y/n]: ", prompt)
	} else {
		fmt.Printf("%s [y/N]: ", prompt)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defYes
	}
	return line == "y" || line == "yes"
}

func readLine() string {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
