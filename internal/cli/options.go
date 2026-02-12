package cli

import (
	"fmt"

	"github.com/spf13/pflag"
)

type Options struct {
	Host             string
	ShipName         string
	ListShips        bool
	SSHPort          int
	SSHUser          string
	SSHPassword      string
	Protocol         string
	ProxyPort        int
	Action           string
	ShowInventory    bool
	PreflightOnly    bool
	NoFirewallChange bool
	SelfUpdate       bool
	AutoUpdate       bool
	BaseURL          string
	VersionOnly      bool
	Yes              bool
	Help             bool
	RawArgs          []string
}

func DefaultOptions() Options {
	return Options{
		SSHPort:  22,
		SSHUser:  "root",
		BaseURL:  "https://beammeup.pw",
		Protocol: "",
		Action:   "",
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
	fs.StringVar(&opts.Protocol, "protocol", "", "http or socks5")
	fs.IntVar(&opts.ProxyPort, "proxy-port", 0, "Proxy port")
	fs.StringVar(&opts.Action, "action", "", "show|configure|rotate|destroy")
	fs.BoolVar(&opts.ShowInventory, "show-inventory", false, "Show inventory")
	fs.BoolVar(&opts.PreflightOnly, "preflight-only", false, "Preflight only")
	fs.BoolVar(&opts.NoFirewallChange, "no-firewall-change", false, "Skip firewall changes")
	fs.BoolVar(&opts.SelfUpdate, "self-update", false, "Self update")
	fs.BoolVar(&opts.AutoUpdate, "auto-update", false, "Auto update")
	fs.StringVar(&opts.BaseURL, "base-url", opts.BaseURL, "Release base URL")
	fs.BoolVar(&opts.VersionOnly, "version", false, "Print version")
	fs.BoolVar(&opts.Yes, "yes", false, "Skip confirmations")
	fs.BoolVarP(&opts.Help, "help", "h", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return opts, err
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
