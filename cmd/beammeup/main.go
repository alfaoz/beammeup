package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alfaoz/beammeup/internal/cli"
	"github.com/alfaoz/beammeup/internal/hangar"
	"github.com/alfaoz/beammeup/internal/session"
	"github.com/alfaoz/beammeup/internal/ships"
	"github.com/alfaoz/beammeup/internal/sshx"
	"github.com/alfaoz/beammeup/internal/tui"
	"github.com/alfaoz/beammeup/internal/update"
	"github.com/alfaoz/beammeup/internal/version"
	"golang.org/x/term"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, err := cli.Parse(args)
	if err != nil {
		printErr(err)
		cli.PrintHelp()
		return cli.ExitUsage
	}

	if opts.Help {
		cli.PrintHelp()
		return cli.ExitSuccess
	}

	if opts.VersionOnly {
		fmt.Printf("beammeup v%s\n", version.AppVersion)
		return cli.ExitSuccess
	}

	store, err := ships.NewStore(strings.TrimSpace(os.Getenv("BEAMMEUP_SHIPS_DIR")))
	if err != nil {
		printErr(fmt.Errorf("initialize ships store: %w", err))
		return cli.ExitFailure
	}

	hangarSvc := hangar.NewService()
	sshOpts := sshx.DefaultConnectOptions()
	if strings.TrimSpace(opts.SSHKnownHosts) != "" {
		sshOpts.KnownHostsPath = strings.TrimSpace(opts.SSHKnownHosts)
	}
	if opts.StrictHostKey {
		sshOpts.HostKeyMode = sshx.HostKeyStrict
	}
	if opts.InsecureHostKey {
		sshOpts.HostKeyMode = sshx.HostKeyInsecureIgnore
	}
	hangarSvc.SSH = sshOpts

	if opts.SelfUpdate {
		result, err := runSelfUpdate(opts.BaseURL)
		if err != nil {
			printErr(err)
			return cli.ExitFailure
		}
		printUpdateMessage(result)
		return cli.ExitSuccess
	}

	if shouldAutoUpdate(opts) {
		result, err := runSelfUpdate(opts.BaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[beammeup] auto-update skipped: %v\n", err)
		} else if result.Updated {
			printUpdateMessage(result)
		}
	}

	isTTY := isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout)
	if cli.RequiresNonInteractive(opts, isTTY) {
		runner := &cli.Runner{Store: store, Hangar: hangarSvc}
		code, err := runner.Run(opts)
		if err != nil {
			printErr(err)
		}
		return code
	}

	app := tui.New(store, hangarSvc, session.NewPasswordCache())
	if err := app.Run(); err != nil {
		if errors.Is(err, os.ErrClosed) {
			return cli.ExitSuccess
		}
		printErr(err)
		return cli.ExitFailure
	}
	return cli.ExitSuccess
}

func shouldAutoUpdate(opts cli.Options) bool {
	if opts.AutoUpdate {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BEAMMEUP_AUTO_UPDATE")))
	return v == "1" || v == "true" || v == "yes"
}

func runSelfUpdate(baseURL string) (update.Result, error) {
	return update.SelfUpdate(strings.TrimSpace(baseURL))
}

func printUpdateMessage(res update.Result) {
	v := strings.TrimPrefix(strings.TrimSpace(res.Version), "v")
	if v == "" {
		v = version.AppVersion
	}
	if res.Updated {
		fmt.Printf("[beammeup] updated to v%s\n", v)
		return
	}
	fmt.Printf("[beammeup] already on beammeup v%s\n", v)
}

func printErr(err error) {
	fmt.Fprintf(os.Stderr, "[beammeup] ERROR: %v\n", err)
}

func isTerminalFile(f *os.File) bool {
	fd := f.Fd()
	// Guard against uintptr->int overflow (paranoia, but keeps scanners quiet).
	if fd > uintptr(^uint(0)>>1) {
		return false
	}
	return term.IsTerminal(int(fd))
}
