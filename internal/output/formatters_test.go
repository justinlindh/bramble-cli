package output

import (
	"strings"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestFormatBroadcastSendStatus(t *testing.T) {
	cases := []struct {
		name        string
		channel     int
		status      string
		broadcastID string
		contains    []string
	}{
		{
			name:        "with channel and id",
			channel:     7,
			status:      "ok",
			broadcastID: "abc",
			contains:    []string{"Channel 7", "(ok)", "broadcast_id=abc"},
		},
		{
			name:        "without channel and id",
			channel:     -1,
			status:      "queued",
			broadcastID: "",
			contains:    []string{"Broadcast sent (queued)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatBroadcastSendStatus(tc.channel, tc.status, tc.broadcastID)
			for _, c := range tc.contains {
				if !strings.Contains(got, c) {
					t.Fatalf("expected %q to contain %q", got, c)
				}
			}
		})
	}
}

func TestFormatBroadcastDeliveryLine(t *testing.T) {
	ts := time.Date(2026, 3, 6, 9, 8, 7, 0, time.UTC)
	evt := bramble.BroadcastDelivery{
		BroadcastID: "b1",
		Recipient:   "00001EE6",
		Status:      "delivered",
	}

	got := FormatBroadcastDeliveryLine(ts, evt)
	wantParts := []string{"[09:08:07]", "id=b1", "recipient=00001EE6", "status=delivered"}
	for _, p := range wantParts {
		if !strings.Contains(got, p) {
			t.Fatalf("expected %q to contain %q", got, p)
		}
	}
}
