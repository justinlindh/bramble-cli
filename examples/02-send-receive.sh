#!/usr/bin/env bash
# 02-send-receive.sh — Basic send and receive
#
# Shows unicast send, broadcast, delivery confirmation, and
# how to receive messages via monitor.
# Run with: bash examples/02-send-receive.sh

set -euo pipefail

TARGET="CAFEBABE"  # Replace with destination node address
MESSAGE="hello from CLI"

echo "=== Send unicast message ==="
bramble send "$TARGET" "$MESSAGE"
# Expected output:
#   Sent to CAFEBABE  packetId=a1b2c3d4  channel=0

echo
echo "=== Send unicast with JSON output ==="
bramble --json send "$TARGET" "$MESSAGE" | jq .
# Expected output:
# {
#   "packetId": "a1b2c3d4",
#   "channel": 0,
#   "fragmented": false,
#   "max_bytes": 230,
#   "actual_bytes": 17
# }

echo
echo "=== Broadcast to all mesh nodes ==="
bramble broadcast "hello everyone"
# Expected output:
#   Broadcast  broadcast_id=b5e6f7a8  channel=-1

echo
echo "=== Broadcast on a specific channel ==="
bramble broadcast --channel 2 "hello channel 2 only"

echo
echo "=== Broadcast with delivery telemetry (wait up to 10s) ==="
bramble broadcast --wait-delivery 10 "ping with ack"
# Waits for ACK/delivery events and prints telemetry.
# Expected output (example):
#   Broadcast  broadcast_id=b5e6f7a8  channel=-1
#   Delivery   from=DEADBEEF  rssi=-85  snr=8.5  hops=1

echo
echo "=== Receive messages (via monitor) ==="
echo "Listening for incoming messages for 30 seconds... (Ctrl+C to stop early)"
timeout 30 bramble monitor --events message || true
# Expected output (example):
#   [10:42:01] message  from=DEADBEEF  to=CAFEBABE  ch=0  "hi there"
#   [10:42:15] message  from=AABBCCDD  to=CAFEBABE  ch=0  "another message"
