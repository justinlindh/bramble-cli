# bramble-cli

Command-line interface for [Bramble](https://github.com/justinlindh/bramble-go) LoRa mesh nodes.

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

## Connection

`bramble` connects to a node via USB serial or WebSocket.

**Auto-detect USB** (scans `/dev/ttyUSB*` and `/dev/ttyACM*`):
```bash
bramble status
```

**Specify a serial port:**
```bash
bramble --port /dev/ttyUSB0 status
```

**WebSocket transport** (e.g. ESP32 in AP mode):
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
Show node address, firmware, radio config, peer count, packet counters, and uptime.

```
$ bramble status
Address:   DEADBEEF
Pubkey:    A1B2C3D4
Firmware:  0.5.2
Protocol:  0.1.0
Hardware:  ESP32-S3
Uptime:    2h14m
Neighbors: 3
Routes:    7
TX/RX:     142 / 389  (dropped: 2)
Free heap: 214592 bytes
Airtime:   1240 ms used
```

### `bramble peers`
List direct radio neighbors with RSSI, SNR, and last-heard time.

```
ADDRESS   RSSI      SNR     LAST SEEN  MAILBOX  DR%
--------  --------  ------  ---------  -------  ---
CAFEBABE  -87 dBm   6.5 dB  4s                  95%
12345678  -102 dBm  3.0 dB  1m12s      yes       71%
```

### `bramble routes`
Show the mesh routing table.

```
DEST      NEXT HOP  HOPS  METRIC  STATE   LAST USED
--------  --------  ----  ------  ------  ---------
CAFEBABE  CAFEBABE  1     10      active  2s
AABBCCDD  CAFEBABE  2     22      active  45s
```

### `bramble ping`
Ping the connected node and confirm it responds.

```
$ bramble ping
Pong from DEADBEEF (protocol: 0.1.0)
```

### `bramble send <address> <message>`
Send a unicast message to a specific node.

```
$ bramble send CAFEBABE "hello there"
Sent: packet#42 → CAFEBABE
```

The address can be hex (`DEADBEEF`, `0xDEADBEEF`) or decimal.

### `bramble broadcast <message>`
Broadcast a message to all reachable nodes.

```
$ bramble broadcast "hello everyone"
Broadcast: packet#43
```

### `bramble monitor [--messages] [--neighbors]`
Stream real-time events from the node. Press `Ctrl+C` to stop.

```
$ bramble monitor
Monitoring node events... (Ctrl+C to stop)
[14:23:01] MSG CAFEBABE→DEADBEEF  "hey!"
[14:23:05] ACK  packet#42  status=delivered
[14:23:47] NEIGHBOR  table updated
```

Flags:
- `--messages` — only show incoming messages
- `--neighbors` — only show neighbor-change events

### `bramble config get`
Print the full node configuration (identity, radio, channels, location).

### `bramble config set-name <name>`
Set the node display name (max 8 characters).

```
$ bramble config set-name "base1"
Node name set to "base1"
```

### `bramble config set-radio`
Update radio parameters (any combination of flags).

```
$ bramble config set-radio --freq 915.0 --sf 10 --bw 125 --cr 5 --txpower 20
Radio parameters updated.
```

| Flag | Description |
|------|-------------|
| `--freq` | Frequency in MHz (e.g. 915.0) |
| `--sf` | Spreading factor (7–12) |
| `--bw` | Bandwidth in kHz (125, 250, 500) |
| `--cr` | Coding rate (5–8, meaning 4/5…4/8) |
| `--txpower` | TX power in dBm |

### `bramble channels list`
List all configured channels.

### `bramble channels add <name> [psk]`
Add a channel with an optional pre-shared key.

```
$ bramble channels add "team" "s3cr3t"
Channel "team" added at index 1
```

### `bramble channels remove <index>`
Remove a channel by its index.

### `bramble channels set-default <index>`
Set the default channel for outgoing messages.

### `bramble probe`
Send a network probe and print the probe ID. Responses appear in `bramble monitor`.

```
$ bramble probe
Probe sent: ID=7  ack_window=5000ms
Use 'bramble monitor' to see responses as they arrive.
```

### `bramble reboot`
Trigger a software reboot of the node.

```
$ bramble reboot
Node rebooting...
```

### `bramble location status`
Show location data for all known peers.

### `bramble location set-contact <address> <tier>`
Configure location sharing with a peer. Tiers: `exact`, `city`, `region`.

```
$ bramble location set-contact CAFEBABE exact
Location contact CAFEBABE set to tier "exact"
```

### `bramble location remove-contact <address>`
Stop sharing location with a peer.

### `bramble location share-once <address>`
Send a one-time location update to a peer.

## JSON Output

All commands support `--json` for machine-readable output:

```bash
bramble --json status | jq .address
bramble --json peers | jq '.[].rssi'
bramble --json monitor | jq -c .
```

## Private Module Note

`bramble-cli` depends on `github.com/justinlindh/bramble-go`, a private module.
The `go.mod` uses a `replace` directive to point to a local checkout.
Configure SSH access to the Gitea instance at `192.0.2.0:2222`.

## License

MIT
