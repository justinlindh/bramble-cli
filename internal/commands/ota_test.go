package commands

import (
	"context"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestOTACmd_RequiresURL(t *testing.T) {
	t.Parallel()

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

func TestOTACmd_AcceptsURLAndCallsClient(t *testing.T) {
	t.Parallel()

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
	cmd.SetArgs([]string{"--url", wantURL})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected OTA update runner to be called")
	}
}
