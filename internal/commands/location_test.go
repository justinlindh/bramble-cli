package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLocationCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newLocationCmd()
	want := []string{"status", "set-contact", "remove-contact", "share-once", "get-config", "set-config"}

	for _, name := range want {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

func TestRunLocationSetConfig_NoInput(t *testing.T) {
	t.Parallel()

	err := runLocationSetConfig(newLocationSetConfigCmd(), nil)
	if err == nil {
		t.Fatal("expected error when no config values are provided")
	}
	if !strings.Contains(err.Error(), "specify at least one location config field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildLocationConfigFromInput_Flags(t *testing.T) {
	t.Parallel()

	cmd := newLocationSetConfigCmd()
	if err := cmd.Flags().Set("enabled", "true"); err != nil {
		t.Fatalf("set enabled flag: %v", err)
	}
	if err := cmd.Flags().Set("default-tier", "critical"); err != nil {
		t.Fatalf("set default-tier flag: %v", err)
	}
	if err := cmd.Flags().Set("interval-s", "120"); err != nil {
		t.Fatalf("set interval-s flag: %v", err)
	}
	if err := cmd.Flags().Set("source", "gps"); err != nil {
		t.Fatalf("set source flag: %v", err)
	}
	if err := cmd.Flags().Set("contact-rules", `[{"address":"AABBCCDD","enabled":true,"tier":"city","interval_s":90}]`); err != nil {
		t.Fatalf("set contact-rules flag: %v", err)
	}
	if err := cmd.Flags().Set("channel-targets", `[{"channel":1,"enabled":false,"tier":"region","interval_s":300}]`); err != nil {
		t.Fatalf("set channel-targets flag: %v", err)
	}

	cfg, changed, err := buildLocationConfigFromInput(cmd)
	if err != nil {
		t.Fatalf("buildLocationConfigFromInput error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if cfg.Enabled == nil || !*cfg.Enabled {
		t.Fatalf("expected enabled=true, got %+v", cfg.Enabled)
	}
	if cfg.DefaultTier == nil || *cfg.DefaultTier != "critical" {
		t.Fatalf("expected default_tier=critical, got %+v", cfg.DefaultTier)
	}
	if cfg.IntervalS == nil || *cfg.IntervalS != 120 {
		t.Fatalf("expected interval_s=120, got %+v", cfg.IntervalS)
	}
	if cfg.Source == nil || *cfg.Source != "gps" {
		t.Fatalf("expected source=gps, got %+v", cfg.Source)
	}
	if len(cfg.ContactRules) != 1 || cfg.ContactRules[0].Address != "AABBCCDD" {
		t.Fatalf("unexpected contact_rules: %+v", cfg.ContactRules)
	}
	if len(cfg.ChannelTargets) != 1 || cfg.ChannelTargets[0].Channel != 1 {
		t.Fatalf("unexpected channel_targets: %+v", cfg.ChannelTargets)
	}
}

func TestBuildLocationConfigFromInput_FileRejectsAliasFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "location.json")
	content := `{"defaultTier":"critical"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	cmd := newLocationSetConfigCmd()
	if err := cmd.Flags().Set("file", path); err != nil {
		t.Fatalf("set file flag: %v", err)
	}

	_, _, err := buildLocationConfigFromInput(cmd)
	if err == nil {
		t.Fatal("expected decode error for alias field")
	}
	if !strings.Contains(err.Error(), "decode location config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
