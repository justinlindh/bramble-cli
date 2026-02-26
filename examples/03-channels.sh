#!/usr/bin/env bash
# 03-channels.sh — Channel operations
#
# Channels let you partition traffic. Channel 0 is always the public
# broadcast channel. Additional channels can have a PSK for privacy.
# Run with: bash examples/03-channels.sh

set -euo pipefail

echo "=== List channels ==="
bramble channels list
# Expected output (example):
#   0  broadcast   (public)
#   1  team        🔒
#   2  ops         🔒

echo
echo "=== List channels (JSON) ==="
bramble --json channels list | jq '.[] | {index, name, has_psk}'
# Expected output:
# {"index": 0, "name": "broadcast", "has_psk": false}
# {"index": 1, "name": "team",      "has_psk": true}

echo
echo "=== Add a public channel ==="
bramble channels add "alerts"
# Expected: Channel added at index 2

echo
echo "=== Add a private channel with PSK ==="
bramble channels add "ops" "mysecretkey"
# Expected: Channel added at index 3  (PSK set)

echo
echo "=== Set default outgoing channel ==="
bramble channels set-default 1
# All subsequent broadcasts/sends use channel 1 unless overridden.

echo
echo "=== Send on a specific channel ==="
bramble broadcast --channel 1 "team update: all good"

echo
echo "=== Monitor a specific channel ==="
echo "Listening on channel 1 for 20 seconds... (Ctrl+C to stop early)"
timeout 20 bramble monitor --topic message --grep '"channel":1' || true

echo
echo "=== Remove a channel ==="
bramble channels remove 3
# Removes channel at index 3.
# Note: channel 0 (broadcast) cannot be removed.
