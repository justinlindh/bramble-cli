package commands

import (
	"path/filepath"
	"testing"

	"github.com/justinlindh/bramble-cli/internal/devices"
)

// writeTestBook creates a devices.json under a temp XDG_CONFIG_HOME and points
// DefaultPath at it via the environment.
func writeTestBook(t *testing.T, entries map[string]devices.Entry) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	path := filepath.Join(cfg, "bramble", "devices.json")
	book := &devices.Book{Devices: map[string]devices.Entry{}}
	for alias, e := range entries {
		if err := book.Add(alias, e); err != nil {
			t.Fatalf("seed add %q: %v", alias, err)
		}
	}
	if err := book.Save(path); err != nil {
		t.Fatalf("seed save: %v", err)
	}
}

func resetResolution() {
	flagAuthToken = ""
	flagDevice = ""
	resolvedDevice = nil
}

func TestLoadDeviceEntry_UnknownAliasErrors(t *testing.T) {
	writeTestBook(t, map[string]devices.Entry{"v4": {Host: "198.51.100.146", Token: "tok"}})
	resetResolution()
	t.Cleanup(resetResolution)

	flagDevice = "nope"
	if err := loadDeviceEntry(); err == nil {
		t.Fatal("expected error for unknown alias")
	}
}

func TestLoadDeviceEntry_PopulatesResolvedDevice(t *testing.T) {
	writeTestBook(t, map[string]devices.Entry{"v4": {Host: "198.51.100.146", Token: "tok", Name: "V4"}})
	resetResolution()
	t.Cleanup(resetResolution)

	flagDevice = "v4"
	if err := loadDeviceEntry(); err != nil {
		t.Fatalf("loadDeviceEntry: %v", err)
	}
	if resolvedDevice == nil {
		t.Fatal("resolvedDevice is nil")
	}
	if resolvedDevice.Host != "ws://198.51.100.146/ws" {
		t.Errorf("host = %q, want normalized ws url", resolvedDevice.Host)
	}
}

func TestResolvedAuthToken_Precedence(t *testing.T) {
	resetResolution()
	t.Cleanup(resetResolution)

	// alias token used when no flag/env.
	t.Setenv("BRAMBLE_TOKEN", "")
	resolvedDevice = &devices.Entry{Host: "ws://h/ws", Token: "alias-token"}
	if got := resolvedAuthToken(); got != "alias-token" {
		t.Fatalf("expected alias token, got %q", got)
	}

	// env beats alias.
	t.Setenv("BRAMBLE_TOKEN", "env-token")
	if got := resolvedAuthToken(); got != "env-token" {
		t.Fatalf("expected env token, got %q", got)
	}

	// flag beats env and alias.
	flagAuthToken = "flag-token"
	if got := resolvedAuthToken(); got != "flag-token" {
		t.Fatalf("expected flag token, got %q", got)
	}
}

func TestResolvedAuthToken_NoAliasUnchanged(t *testing.T) {
	resetResolution()
	t.Cleanup(resetResolution)
	t.Setenv("BRAMBLE_TOKEN", "")
	if got := resolvedAuthToken(); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}
