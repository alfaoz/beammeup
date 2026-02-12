package ships

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const DefaultDirSuffix = ".beammeup/ships"

type Ship struct {
	Name             string
	Host             string
	SSHPort          int
	SSHUser          string
	Protocol         string
	ProxyPort        int
	NoFirewallChange bool
}

type Store struct {
	Dir string
}

func NewStore(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, DefaultDirSuffix)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure ships dir: %w", err)
	}
	return &Store{Dir: dir}, nil
}

func SanitizeName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, " ", "-")
	b := strings.Builder{}
	lastDash := false
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if ok {
			if r == '-' {
				if lastDash {
					continue
				}
				lastDash = true
			} else {
				lastDash = false
			}
			b.WriteRune(r)
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("read ships dir: %w", err)
	}
	var ships []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".ship") {
			ships = append(ships, strings.TrimSuffix(name, ".ship"))
		}
	}
	sort.Strings(ships)
	return ships, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.Dir, name+".ship")
}

func (s *Store) Load(name string) (Ship, error) {
	name = SanitizeName(name)
	if name == "" {
		return Ship{}, errors.New("invalid ship name")
	}
	f, err := os.Open(s.path(name))
	if err != nil {
		return Ship{}, fmt.Errorf("open ship file: %w", err)
	}
	defer f.Close()

	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		vals[parts[0]] = parts[1]
	}
	if err := scanner.Err(); err != nil {
		return Ship{}, fmt.Errorf("scan ship file: %w", err)
	}

	sshPort := parseIntDefault(vals["SSH_PORT"], 22)
	proxyPort := parseIntDefault(vals["PROXY_PORT"], 18181)
	protocol := vals["PROTOCOL"]
	if protocol == "" {
		protocol = "http"
	}
	noFW := vals["NO_FIREWALL_CHANGE"] == "1" || strings.EqualFold(vals["NO_FIREWALL_CHANGE"], "true")

	ship := Ship{
		Name:             name,
		Host:             vals["HOST"],
		SSHPort:          sshPort,
		SSHUser:          defaultIfEmpty(vals["SSH_USER"], "root"),
		Protocol:         protocol,
		ProxyPort:        proxyPort,
		NoFirewallChange: noFW,
	}
	if strings.TrimSpace(ship.Host) == "" {
		return Ship{}, fmt.Errorf("ship %q missing HOST", name)
	}
	return ship, nil
}

func (s *Store) Save(ship Ship) (Ship, error) {
	ship.Name = SanitizeName(ship.Name)
	if ship.Name == "" {
		return Ship{}, errors.New("ship name is required")
	}
	if strings.TrimSpace(ship.Host) == "" {
		return Ship{}, errors.New("ship host is required")
	}
	if ship.SSHPort == 0 {
		ship.SSHPort = 22
	}
	if strings.TrimSpace(ship.SSHUser) == "" {
		ship.SSHUser = "root"
	}
	if strings.TrimSpace(ship.Protocol) == "" {
		ship.Protocol = "http"
	}
	if ship.ProxyPort == 0 {
		if ship.Protocol == "socks5" {
			ship.ProxyPort = 1080
		} else {
			ship.ProxyPort = 18181
		}
	}

	var noFW string
	if ship.NoFirewallChange {
		noFW = "1"
	} else {
		noFW = "0"
	}

	content := strings.Join([]string{
		"HOST=" + ship.Host,
		"SSH_PORT=" + strconv.Itoa(ship.SSHPort),
		"SSH_USER=" + ship.SSHUser,
		"PROTOCOL=" + ship.Protocol,
		"PROXY_PORT=" + strconv.Itoa(ship.ProxyPort),
		"NO_FIREWALL_CHANGE=" + noFW,
		"",
	}, "\n")

	path := s.path(ship.Name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return Ship{}, fmt.Errorf("write ship file: %w", err)
	}
	return ship, nil
}

func (s *Store) Delete(name string) error {
	name = SanitizeName(name)
	if name == "" {
		return errors.New("invalid ship name")
	}
	if err := os.Remove(s.path(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete ship: %w", err)
	}
	return nil
}

func defaultIfEmpty(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func parseIntDefault(raw string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return def
	}
	return v
}
