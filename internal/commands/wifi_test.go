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

	got := formatWifiStatus(&bramble.WifiStatus{Mode: "station", SSID: "HomeWiFi", IP: "192.0.2.0", RSSI: -61})
	for _, want := range []string{"Mode: STATION", "SSID: HomeWiFi", "IP: 192.0.2.0", "RSSI: -61 dBm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatWifiStatus(station) = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "Clients:") {
		t.Fatalf("formatWifiStatus(station) should not include clients: %q", got)
	}
}
