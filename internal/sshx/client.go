package sshx

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Target struct {
	Host     string
	Port     int
	User     string
	Password string
}

type HostKeyMode int

const (
	// HostKeyAcceptNew trusts the first-seen host key (TOFU) and records it in
	// the known_hosts file. Subsequent changes are treated as errors.
	HostKeyAcceptNew HostKeyMode = iota
	// HostKeyStrict requires the host key to already exist in the known_hosts
	// file.
	HostKeyStrict
	// HostKeyInsecureIgnore disables host key verification (unsafe).
	HostKeyInsecureIgnore
)

type ConnectOptions struct {
	KnownHostsPath string
	HostKeyMode    HostKeyMode
}

type Client struct {
	sshClient *ssh.Client
}

func DefaultConnectOptions() ConnectOptions {
	mode := HostKeyAcceptNew
	if envTrue("BEAMMEUP_STRICT_HOST_KEY") {
		mode = HostKeyStrict
	}
	if envTrue("BEAMMEUP_INSECURE_IGNORE_HOST_KEY") {
		mode = HostKeyInsecureIgnore
	}

	if v := strings.TrimSpace(os.Getenv("BEAMMEUP_SSH_KNOWN_HOSTS")); v != "" {
		return ConnectOptions{KnownHostsPath: v, HostKeyMode: mode}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// If we can't resolve home dir, leave KnownHostsPath empty and let
		// ConnectWithOptions return an explicit error.
		return ConnectOptions{HostKeyMode: mode}
	}
	return ConnectOptions{
		KnownHostsPath: filepath.Join(home, ".beammeup", "known_hosts"),
		HostKeyMode:    mode,
	}
}

func envTrue(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "y" || v == "on"
}

func Connect(t Target) (*Client, error) {
	return ConnectWithOptions(t, DefaultConnectOptions())
}

func ConnectWithOptions(t Target, opts ConnectOptions) (*Client, error) {
	if t.Port == 0 {
		t.Port = 22
	}

	addr := net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))

	cfg := &ssh.ClientConfig{
		User:    t.User,
		Auth:    []ssh.AuthMethod{ssh.Password(t.Password)},
		Timeout: 20 * time.Second,
	}

	switch opts.HostKeyMode {
	case HostKeyInsecureIgnore:
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	default:
		khPath := strings.TrimSpace(opts.KnownHostsPath)
		if khPath == "" {
			return nil, errors.New("ssh known_hosts path not set")
		}
		if err := ensureKnownHostsFile(khPath); err != nil {
			return nil, fmt.Errorf("prepare known_hosts: %w", err)
		}
		kh, err := knownhosts.New(khPath)
		if err != nil {
			return nil, fmt.Errorf("load known_hosts: %w", err)
		}

		acceptNew := opts.HostKeyMode == HostKeyAcceptNew
		cfg.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if err := kh(hostname, remote, key); err == nil {
				return nil
			} else if ke, ok := err.(*knownhosts.KeyError); ok {
				fp := ssh.FingerprintSHA256(key)
				if len(ke.Want) == 0 {
					if !acceptNew {
						return &HostKeyError{Addr: hostname, Fingerprint: fp, KnownHostsPath: khPath, Reason: "unknown"}
					}
					if err := appendKnownHost(khPath, hostname, key); err != nil {
						return fmt.Errorf("trust new host key: %w", err)
					}
					return nil
				}
				return &HostKeyError{Addr: hostname, Fingerprint: fp, KnownHostsPath: khPath, Reason: "mismatch"}
			}

			// For revoked keys or other knownhosts parser errors, keep the original
			// message.
			return err
		}
	}

	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{sshClient: c}, nil
}

type HostKeyError struct {
	Addr           string
	Fingerprint    string
	KnownHostsPath string
	Reason         string // unknown|mismatch
}

func (e *HostKeyError) Error() string {
	switch e.Reason {
	case "unknown":
		return fmt.Sprintf("unknown SSH host key for %s (fingerprint %s). To trust it, add it to %s or enable TOFU mode", e.Addr, e.Fingerprint, e.KnownHostsPath)
	case "mismatch":
		return fmt.Sprintf("SSH host key mismatch for %s (fingerprint %s). This may indicate a MITM attack or a rebuilt server. Update %s (or use insecure mode to bypass verification)", e.Addr, e.Fingerprint, e.KnownHostsPath)
	default:
		return fmt.Sprintf("SSH host key error for %s (fingerprint %s)", e.Addr, e.Fingerprint)
	}
}

func ensureKnownHostsFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Chmod(path, 0o600)
	return nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	line := knownhosts.Line([]string{hostname}, key) + "\n"

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(line)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.sshClient == nil {
		return nil
	}
	return c.sshClient.Close()
}

func (c *Client) RunCombined(command string) (string, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(command)
	return string(out), err
}

func (c *Client) Dial(network, addr string) (net.Conn, error) {
	if c == nil || c.sshClient == nil {
		return nil, errors.New("ssh client not connected")
	}
	return c.sshClient.Dial(network, addr)
}

func (c *Client) Upload(content []byte, remotePath string, mode os.FileMode) error {
	sftpClient, err := sftp.NewClient(c.sshClient)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	f, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
