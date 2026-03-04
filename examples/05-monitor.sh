#!/usr/bin/env bash
# 05-monitor.sh — Monitor and debug output patterns
#
# bramble monitor streams live node events. This script shows
# filtering, grep, JSON output, and traffic debug patterns.
# Run with: bash examples/05-monitor.sh

set -euo pipefail

echo "=== Monitor all events (Ctrl+C to stop) ==="
# bramble monitor

echo
echo "=== Monitor specific event types (legacy filter) ==="
bramble monitor --events message,ack &
BGPID=$!
sleep 5
kill $BGPID 2>/dev/null || true
# Expected output (example):
#   [10:42:01] message  from=DEADBEEF  to=CAFEBABE  ch=0  "hey"
#   [10:42:02] ack      for=a1b2c3d4   from=DEADBEEF

echo
echo "=== Monitor by topic ==="
bramble monitor --topic gps,location &
BGPID=$!
sleep 5
kill $BGPID 2>/dev/null || true
# Expected output (example):
#   [10:42:10] gps      lat=45.1234  lon=-93.5678  alt=312
#   [10:42:40] location from=DEADBEEF  lat=45.2222  lon=-93.4444

echo
echo "=== Monitor with grep filter ==="
bramble monitor --topic traffic --grep route &
BGPID=$!
sleep 5
kill $BGPID 2>/dev/null || true

echo
echo "=== Monitor with JSON output (pipe to jq) ==="
timeout 10 bramble monitor --topic message --json \
  | jq '{time: .timestamp, from: .from, text: .text}' || true

echo
echo "=== Monitor: stop after first match (--follow=false) ==="
bramble monitor --follow=false --topic gps
# Exits after printing the first matching GPS event.

echo
echo "=== Monitor with a time window hint ==="
bramble monitor --since 30s --topic location &
BGPID=$!
sleep 5
kill $BGPID 2>/dev/null || true

echo
echo "=== Traffic debug: live TX/RX telemetry ==="
bramble traffic monitor &
BGPID=$!
sleep 10
kill $BGPID 2>/dev/null || true
# Expected output (example):
#   TX  ch=0  sf=10  bw=125  size=45  rssi=-90  snr=7.2
#   RX  ch=0  sf=10  bw=125  size=32  rssi=-87  snr=8.1  from=DEADBEEF

echo
echo "=== Traffic debug: TX only ==="
timeout 10 bramble traffic monitor --tx-only || true

echo
echo "=== Traffic debug: RX routing category only ==="
timeout 10 bramble traffic monitor --rx-only --category routing || true

echo
echo "=== Traffic debug: export ring-buffer to JSONL ==="
bramble traffic export --format jsonl > "/tmp/bramble-traffic-$(date +%s).jsonl"
echo "Exported to /tmp/bramble-traffic-*.jsonl"

echo
echo "=== Traffic debug: export with cursor and limit ==="
# Get last 50 events after sequence 12345
bramble traffic export --since 12345 --limit 50 --format jsonl | jq '{seq: .seq, dir: .dir, size: .size}'

echo
echo "=== Node health: check routing table ==="
bramble routes
# Expected output (example):
#   Destination  NextHop    Hops  Quality
#   DEADBEEF     DEADBEEF   1     95
#   AABBCCDD     DEADBEEF   2     72

echo
echo "=== Ping the connected node ==="
bramble ping
# Expected output:
#   Pong from CAFEBABE in 12ms
