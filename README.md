# bramble-cli

Command-line interface for [Bramble](https://github.com/justinlindh/bramble) LoRa mesh nodes, built on [bramble-go](https://github.com/justinlindh/bramble-go).

Current SDK protocol compatibility follows `bramble-go` (`MinProtocolVersion=0.1.0`, `MaxProtocolVersion=0.5.0`).

## Examples

See the [`examples/`](examples/) directory for common usage patterns:

- [`01-connect.sh`](examples/01-connect.sh) — BLE vs WiFi vs serial connection
- [`02-send-receive.sh`](examples/02-send-receive.sh) — Basic send and receive
- [`03-channels.sh`](examples/03-channels.sh) — Channel operations
- [`04-location.sh`](examples/04-location.sh) — Location sharing
- [`05-monitor.sh`](examples/05-monitor.sh) — Monitor and debug output

## Install

```bash
go install github.com/justinlindh/bramble-cli/cmd/bramble@latest
```

Or build from source:

```bash
git clone ssh://git@192.0.2.0:2222/justinlindh/bramble-cli.git
cd bramble-cli
go build -o bramble ./cmd/bramble
```

> **Private module note:** This depends on `github.com/justinlindh/bramble-go`, a private Gitea module. The `go.mod` uses a `replace` directive pointing to a local checkout. Configure SSH access and set `GOPRIVATE=github.com/*`.

## Connection / Transport

Auto-detect USB serial (scans `/dev/ttyUSB*` and `/dev/ttyACM*`):
```bash
bramble status
```

Specify a serial port:
```bash
bramble --port /dev/ttyUSB0 status
```

WebSocket transport (e.g. ESP32 in AP mode):
```bash
bramble --transport ws://192.168.4.1/ws status
```

Bluetooth Low Energy transport (scan by advertised name):
```bash
bramble --ble Bramble status
```

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--ble <name>` | `-b` | BLE device name to scan for (example: `Bramble`) |
| `--port <path>` | `-p` | Serial port path (example: `/dev/ttyUSB0`) |
| `--transport <url>` | `-t` | WebSocket transport URL (example: `ws://192.168.4.1/ws`) |
| `--json` | | Output command results as JSON |

## Commands

### `bramble status`
Show node address, firmware, radio, peers, counters, and uptime.

### `bramble peers`
List direct radio neighbors with RSSI, SNR, and last-heard time.

### `bramble routes`
Show the mesh routing table.

### `bramble ping`
Ping the connected node.

### `bramble send <address> <message>`
Send a unicast message to a specific node.
```bash
bramble send CAFEBABE "hello there"
```

### `bramble broadcast <message>`
Send a mesh-wide message.
- Default: public Broadcast channel
- Use `--channel <index>` to send on a specific channel
- Use `--wait-delivery <seconds>` to wait for delivery telemetry after send

```bash
bramble broadcast "hello everyone"
bramble broadcast --channel 2 "hello channel 2"
bramble broadcast --wait-delivery 10 "delivery telemetry please"
```

### `bramble monitor`
Stream real-time node events. Press `Ctrl+C` to stop.

Event types include protocol-native mesh + telemetry streams. Topic filtering is supported.

| Flag | Default | Description |
|------|---------|-------------|
| `--topic <csv>` | | Topic filter CSV (`wifi`, `gps`, `mesh`, `location`, `traffic`) |
| `--events <csv>` | | Legacy event filter (`message`, `ack`, `neighbor`, `broadcast-delivery`) |
| `--grep <regex>` | | Regex to filter monitor output |
| `--follow` | `true` | Keep streaming new events (use `--follow=false` to stop after first match) |
| `--since <duration>` | | Show only events newer than this duration (hint) |
| `--messages` | | Shorthand: only show message events |
| `--neighbors` | | Shorthand: only show neighbor-change events |

```bash
bramble monitor                                      # all events
bramble monitor --events message,ack                 # legacy event filter set
bramble monitor --topic wifi,gps,location            # topic filter
bramble monitor --topic traffic --grep route         # grep filter
bramble monitor --topic gps,location --json          # structured JSON output
bramble monitor --follow=false --topic gps           # stop after first matching event
bramble monitor --since 30s --topic location         # recent window hint
bramble monitor --messages                           # only message events
bramble monitor --neighbors                          # only neighbor-change events
```

### `bramble traffic`
Traffic debug telemetry commands:
- `bramble traffic monitor`: live TX/RX telemetry stream
- `bramble traffic export`: export ring-buffer events to JSONL

```bash
bramble traffic monitor
bramble traffic monitor --tx-only
bramble traffic monitor --rx-only --category routing

bramble traffic export
bramble traffic export --since 12345 --limit 100   # fetch events with seq > 12345
bramble traffic export --format jsonl > traffic-events.jsonl
```

### `bramble config get`
Print the full node configuration.

### `bramble config set-name <name>`
Set the node display name (max 32 characters).

### `bramble config set-radio`
Update radio parameters:
```bash
bramble config set-radio --freq 915.0 --sf 10 --bw 125 --cr 5 --txpower 20
```

### `bramble channels list`
List configured channels (shows 🔒 for channels with a PSK).

### `bramble channels add <name> [psk]`
Add a channel with an optional pre-shared key.

### `bramble channels remove <index>`
Remove a channel by index.

### `bramble channels set-default <index>`
Set the default outgoing channel.

### `bramble ota`
Trigger an OTA firmware update from a URL. The node downloads and applies the firmware, then reboots.

| Flag | Default | Description |
|------|---------|-------------|
| `--url <url>` | *(required)* | Firmware URL (`http(s)://.../bramble.bin`) |
| `--wait` | `true` | Wait for node reboot/reconnect and report OTA outcome |
| `--wait-timeout <duration>` | `2m` | Max time to wait for OTA reboot/reconnect |
| `--poll-interval <duration>` | `2s` | Status poll interval while waiting for OTA outcome |

```bash
bramble ota --url http://192.0.2.0:8080/firmware/bramble.bin
bramble ota --url http://192.0.2.0:8080/firmware/bramble.bin --wait-timeout 5m
bramble ota --url http://192.0.2.0:8080/firmware/bramble.bin --wait=false
```

### `bramble probe`
Send a network probe. Responses appear in `bramble monitor`.

### `bramble discover`
Scan for Bramble nodes on the local network via mDNS.

| Flag | Default | Description |
|------|---------|-------------|
| `--timeout <duration>` | `3s` | How long to scan for nodes |

```bash
bramble discover
bramble discover --timeout 10s
```

### `bramble reboot`
Reboot the node.

### `bramble location status`
Show location data for all known peers.

### `bramble location set-config`
Set canonical location policy fields. Pass flags individually or supply a JSON file with `--file`.

| Flag | Description |
|------|-------------|
| `--file <path>` | Path to JSON file containing canonical location config fields |
| `--enabled` | Enable/disable location sharing |
| `--default-tier <tier>` | Default sharing tier |
| `--interval-s <seconds>` | Default share interval in seconds |
| `--source <source>` | Location source |
| `--contact-rules <json>` | JSON array for `contact_rules` |
| `--channel-targets <json>` | JSON array for `channel_targets` |

```bash
bramble location set-config --enabled --default-tier full --interval-s 30 --source gps \
  --contact-rules '[{"address":"6CBF8FE3","enabled":true,"tier":"full","interval_s":30}]'

# Or supply a JSON file
bramble location set-config --file location-config.json
```

### `bramble location get-config`
Get canonical location policy block from node config.

```bash
bramble location get-config --json
```

### `bramble location set-contact <address> <tier>`
Set a per-peer location contact rule quickly.

### `bramble location remove-contact <address>`
Remove a location contact rule for a peer.

### `bramble location share-once <address>`
Send a one-time location update.

## JSON Output

All commands support `--json` for machine-readable output:
```bash
bramble --json status | jq .address
bramble --json peers | jq '.[].rssi'
```

## Shell Completion

Bramble supports shell completions via Cobra:

```bash
# Bash
bramble completion bash > /etc/bash_completion.d/bramble

# Zsh
bramble completion zsh > "${fpath[1]}/_bramble"

# Fish
bramble completion fish > ~/.config/fish/completions/bramble.fish
```

## Docs Maintenance / Drift Check

CLI help text (`./bramble <command> --help`) is the source of truth. After adding/changing flags or commands:

1. Rebuild the CLI: `go build -o bramble ./cmd/bramble`
2. Update README command snippets and flag tables.
3. Run doc drift guard:

```bash
scripts/check-doc-drift.sh
```

This script validates a small set of critical README snippets and help flags (global transport flags, `broadcast --wait-delivery`, `monitor --events`, and traffic monitor/export flags).

## License

MIT
