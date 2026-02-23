package output

import (
	"fmt"
	"strings"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

// FormatBroadcastSendStatus returns a one-line human summary for broadcast send results.
func FormatBroadcastSendStatus(channel int, status, broadcastID string) string {
	var b strings.Builder
	if channel >= 0 {
		fmt.Fprintf(&b, "Channel %d broadcast sent (%s)", channel, status)
	} else {
		fmt.Fprintf(&b, "Broadcast sent (%s)", status)
	}
	if broadcastID != "" {
		fmt.Fprintf(&b, " broadcast_id=%s", broadcastID)
	}
	return b.String()
}

// FormatBroadcastDeliveryLine returns a one-line human summary for delivery telemetry events.
func FormatBroadcastDeliveryLine(ts time.Time, evt bramble.BroadcastDelivery) string {
	return fmt.Sprintf("[%s] BCAST_DELIVERY id=%s recipient=%s status=%s",
		ts.Format("15:04:05"), evt.BroadcastID, evt.Recipient, evt.Status)
}
