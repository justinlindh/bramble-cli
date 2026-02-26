package tui

import (
	tea "charm.land/bubbletea/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// ── Tea Msg types ────────────────────────────────────────────────────────────

// MsgReceived is sent when a new message arrives via push notification.
type MsgReceived struct{ Msg bramble.Message }

// AckReceived is sent when a delivery ack arrives.
type AckReceived struct{ Ack bramble.Ack }

// NeighborChanged is sent when the neighbor list changes.
type NeighborChanged struct{}

// TrafficEventReceived is sent on a traffic debug event.
type TrafficEventReceived struct{ Event bramble.TrafficEvent }

// BroadcastDeliveryReceived is sent on a broadcast delivery event.
type BroadcastDeliveryReceived struct{ Delivery bramble.BroadcastDelivery }

// WifiEventReceived is sent on a WiFi event.
type WifiEventReceived struct{ Event bramble.WifiEvent }

// GpsEventReceived is sent on a GPS event.
type GpsEventReceived struct{ Event bramble.GpsEvent }

// LocationEventReceived is sent on a location push event.
type LocationEventReceived struct{ Event bramble.LocationEvent }

// ── Bridge ───────────────────────────────────────────────────────────────────

// Bridge wires SDK push notifications into the Bubble Tea event loop.
type Bridge struct {
	program *tea.Program
}

// NewBridge creates a Bridge for the given program.
func NewBridge(p *tea.Program) *Bridge {
	return &Bridge{program: p}
}

// Start registers all 8 notification callbacks on client.
func (b *Bridge) Start(client *bramble.Client) {
	client.OnMessage(func(msg bramble.Message) {
		b.program.Send(MsgReceived{Msg: msg})
	})
	client.OnAck(func(ack bramble.Ack) {
		b.program.Send(AckReceived{Ack: ack})
	})
	client.OnNeighborChange(func() {
		b.program.Send(NeighborChanged{})
	})
	client.OnTrafficEvent(func(ev bramble.TrafficEvent) {
		b.program.Send(TrafficEventReceived{Event: ev})
	})
	client.OnBroadcastDelivery(func(d bramble.BroadcastDelivery) {
		b.program.Send(BroadcastDeliveryReceived{Delivery: d})
	})
	client.OnWifiEvent(func(ev bramble.WifiEvent) {
		b.program.Send(WifiEventReceived{Event: ev})
	})
	client.OnGpsEvent(func(ev bramble.GpsEvent) {
		b.program.Send(GpsEventReceived{Event: ev})
	})
	client.OnLocationEvent(func(ev bramble.LocationEvent) {
		b.program.Send(LocationEventReceived{Event: ev})
	})
}
