# bramble-cli

Command-line interface for [Bramble](https://github.com/justinlindh/bramble) LoRa mesh nodes, built on [bramble-go](https://github.com/justinlindh/bramble-go).

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

## Connection

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
bramble --transport ws://192.168.4.1/rpc status
```

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--port` | `-p` | Serial port path |
| `--transport` | `-t` | WebSocket URL |
| `--json` | | Output as JSON |

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
```bash
bramble broadcast "hello everyone"
bramble broadcast --channel 2 "hello channel 2"
```

### `bramble monitor [--messages] [--neighbors]`
Stream real-time events. Press `Ctrl+C` to stop.
```bash
bramble monitor              # all events
bramble monitor --messages   # only messages
bramble monitor --neighbors  # only neighbor changes
```

### `bramble config get`
Print the full node configuration.

### `bramble config set-name <name>`
Set the node display name (max 8 characters).

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

### `bramble probe`
Send a network probe. Responses appear in `bramble monitor`.

### `bramble reboot`
Reboot the node.

### `bramble location status`
Show location data for all known peers.

### `bramble location set-contact <address> <tier>`
Configure location sharing (tiers: `exact`, `city`, `region`).

### `bramble location remove-contact <address>`
Stop sharing location with a peer.

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

## License

MIT
