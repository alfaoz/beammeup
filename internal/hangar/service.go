package hangar

import (
	"fmt"
	"strings"
	"time"

	"github.com/alfaoz/beammeup/internal/remote"
	"github.com/alfaoz/beammeup/internal/ships"
	"github.com/alfaoz/beammeup/internal/sshx"
)

type Status string

const (
	StatusOnline  Status = "online"
	StatusMissing Status = "missing"
	StatusDrift   Status = "drift"
)

type ProtocolState struct {
	Exists  bool
	Active  bool
	Port    string
	User    string
	Pass    string
	Mode    string
	Managed bool
	Legacy  bool
}

type Inventory struct {
	PublicIP       string
	Socks5         ProtocolState
	HTTP           ProtocolState
	HangarStatus   Status
	MetadataExists bool
}

type ActionInput struct {
	Mode              string // inventory|show|preflight|apply|destroy
	Protocol          string // http|socks5
	HTTPMode          string // auto|sidecar
	ProxyPort         int
	NoFirewallChange  bool
	RotateCredentials bool
}

type ActionResult struct {
	Protocol     string
	HTTPMode     string
	Host         string
	Port         string
	User         string
	Pass         string
	Action       string
	FirewallNote string
	Note         string
	RawOutput    string
	Inventory    Inventory
	Values       remote.KeyValues
}

type Service struct {
	runRemoteFn func(target sshx.Target, in ActionInput) (remote.KeyValues, string, error)
}

func NewService() *Service { return &Service{} }

func (s *Service) runRemote(target sshx.Target, in ActionInput) (remote.KeyValues, string, error) {
	if s.runRemoteFn != nil {
		return s.runRemoteFn(target, in)
	}

	client, err := sshx.Connect(target)
	if err != nil {
		return nil, "", fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	remotePath := fmt.Sprintf("/tmp/beammeup-v2-%d.sh", time.Now().UnixNano())
	if err := client.Upload([]byte(remote.Script), remotePath, 0o700); err != nil {
		return nil, "", fmt.Errorf("upload remote script: %w", err)
	}
	defer client.RunCombined("rm -f " + remotePath)

	args := []string{"--mode", in.Mode}
	if strings.TrimSpace(in.Protocol) != "" {
		args = append(args, "--protocol", in.Protocol)
	}
	if strings.TrimSpace(in.HTTPMode) != "" {
		args = append(args, "--http-mode", in.HTTPMode)
	}
	if in.ProxyPort > 0 {
		args = append(args, "--proxy-port", fmt.Sprintf("%d", in.ProxyPort))
	}
	if in.NoFirewallChange {
		args = append(args, "--no-firewall-change")
	}
	if in.RotateCredentials {
		args = append(args, "--rotate-credentials")
	}

	cmd := "bash " + remotePath + " " + shellJoin(args)
	out, err := client.RunCombined(cmd)
	kv := remote.ParseBM(out)
	if err != nil {
		if len(kv) == 0 {
			return nil, out, fmt.Errorf("remote command failed: %w\n%s", err, out)
		}
	}
	return kv, out, nil
}

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		q := strings.ReplaceAll(p, "'", "'\"'\"'")
		quoted = append(quoted, "'"+q+"'")
	}
	return strings.Join(quoted, " ")
}

func parseInventory(kv remote.KeyValues) Inventory {
	status := Status(strings.TrimSpace(kv.Get("BM_HANGAR_STATUS")))
	if status == "" {
		any := kv.Bool("BM_SOCKS_EXISTS") || kv.Bool("BM_HTTP_EXISTS")
		if any {
			status = StatusOnline
		} else {
			status = StatusMissing
		}
	}
	return Inventory{
		PublicIP: kv.Get("BM_PUBLIC_IP"),
		Socks5: ProtocolState{
			Exists: kv.Bool("BM_SOCKS_EXISTS"),
			Active: kv.Bool("BM_SOCKS_ACTIVE"),
			Port:   kv.Get("BM_SOCKS_PORT"),
			User:   kv.Get("BM_SOCKS_USER"),
			Pass:   kv.Get("BM_SOCKS_PASS"),
			Mode:   kv.Get("BM_SOCKS_MODE"),
		},
		HTTP: ProtocolState{
			Exists:  kv.Bool("BM_HTTP_EXISTS"),
			Active:  kv.Bool("BM_HTTP_ACTIVE"),
			Port:    kv.Get("BM_HTTP_PORT"),
			User:    kv.Get("BM_HTTP_USER"),
			Pass:    kv.Get("BM_HTTP_PASS"),
			Mode:    kv.Get("BM_HTTP_MODE"),
			Managed: kv.Bool("BM_HTTP_MANAGED"),
			Legacy:  kv.Bool("BM_HTTP_LEGACY"),
		},
		HangarStatus:   status,
		MetadataExists: kv.Bool("BM_METADATA_EXISTS"),
	}
}

func (s *Service) Inventory(ship ships.Ship, password string) (Inventory, error) {
	target := sshx.Target{Host: ship.Host, Port: ship.SSHPort, User: ship.SSHUser, Password: password}
	kv, out, err := s.runRemote(target, ActionInput{Mode: "inventory"})
	if err != nil {
		return Inventory{}, fmt.Errorf("inventory failed: %w", err)
	}
	if len(kv) == 0 {
		return Inventory{}, fmt.Errorf("inventory returned no BM output\n%s", out)
	}
	return parseInventory(kv), nil
}

func (s *Service) Execute(ship ships.Ship, password string, in ActionInput) (ActionResult, error) {
	target := sshx.Target{Host: ship.Host, Port: ship.SSHPort, User: ship.SSHUser, Password: password}
	kv, out, err := s.runRemote(target, in)
	if err != nil {
		return ActionResult{}, err
	}

	res := ActionResult{
		Protocol:     kv.Get("BM_RESULT_PROTOCOL"),
		HTTPMode:     kv.Get("BM_RESULT_HTTP_MODE"),
		Host:         kv.Get("BM_RESULT_HOST"),
		Port:         kv.Get("BM_RESULT_PORT"),
		User:         kv.Get("BM_RESULT_USER"),
		Pass:         kv.Get("BM_RESULT_PASS"),
		Action:       kv.Get("BM_RESULT_ACTION"),
		FirewallNote: kv.Get("BM_RESULT_FIREWALL_NOTE"),
		Note:         kv.Get("BM_RESULT_NOTE"),
		RawOutput:    out,
		Values:       kv,
	}
	if res.Host == "" || res.Host == "UNKNOWN" {
		res.Host = ship.Host
	}
	if len(kv) > 0 && kv.Get("BM_SOCKS_EXISTS") != "" {
		res.Inventory = parseInventory(kv)
	}
	return res, nil
}
