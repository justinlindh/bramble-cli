package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

type monitorEvent struct {
	Type       string
	Topic      string
	Timestamp  time.Time
	SearchText string
	Payload    map[string]any
	Line       string
}

type monitorFilterOptions struct {
	topics map[string]struct{}
	grep   *regexp.Regexp
	since  time.Duration
	now    func() time.Time
}

func newMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Live event stream from the node",
		Long: `Subscribe to real-time events from the connected node.
Prints incoming messages, delivery acks, and neighbor changes.
Press Ctrl+C to stop.

Flags:
  --messages    Only show message events
  --neighbors   Only show neighbor change events
  --events      Comma-separated event filter (message, ack, neighbor, broadcast-delivery)
  --follow      Keep streaming events (default true)
  --since       Only show events newer than this duration (e.g. 5m, 1h)
  --topic       Comma-separated topic filter (wifi,gps,mesh,location,traffic)
  --grep        Regex filter applied to event text`,
		RunE: runMonitor,
	}
	cmd.Flags().Bool("messages", false, "only show message events")
	cmd.Flags().Bool("neighbors", false, "only show neighbor-change events")
	cmd.Flags().StringSlice("events", nil, "event filter (message, ack, neighbor, broadcast-delivery)")
	cmd.Flags().Bool("follow", true, "keep streaming new events")
	cmd.Flags().Duration("since", 0, "show only events newer than this duration")
	cmd.Flags().String("topic", "", "topic filter CSV (wifi,gps,mesh,location,traffic)")
	cmd.Flags().String("grep", "", "regex to filter monitor output")
	return cmd
}

