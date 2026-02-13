package cli

import (
	"fmt"

	"github.com/spf13/pflag"
)

type Options struct {
	Host                    string
	ShipName                string
	ListShips               bool
	SSHPort                 int
	SSHUser                 string
	SSHPassword             string
	SSHKnownHosts           string
	StrictHostKey           bool
	InsecureHostKey         bool
	Protocol                string
	HTTPMode                string
	ProxyPort               int
	Action                  string
	ShowInventory           bool
	PreflightOnly           bool
	NoFirewallChange        bool
	ListenLocal             bool
	SmartBlinder            bool
	SmartBlinderIdleMinutes int
	Stealth                 bool
	SelfUpdate              bool
	AutoUpdate              bool
	BaseURL                 string
	VersionOnly             bool
	Yes                     bool
	Help                    bool
	RawArgs                 []string

	// Tracks whether flags were explicitly provided so we don't override ship defaults.
	ListenLocalSet         bool
	SmartBlinderSet        bool
	SmartBlinderIdleMinSet bool
}

func DefaultOptions() Options {
	return Options{
		SSHPort:                 22,
		SSHUser:                 "root",
		BaseURL:                 "https://beammeup.pw",
		Protocol:                "",
		Action:                  "",
		SmartBlinder:            true,
		SmartBlinderIdleMinutes: 10,
	}
}

func Parse(args []string) (Options, error) {
	opts := DefaultOptions()
	fs := pflag.NewFlagSet("beammeup", pflag.ContinueOnError)
	fs.SetInterspersed(false)

	fs.StringVar(&opts.Host, "host", opts.Host, "VPS host or IP")
	fs.StringVar(&opts.ShipName, "ship", opts.ShipName, "Use saved ship profile")
	fs.BoolVar(&opts.ListShips, "list-ships", false, "List saved ships")
	fs.IntVar(&opts.SSHPort, "ssh-port", opts.SSHPort, "SSH port")
	fs.StringVar(&opts.SSHUser, "ssh-user", opts.SSHUser, "SSH user")
	fs.StringVar(&opts.SSHPassword, "ssh-password", "", "SSH password")
	fs.StringVar(&opts.SSHKnownHosts, "ssh-known-hosts", "", "SSH known_hosts file path")
	fs.BoolVar(&opts.StrictHostKey, "strict-host-key", false, "Require known SSH host key (no TOFU)")
	fs.BoolVar(&opts.InsecureHostKey, "insecure-ignore-host-key", false, "Disable SSH host key verification (UNSAFE)")
	fs.StringVar(&opts.Protocol, "protocol", "", "http or socks5")
	fs.StringVar(&opts.HTTPMode, "http-mode", "", "auto or sidecar")
	fs.IntVar(&opts.ProxyPort, "proxy-port", 0, "Proxy port")
	fs.StringVar(&opts.Action, "action", "", "show|configure|rotate|destroy")
	fs.BoolVar(&opts.ShowInventory, "show-inventory", false, "Show inventory")
	fs.BoolVar(&opts.PreflightOnly, "preflight-only", false, "Preflight only")
	fs.BoolVar(&opts.NoFirewallChange, "no-firewall-change", false, "Skip firewall changes")
	fs.BoolVar(&opts.Stealth, "stealth", false, "Stealth mode: local SOCKS5 proxy via SSH tunnel, zero VPS footprint")
	fs.BoolVar(&opts.ListenLocal, "listen-local", opts.ListenLocal, "Bind proxy to localhost on VPS (requires SSH tunnel)")
	fs.BoolVar(&opts.SmartBlinder, "smart-blinder", opts.SmartBlinder, "Smart blinder: stop proxy after idle (recommended)")
	fs.IntVar(&opts.SmartBlinderIdleMinutes, "smart-blinder-idle-minutes", opts.SmartBlinderIdleMinutes, "Smart blinder idle minutes (default: 10)")
	fs.BoolVar(&opts.SelfUpdate, "self-update", false, "Self update")
	fs.BoolVar(&opts.AutoUpdate, "auto-update", false, "Auto update")
	fs.StringVar(&opts.BaseURL, "base-url", opts.BaseURL, "Release base URL")
	fs.BoolVar(&opts.VersionOnly, "version", false, "Print version")
	fs.BoolVar(&opts.Yes, "yes", false, "Skip confirmations")
	fs.BoolVarP(&opts.Help, "help", "h", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.StrictHostKey && opts.InsecureHostKey {
		return opts, fmt.Errorf("use either --strict-host-key or --insecure-ignore-host-key, not both")
	}
	opts.ListenLocalSet = fs.Changed("listen-local")
	opts.SmartBlinderSet = fs.Changed("smart-blinder")
	opts.SmartBlinderIdleMinSet = fs.Changed("smart-blinder-idle-minutes")
	if opts.SmartBlinder && opts.SmartBlinderIdleMinutes <= 0 {
		return opts, fmt.Errorf("--smart-blinder-idle-minutes must be > 0")
	}
	opts.RawArgs = fs.Args()
	if len(opts.RawArgs) > 0 {
		return opts, fmt.Errorf("unknown arguments: %v", opts.RawArgs)
	}

	return opts, nil
}

func NormalizeProtocol(v string) (string, bool) {
	switch v {
	case "", "http", "socks5", "socks":
		if v == "socks" {
			return "socks5", true
		}
		return v, true
	default:
		return "", false
	}
}

func NormalizeAction(v string) (string, bool) {
	switch v {
	case "", "show", "configure", "rotate", "destroy", "install", "uninstall":
		if v == "install" {
			return "configure", true
		}
		if v == "uninstall" {
			return "destroy", true
		}
		return v, true
	default:
		return "", false
	}
}

func NormalizeHTTPMode(v string) (string, bool) {
	switch v {
	case "", "auto", "sidecar":
		if v == "auto" {
			return "", true
		}
		return v, true
	default:
		return "", false
	}
}
