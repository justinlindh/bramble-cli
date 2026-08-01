package commands

import (
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewWifiCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newWifiCmd()
	if cmd.Use != "wifi" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler for default wifi status")
	}

	statusCmd, _, err := cmd.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find status subcommand: %v", err)
	}
	if statusCmd == nil || statusCmd.Use != "status" {
		t.Fatalf("expected status subcommand, got %#v", statusCmd)
	}
	if statusCmd.RunE == nil {
		t.Fatal("expected status subcommand RunE handler")
	}
}

func TestFormatWifiStatus_AP(t *testing.T) {
	t.Parallel()

	got := formatWifiStatus(&bramble.WifiStatus{Mode: "ap", SSID: "Bramble-EC7A", IP: "192.168.4.1", Clients: 1})
	want := "Mode: AP | SSID: Bramble-EC7A | IP: 192.168.4.1 | Clients: 1"
	if got != want {
		t.Fatalf("formatWifiStatus(AP) = %q, want %q", got, want)
	}
}

func TestFormatWifiStatus_StationIncludesRSSI(t *testing.T) {
	t.Parallel()

	got := formatWifiStatus(&bramble.WifiStatus{Mode: "station", SSID: "HomeWiFi", IP: "192.0.2.50", RSSI: -61})
	for _, want := range []string{"Mode: STATION", "SSID: HomeWiFi", "IP: 192.0.2.50", "RSSI: -61 dBm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatWifiStatus(station) = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "Clients:") {
		t.Fatalf("formatWifiStatus(station) should not include clients: %q", got)
	}
}

func TestNewWifiCmd_HasSetSubcommand(t *testing.T) {
	t.Parallel()

	cmd := newWifiCmd()
	setCmd, _, err := cmd.Find([]string{"set"})
	if err != nil {
		t.Fatalf("find set subcommand: %v", err)
	}
	if setCmd == nil || setCmd.Name() != "set" {
		t.Fatalf("expected set subcommand, got %#v", setCmd)
	}
	if setCmd.RunE == nil {
		t.Fatal("expected set subcommand RunE handler")
	}
	for _, flag := range []string{"password", "open", "reboot"} {
		if setCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected --%s flag on wifi set", flag)
		}
	}
}

func TestNewWifiSetCmd_PasswordNeverPositional(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	if cmd.Use != "set <ssid>" {
		t.Fatalf("unexpected use string: %q (password must not be a positional argument)", cmd.Use)
	}
}

func TestRunWifiSet_SSIDTooLong(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	err := cmd.RunE(cmd, []string{strings.Repeat("a", wifiSSIDMaxLen+1)})
	if err == nil {
		t.Fatal("expected error for too-long ssid")
	}
	if !strings.Contains(err.Error(), "must be 1-32 characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWifiSet_SSIDEmpty(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	err := cmd.RunE(cmd, []string{""})
	if err == nil {
		t.Fatal("expected error for empty ssid")
	}
	if !strings.Contains(err.Error(), "must be 1-32 characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWifiSet_OpenAndPasswordMutuallyExclusive(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	if err := cmd.Flags().Set("open", "true"); err != nil {
		t.Fatalf("set --open: %v", err)
	}
	if err := cmd.Flags().Set("password", "hunter2"); err != nil {
		t.Fatalf("set --password: %v", err)
	}
	err := cmd.RunE(cmd, []string{"HomeWiFi"})
	if err == nil {
		t.Fatal("expected error combining --open and --password")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWifiSet_PasswordTooLong(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	if err := cmd.Flags().Set("password", strings.Repeat("a", wifiPasswordMaxLen+1)); err != nil {
		t.Fatalf("set --password: %v", err)
	}
	err := cmd.RunE(cmd, []string{"HomeWiFi"})
	if err == nil {
		t.Fatal("expected error for too-long password")
	}
	if !strings.Contains(err.Error(), "password too long") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWifiSet_OpenSkipsPasswordPrompt(t *testing.T) {
	t.Parallel()

	cmd := newWifiSetCmd()
	if err := cmd.Flags().Set("open", "true"); err != nil {
		t.Fatalf("set --open: %v", err)
	}
	// With --open, validation passes and the command proceeds to connect
	// (which fails in this test environment); it must not fail on our own
	// validation, and in particular must not block on an interactive prompt.
	err := cmd.RunE(cmd, []string{"GuestWiFi"})
	if err == nil {
		t.Fatal("expected a connection error in the test environment")
	}
	for _, unwanted := range []string{"must be 1-32 characters", "mutually exclusive", "password too long", "--password required"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Fatalf("unexpected validation error leaked through: %v", err)
		}
	}
}
