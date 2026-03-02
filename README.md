# bramble-cli

`bramble-cli` is the command-line and terminal UI client for Bramble mesh nodes. Use it to connect over serial, WebSocket, or BLE; send and monitor mesh traffic; inspect node health; and manage configuration from a laptop shell or an interactive full-screen TUI.

It is built on [bramble-go](https://github.com/justinlindh/bramble-go), and follows its protocol compatibility (`MinProtocolVersion=0.1.0`, `MaxProtocolVersion=0.5.0`).

## Table of Contents

- [Install](#install)
- [Quick Start](#quick-start)
- [Terminal UI](#terminal-ui)
- [Global Flags](#global-flags)
- [Commands](#commands)
  - [Messaging](#messaging)
  - [Monitoring and Diagnostics](#monitoring-and-diagnostics)
  - [Configuration](#configuration)
  - [Location](#location)
  - [System and Network](#system-and-network)
- [Shell Completion](#shell-completion)
- [JSON Output](#json-output)
- [Examples](#examples)
- [License](#license)

## Install

```bash
go install github.com/justinlindh/bramble-cli/cmd/bramble@latest
```

Or build from source:

```bash
git clone https://github.com/justinlindh/bramble-cli.git
cd bramble-cli
go build -o bramble ./cmd/bramble
```

## Quick Start

Auto-detect USB serial (scans `/dev/ttyUSB*` and `/dev/ttyACM*`):

```bash
bramble status
```

Specify a serial port:

```bash
bramble --port /dev/ttyUSB0 status
```

WebSocket transport:

```bash
bramble --transport ws://192.168.4.1/ws status
```

Bluetooth Low Energy transport:

```bash
bramble --ble Bramble status
```

## Terminal UI

Launch the interactive TUI:

```bash
bramble tui --transport ws://192.0.2.0/ws
bramble tui --port /dev/ttyUSB0
```

The TUI is designed as an IRC-style operations console for Bramble. It combines live chat, system events, slash-command output, and connection status in a single full-screen view so you can operate a node without bouncing between multiple terminal commands.

![Bramble TUI overview](docs/images/tui-overview.png)

![Bramble TUI chat flow](docs/images/tui-chat.gif)

### What it shows

- **Unified scrollback:** inbound/outbound messages, delivery updates, and node events
- **Status bar:** transport state, active buffers, unread counts, peer counts, clock
- **Command input:** message composition plus slash commands like `/nodes`, `/stats`, `/config`, `/location`
- **Buffer model:** broadcast, channel, and DM buffers with quick keyboard switching

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--ble <name>` | `-b` | BLE device name to scan for (example: `Bramble`) |
| `--port <path>` | `-p` | Serial port path (example: `/dev/ttyUSB0`) |
| `--transport <url>` | `-t` | WebSocket transport URL (example: `ws://192.168.4.1/ws`) |
| `--json` | | Output command results as JSON |

## Commands

### Messaging

- `bramble send <address> <message>` — send a unicast message
- `bramble broadcast <message>` — send a mesh-wide message
- `bramble channels list` — list configured channels
- `bramble channels add <name> [psk]` — add a channel
- `bramble channels remove <index>` — remove a channel
- `bramble channels set-default <index>` — set default outgoing channel

```bash
bramble send CAFEBABE "hello there"
bramble broadcast "hello everyone"
bramble broadcast --channel 2 "hello channel 2"
bramble broadcast --wait-delivery 10 "delivery telemetry please"
```

### Monitoring and Diagnostics

- `bramble monitor` — stream real-time node events
- `bramble traffic monitor` — live TX/RX telemetry stream
- `bramble traffic export` — export ring-buffer traffic telemetry to JSONL
- `bramble peers` — list direct radio neighbors
- `bramble routes` — show routing table
- `bramble ping` — ping connected node
- `bramble probe` — send network probe

```bash
bramble monitor --topic wifi,gps,location
bramble monitor --messages
bramble traffic monitor --tx-only
bramble traffic export --format jsonl > traffic-events.jsonl
```

### Configuration

- `bramble config get` — print full node configuration
- `bramble config set-name <name>` — set node display name
- `bramble config set-radio` — update radio parameters

```bash
bramble config set-name my-node
bramble config set-radio --freq 915.0 --sf 10 --bw 125 --cr 5 --txpower 20
```

### Location

- `bramble location status` — show known peer locations
- `bramble location get-config` — show canonical location config
- `bramble location set-config` — set canonical location policy
- `bramble location set-contact <address> <tier>` — quick per-peer rule
- `bramble location remove-contact <address>` — remove per-peer rule
- `bramble location share-once <address>` — send one-time location update

```bash
bramble location set-config --enabled --default-tier full --interval-s 30 --source gps
bramble location get-config --json
```

### System and Network

- `bramble status` — show node address, firmware, radio, peers, counters, uptime
- `bramble discover` — scan local network for Bramble nodes via mDNS
- `bramble ota --url <url>` — trigger OTA firmware update
- `bramble reboot` — reboot node
- `bramble tui` — launch full-screen interactive terminal UI

```bash
bramble status
bramble discover --timeout 10s
bramble ota --url http://192.0.2.0:8080/firmware/bramble.bin
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

## JSON Output

All commands support `--json` for machine-readable output:

```bash
bramble --json status | jq .address
bramble --json peers | jq '.[].rssi'
bramble monitor --topic location --json
```

## Examples

See the [`examples/`](examples/) directory for common usage patterns:

- [`01-connect.sh`](examples/01-connect.sh) — BLE, WiFi, and serial connection flows
- [`02-send-receive.sh`](examples/02-send-receive.sh) — basic send/receive
- [`03-channels.sh`](examples/03-channels.sh) — channel operations
- [`04-location.sh`](examples/04-location.sh) — location sharing
- [`05-monitor.sh`](examples/05-monitor.sh) — monitor and debug output

## License

MIT
