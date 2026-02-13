package ships

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreSaveLoadLegacyCompatibility(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	saved, err := store.Save(Ship{
		Name:                    "RPS VPS",
		Host:                    "REDACTED_IP",
		SSHPort:                 22,
		SSHUser:                 "root",
		Protocol:                "http",
		HTTPMode:                "sidecar",
		ProxyPort:               18181,
		NoFirewallChange:        true,
		ListenLocal:             true,
		SmartBlinder:            true,
		SmartBlinderIdleMinutes: 15,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Name != "rps-vps" {
		t.Fatalf("expected sanitized name, got %q", saved.Name)
	}

	content, err := os.ReadFile(filepath.Join(dir, "rps-vps.ship"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)
	for _, key := range []string{
		"HOST=REDACTED_IP",
		"SSH_PORT=22",
		"SSH_USER=root",
		"PROTOCOL=http",
		"HTTP_MODE=sidecar",
		"PROXY_PORT=18181",
		"NO_FIREWALL_CHANGE=1",
		"LISTEN_LOCAL=1",
		"SMART_BLINDER=1",
		"SMART_BLINDER_IDLE_MINUTES=15",
	} {
		if !strings.Contains(got, key) {
			t.Fatalf("expected %q in file", key)
		}
	}

	loaded, err := store.Load("rps-vps")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Host != "REDACTED_IP" || loaded.SSHUser != "root" || loaded.ProxyPort != 18181 {
		t.Fatalf("unexpected loaded ship: %+v", loaded)
	}
	if loaded.HTTPMode != "sidecar" {
		t.Fatalf("expected HTTPMode=sidecar, got %q", loaded.HTTPMode)
	}
	if !loaded.NoFirewallChange {
		t.Fatalf("expected NoFirewallChange=true")
	}
	if !loaded.ListenLocal {
		t.Fatalf("expected ListenLocal=true")
	}
	if !loaded.SmartBlinder {
		t.Fatalf("expected SmartBlinder=true")
	}
	if loaded.SmartBlinderIdleMinutes != 15 {
		t.Fatalf("expected SmartBlinderIdleMinutes=15, got %d", loaded.SmartBlinderIdleMinutes)
	}
}

func TestStoreLoadLegacyDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	legacy := "HOST=203.0.113.10\nSSH_USER=root\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy.ship"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.Load("legacy")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Protocol != "http" {
		t.Fatalf("expected default protocol http, got %q", loaded.Protocol)
	}
	if loaded.SSHPort != 22 {
		t.Fatalf("expected default ssh port 22, got %d", loaded.SSHPort)
	}
	if loaded.ProxyPort != 18181 {
		t.Fatalf("expected default proxy port 18181, got %d", loaded.ProxyPort)
	}
	if loaded.ListenLocal {
		t.Fatalf("expected default ListenLocal=false")
	}
	if !loaded.SmartBlinder {
		t.Fatalf("expected default SmartBlinder=true")
	}
	if loaded.SmartBlinderIdleMinutes != 10 {
		t.Fatalf("expected default SmartBlinderIdleMinutes=10, got %d", loaded.SmartBlinderIdleMinutes)
	}
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	_, err = store.Save(Ship{Name: "deleteme", Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete("deleteme"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deleteme.ship")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}
