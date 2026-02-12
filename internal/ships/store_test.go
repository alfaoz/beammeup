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
		Name:             "RPS VPS",
		Host:             "91.98.67.180",
		SSHPort:          22,
		SSHUser:          "root",
		Protocol:         "http",
		ProxyPort:        18181,
		NoFirewallChange: true,
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
	for _, key := range []string{"HOST=91.98.67.180", "SSH_PORT=22", "SSH_USER=root", "PROTOCOL=http", "PROXY_PORT=18181", "NO_FIREWALL_CHANGE=1"} {
		if !strings.Contains(got, key) {
			t.Fatalf("expected %q in file", key)
		}
	}

	loaded, err := store.Load("rps-vps")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Host != "91.98.67.180" || loaded.SSHUser != "root" || loaded.ProxyPort != 18181 {
		t.Fatalf("unexpected loaded ship: %+v", loaded)
	}
	if !loaded.NoFirewallChange {
		t.Fatalf("expected NoFirewallChange=true")
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
