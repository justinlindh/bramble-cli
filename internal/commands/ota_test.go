package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestOTACmd_RejectsNonHTTPScheme(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"ftp scheme", "ftp://example.com/bramble.bin"},
		{"file scheme", "file:///etc/passwd"},
		{"no scheme", "example.com/bramble.bin"},
		{"empty scheme", "://example.com/bramble.bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newOTACmd()
			cmd.SetArgs([]string{"--url", tt.url, "--wait=false"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", tt.url)
			}
			if !strings.Contains(err.Error(), "invalid URL scheme") {
				t.Fatalf("expected 'invalid URL scheme' in error, got: %v", err)
			}
		})
	}
}

func TestOTACmd_AcceptsHTTPSScheme(t *testing.T) {
	oldRunner := runOTAUpdate
	t.Cleanup(func() { runOTAUpdate = oldRunner })

	runOTAUpdate = func(ctx context.Context, url string) (*bramble.OTAUpdateResponse, error) {
		return &bramble.OTAUpdateResponse{OK: true}, nil
	}

	cmd := newOTACmd()
	cmd.SetArgs([]string{"--url", "https://example.com/bramble.bin", "--wait=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error for https URL, got: %v", err)
	}
}

func TestOTACmd_AcceptsHTTPScheme(t *testing.T) {
	oldRunner := runOTAUpdate
	t.Cleanup(func() { runOTAUpdate = oldRunner })

	runOTAUpdate = func(ctx context.Context, url string) (*bramble.OTAUpdateResponse, error) {
		return &bramble.OTAUpdateResponse{OK: true}, nil
	}

	cmd := newOTACmd()
	cmd.SetArgs([]string{"--url", "http://192.0.2.1/bramble.bin", "--wait=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error for http URL, got: %v", err)
	}
}

func TestOTACmd_RequiresURL(t *testing.T) {
	cmd := newOTACmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --url is missing")
	}
	if !strings.Contains(err.Error(), "required flag(s) \"url\" not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOTACmd_AcceptsURLAndCallsClient_NoWait(t *testing.T) {
	oldRunner := runOTAUpdate
	t.Cleanup(func() { runOTAUpdate = oldRunner })

	const wantURL = "https://example.com/bramble.bin"
	called := false
	runOTAUpdate = func(ctx context.Context, url string) (*bramble.OTAUpdateResponse, error) {
		called = true
		if url != wantURL {
			t.Fatalf("expected URL %q, got %q", wantURL, url)
		}
		return &bramble.OTAUpdateResponse{OK: true, Note: "queued", Partition: "app0"}, nil
	}

	cmd := newOTACmd()
	cmd.SetArgs([]string{"--url", wantURL, "--wait=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected OTA update runner to be called")
	}
}

func TestOTACmd_WaitDetectsRebootAndReconnect(t *testing.T) {
	oldRunner := runOTAUpdate
	oldStatus := runStatusCheck
	oldSleep := otaSleep
	t.Cleanup(func() {
		runOTAUpdate = oldRunner
		runStatusCheck = oldStatus
		otaSleep = oldSleep
	})

	runOTAUpdate = func(ctx context.Context, url string) (*bramble.OTAUpdateResponse, error) {
		return &bramble.OTAUpdateResponse{OK: true, Note: "queued", Partition: "app0"}, nil
	}

	calls := 0
	runStatusCheck = func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return nil
		}
		if calls == 2 {
			return errors.New("disconnected")
		}
		return nil
	}
	otaSleep = func(_ time.Duration) {}

	cmd := newOTACmd()
	cmd.SetArgs([]string{"--url", "http://example/bramble.bin", "--wait-timeout", "1s", "--poll-interval", "1ms"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected reboot success, got %v", err)
	}
}
