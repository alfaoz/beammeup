package tunnel

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/alfaoz/beammeup/internal/sshx"
)

// LogFunc is called for status messages.
type LogFunc func(format string, args ...any)

// Run connects to the target via SSH and starts a local SOCKS5 proxy that
// tunnels all traffic through the SSH connection. It blocks until ctx is
// cancelled or a fatal error occurs.
func Run(ctx context.Context, target sshx.Target, opts sshx.ConnectOptions, localAddr string, logf LogFunc) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	client, err := sshx.ConnectWithOptions(target, opts)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", localAddr, err)
	}
	defer ln.Close()

	logf("stealth tunnel active at %s", ln.Addr())
	logf("all traffic is routed through SSH to %s", target.Host)

	// Close listener when context is cancelled.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				logf("tunnel closed")
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := HandleConn(conn, client.Dial); err != nil {
				logf("conn error: %v", err)
			}
		}()
	}
}