func runMonitor(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	onlyMessages, _ := cmd.Flags().GetBool("messages")
	onlyNeighbors, _ := cmd.Flags().GetBool("neighbors")
	eventFilters, _ := cmd.Flags().GetStringSlice("events")
	follow, _ := cmd.Flags().GetBool("follow")
	since, _ := cmd.Flags().GetDuration("since")
	topicCSV, _ := cmd.Flags().GetString("topic")
	grepPattern, _ := cmd.Flags().GetString("grep")

	var grepRE *regexp.Regexp
	if strings.TrimSpace(grepPattern) != "" {
		re, err := regexp.Compile(grepPattern)
		if err != nil {
			return fmt.Errorf("bramble-cli: invalid --grep regex: %w", err)
		}
		grepRE = re
	}

	filters := monitorFilterOptions{
		topics: parseMonitorTopicsCSV(topicCSV),
		grep:   grepRE,
		since:  since,
		now:    time.Now,
	}

	// Default: show everything.
	showMessages := !onlyNeighbors || onlyMessages
	showNeighbors := !onlyMessages || onlyNeighbors
	showAcks := !onlyMessages && !onlyNeighbors
	showBroadcastDeliveries := !onlyMessages && !onlyNeighbors
	showTraffic := true
	showWifi := true
	showGps := true
	showLocation := true

	if len(eventFilters) > 0 {
		showMessages = false
		showNeighbors = false
		showAcks = false
		showBroadcastDeliveries = false
		showTraffic = false
		showWifi = false
		showGps = false
		showLocation = false
		for _, raw := range eventFilters {
			switch strings.TrimSpace(strings.ToLower(raw)) {
			case "message", "messages":
				showMessages = true
			case "ack", "acks":
				showAcks = true
			case "neighbor", "neighbors", "neighbor-change":
				showNeighbors = true
			case "broadcast-delivery", "broadcast_delivery", "broadcastdelivery":
				showBroadcastDeliveries = true
			case "traffic":
				showTraffic = true
			case "wifi":
				showWifi = true
			case "gps":
				showGps = true
			case "location":
				showLocation = true
			}
		}
	}

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Fprintln(os.Stderr, "Monitoring node events... (Ctrl+C to stop)")

	matchedCh := make(chan struct{}, 1)
	emit := func(evt monitorEvent) {
		if !monitorEventMatches(filters, evt) {
			return
		}
		if flagJSON {
			b, _ := json.Marshal(map[string]any{
				"event":     evt.Type,
				"topic":     evt.Topic,
				"timestamp": evt.Timestamp.Unix(),
				"payload":   evt.Payload,
			})
			fmt.Fprintln(os.Stdout, string(b))
		} else {
			fmt.Fprintln(os.Stdout, evt.Line)
		}
		if monitorShouldStopAfterMatch(follow) {
			select {
			case matchedCh <- struct{}{}:
			default:
			}
		}
	}

	if showMessages {
		client.OnMessage(func(msg bramble.Message) {
			ts := time.Unix(msg.Timestamp, 0)
			emit(monitorEvent{
				Type:      "message",
				Topic:     "mesh",
				Timestamp: ts,
				SearchText: strings.Join([]string{
					msg.From, msg.To, msg.Text, msg.Tier, msg.MsgID,
				}, " "),
				Payload: map[string]any{
					"from":   msg.From,
					"to":     msg.To,
					"text":   msg.Text,
					"tier":   msg.Tier,
					"msg_id": msg.MsgID,
				},
				Line: fmt.Sprintf("[%s] MSG %s→%s  %q", ts.Format("15:04:05"), msg.From, msg.To, msg.Text),
			})
		})
	}

	if showAcks {
		client.OnAck(func(ack bramble.Ack) {
			now := time.Now()
			emit(monitorEvent{
				Type:       "ack",
				Topic:      "mesh",
				Timestamp:  now,
				SearchText: fmt.Sprintf("packet#%d status=%s", ack.PacketID, ack.Status),
				Payload: map[string]any{
					"packet_id": ack.PacketID,
					"status":    ack.Status,
				},
				Line: fmt.Sprintf("[%s] ACK  packet#%d  status=%s", now.Format("15:04:05"), ack.PacketID, ack.Status),
			})
		})
	}

	if showNeighbors {
		client.OnNeighborChange(func() {
			now := time.Now()
			emit(monitorEvent{
				Type:       "neighbor_change",
				Topic:      "mesh",
				Timestamp:  now,
				SearchText: "neighbor table updated",
				Payload:    map[string]any{"state": "updated"},
				Line:       fmt.Sprintf("[%s] NEIGHBOR  table updated", now.Format("15:04:05")),
			})
		})
	}

	if showBroadcastDeliveries {
		client.OnBroadcastDelivery(func(evt bramble.BroadcastDelivery) {
			ts := time.Now()
			if evt.TimestampMs > 0 {
				ts = time.UnixMilli(evt.TimestampMs)
			}
			emit(monitorEvent{
				Type:      "broadcast_delivery",
				Topic:     "mesh",
				Timestamp: ts,
				SearchText: strings.Join([]string{
					evt.BroadcastID, evt.Recipient, evt.Status,
				}, " "),
				Payload: map[string]any{
					"broadcast_id": evt.BroadcastID,
					"recipient":    evt.Recipient,
					"status":       evt.Status,
					"timestamp_ms": evt.TimestampMs,
				},
				Line: monitorBroadcastDeliveryLine(ts, evt),
			})
		})
	}

	if showTraffic {
		client.OnTrafficEvent(func(evt bramble.TrafficEvent) {
			now := time.Now()
			direction := "RX"
			if evt.IsTx {
				direction = "TX"
			}
			emit(monitorEvent{
				Type:       "traffic",
				Topic:      "traffic",
				Timestamp:  now,
				SearchText: fmt.Sprintf("%s %s pkt=%d len=%d tier=%s", direction, evt.Category, evt.PktType, evt.PacketLen, evt.AirtimeTier),
				Payload: map[string]any{
					"seq":          evt.Seq,
					"timestamp_ms": evt.TimestampMs,
					"pkt_type":     evt.PktType,
					"category":     evt.Category,
					"airtime_tier": evt.AirtimeTier,
					"packet_len":   evt.PacketLen,
					"rssi":         evt.RSSI,
					"is_tx":        evt.IsTx,
				},
				Line: fmt.Sprintf("[%s] TRAFFIC %-2s %-10s pkt=%d len=%d tier=%s", now.Format("15:04:05"), direction, evt.Category, evt.PktType, evt.PacketLen, evt.AirtimeTier),
			})
		})
	}

	if showWifi {
		client.OnWifiEvent(func(evt bramble.WifiEvent) {
			now := time.Now()
			emit(monitorEvent{Type: "wifi", Topic: "wifi", Timestamp: now, SearchText: fmt.Sprintf("%s %s %s", evt.Event, evt.Mode, evt.IP), Payload: map[string]any{"event": evt.Event, "mode": evt.Mode, "connected": evt.Connected, "ssid": evt.SSID, "ip": evt.IP, "rssi": evt.RSSI}, Line: fmt.Sprintf("[%s] WIFI event=%s mode=%s ip=%s", now.Format("15:04:05"), evt.Event, evt.Mode, evt.IP)})
		})
	}

	if showGps {
		client.OnGpsEvent(func(evt bramble.GpsEvent) {
			now := time.Now()
			emit(monitorEvent{Type: "gps", Topic: "gps", Timestamp: now, SearchText: fmt.Sprintf("%s valid=%t", evt.Event, evt.Valid), Payload: map[string]any{"event": evt.Event, "valid": evt.Valid, "lat": evt.Lat, "lon": evt.Lon, "alt_m": evt.AltM, "sats": evt.Sats}, Line: fmt.Sprintf("[%s] GPS event=%s valid=%t", now.Format("15:04:05"), evt.Event, evt.Valid)})
		})
	}

	if showLocation {
		client.OnLocationEvent(func(evt bramble.LocationEvent) {
			ts := time.Now()
			if evt.TimestampMs > 0 {
				ts = time.UnixMilli(int64(evt.TimestampMs))
			}
			emit(monitorEvent{Type: "location", Topic: "location", Timestamp: ts, SearchText: fmt.Sprintf("%s peer=%s tier=%d", evt.Event, evt.Peer, evt.Tier), Payload: map[string]any{"event": evt.Event, "peer": evt.Peer, "tier": evt.Tier, "timestamp_ms": evt.TimestampMs, "rssi": evt.RSSI, "snr": evt.SNR, "count": evt.Count}, Line: fmt.Sprintf("[%s] LOCATION event=%s peer=%s tier=%d", ts.Format("15:04:05"), evt.Event, evt.Peer, evt.Tier)})
		})
	}

	// Wait for Ctrl+C or first matching event when --follow=false.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if monitorShouldStopAfterMatch(follow) {
		select {
		case <-sigCh:
		case <-matchedCh:
		}
	} else {
		<-sigCh
	}

	fmt.Fprintln(os.Stderr, "\nStopping monitor.")
	return nil
}

func parseMonitorTopicsCSV(csv string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, part := range strings.Split(csv, ",") {
		topic := strings.TrimSpace(strings.ToLower(part))
		if topic == "" {
			continue
		}
		set[topic] = struct{}{}
	}
	return set
}

func monitorEventMatches(opts monitorFilterOptions, evt monitorEvent) bool {
	if opts.since > 0 {
		nowFn := opts.now
		if nowFn == nil {
			nowFn = time.Now
		}
		if evt.Timestamp.Before(nowFn().Add(-opts.since)) {
			return false
		}
	}
	if len(opts.topics) > 0 {
		if _, ok := opts.topics[strings.ToLower(evt.Topic)]; !ok {
			return false
		}
	}
	if opts.grep != nil && !opts.grep.MatchString(evt.SearchText) {
		return false
	}
	return true
}

func monitorShouldStopAfterMatch(follow bool) bool {
	return !follow
}

func monitorBroadcastDeliveryLine(now time.Time, evt bramble.BroadcastDelivery) string {
	return output.FormatBroadcastDeliveryLine(now, evt)
}

func monitorBroadcastDeliveryJSON(evt bramble.BroadcastDelivery) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event":   "broadcast_delivery",
		"payload": evt,
	})
}
