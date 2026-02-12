package hangar

import (
	"errors"
	"testing"

	"github.com/alfaoz/beammeup/internal/remote"
	"github.com/alfaoz/beammeup/internal/ships"
	"github.com/alfaoz/beammeup/internal/sshx"
)

func TestInventoryMapping(t *testing.T) {
	svc := NewService()
	svc.runRemoteFn = func(_ sshx.Target, in ActionInput) (remote.KeyValues, string, error) {
		if in.Mode != "inventory" {
			t.Fatalf("expected inventory mode, got %q", in.Mode)
		}
		return remote.KeyValues{
			"BM_PUBLIC_IP":       "91.98.67.180",
			"BM_SOCKS_EXISTS":    "1",
			"BM_SOCKS_ACTIVE":    "1",
			"BM_SOCKS_PORT":      "18080",
			"BM_SOCKS_USER":      "beamx",
			"BM_SOCKS_PASS":      "passx",
			"BM_HTTP_EXISTS":     "1",
			"BM_HTTP_ACTIVE":     "0",
			"BM_HTTP_PORT":       "18181",
			"BM_HTTP_USER":       "beamhttp",
			"BM_HTTP_PASS":       "passhttp",
			"BM_HANGAR_STATUS":   "drift",
			"BM_METADATA_EXISTS": "1",
		}, "", nil
	}

	inv, err := svc.Inventory(ships.Ship{Host: "x", SSHUser: "root", SSHPort: 22}, "pw")
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}

	if inv.PublicIP != "91.98.67.180" {
		t.Fatalf("unexpected public IP: %q", inv.PublicIP)
	}
	if inv.HangarStatus != StatusDrift {
		t.Fatalf("expected drift, got %q", inv.HangarStatus)
	}
	if !inv.Socks5.Exists || inv.Socks5.Port != "18080" {
		t.Fatalf("unexpected socks inventory: %+v", inv.Socks5)
	}
	if !inv.HTTP.Exists || inv.HTTP.Active {
		t.Fatalf("unexpected http inventory: %+v", inv.HTTP)
	}
}

func TestExecuteMapping(t *testing.T) {
	svc := NewService()
	svc.runRemoteFn = func(_ sshx.Target, in ActionInput) (remote.KeyValues, string, error) {
		if in.Mode != "apply" {
			t.Fatalf("expected apply mode, got %q", in.Mode)
		}
		return remote.KeyValues{
			"BM_RESULT_PROTOCOL":      "HTTP",
			"BM_RESULT_HOST":          "UNKNOWN",
			"BM_RESULT_PORT":          "18181",
			"BM_RESULT_USER":          "beamhttp",
			"BM_RESULT_PASS":          "secret",
			"BM_RESULT_ACTION":        "updated",
			"BM_RESULT_FIREWALL_NOTE": "opened",
			"BM_RESULT_NOTE":          "ok",
		}, "raw", nil
	}

	res, err := svc.Execute(ships.Ship{Host: "91.98.67.180", SSHUser: "root", SSHPort: 22}, "pw", ActionInput{Mode: "apply"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if res.Host != "91.98.67.180" {
		t.Fatalf("expected fallback host, got %q", res.Host)
	}
	if res.Protocol != "HTTP" || res.Port != "18181" || res.User != "beamhttp" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestInventoryErrorPassthrough(t *testing.T) {
	svc := NewService()
	svc.runRemoteFn = func(_ sshx.Target, _ ActionInput) (remote.KeyValues, string, error) {
		return nil, "", errors.New("boom")
	}

	_, err := svc.Inventory(ships.Ship{Host: "x", SSHUser: "root", SSHPort: 22}, "pw")
	if err == nil {
		t.Fatal("expected error")
	}
}
