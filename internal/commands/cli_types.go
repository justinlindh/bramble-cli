package commands

import bramble "github.com/justinlindh/bramble-go"

// BroadcastResult is the JSON output for the broadcast command.
type BroadcastResult struct {
	Text            string                      `json:"text"`
	Status          string                      `json:"status"`
	Channel         int                         `json:"channel,omitempty"`
	BroadcastID     string                      `json:"broadcast_id,omitempty"`
	Deliveries      []bramble.BroadcastDelivery `json:"deliveries,omitempty"`
	DeliveryWindowS int                         `json:"delivery_window_s,omitempty"`
	DeliveryCount   int                         `json:"delivery_count,omitempty"`
}

// ChannelAddResult is the JSON output for the channels add command.
type ChannelAddResult struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// ChannelStatusResult is the JSON output for channels remove/set-default commands.
type ChannelStatusResult struct {
	Index  int    `json:"index"`
	Status string `json:"status"`
}

// SetNameResult is the JSON output for the config set-name command.
type SetNameResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// StatusResult is a generic status-only JSON output.
type StatusResult struct {
	Status string `json:"status"`
}

// LocationContactResult is the JSON output for the location set-contact command.
type LocationContactResult struct {
	Addr   string `json:"addr"`
	Tier   string `json:"tier"`
	Status string `json:"status"`
}

// LocationAddrResult is the JSON output for location remove-contact/share-once commands.
type LocationAddrResult struct {
	Addr   string `json:"addr"`
	Status string `json:"status"`
}

// LocationSetConfigResult is the JSON output for the location set-config command.
type LocationSetConfigResult struct {
	Status   string                 `json:"status"`
	Location bramble.LocationConfig `json:"location"`
}

// PingResult is the JSON output for the ping command.
type PingResult struct {
	Address         string `json:"address"`
	ProtocolVersion string `json:"protocol_version"`
	Status          string `json:"status"`
}

// SendCommandResult is the JSON output for the send command.
type SendCommandResult struct {
	Dest   string `json:"dest"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

// ProbeCommandResult is the JSON output for the probe command.
type ProbeCommandResult struct {
	ProbeID   int                   `json:"probe_id"`
	AckWindow int                   `json:"ack_window"`
	Responses []bramble.ProbeResult `json:"responses,omitempty"`
}

// monitorEventOutput is the JSON envelope for each monitor event.
type monitorEventOutput struct {
	Event     string `json:"event"`
	Topic     string `json:"topic"`
	Timestamp int64  `json:"timestamp"`
	Payload   any    `json:"payload"`
}

// neighborChangePayload is the payload for neighbor_change monitor events.
type neighborChangePayload struct {
	State string `json:"state"`
}
