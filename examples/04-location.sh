#!/usr/bin/env bash
# 04-location.sh — Location sharing
#
# Bramble supports tiered location sharing with per-peer contact rules.
# Tiers: none | coarse | full
# Run with: bash examples/04-location.sh

set -euo pipefail

PEER="DEADBEEF"  # Replace with peer node address

echo "=== Get location config from node ==="
bramble location get-config
# Expected output (example):
#   enabled:        true
#   default_tier:   coarse
#   interval_s:     60
#   source:         gps

echo
echo "=== Get location config (JSON) ==="
bramble --json location get-config | jq .
# Expected output:
# {
#   "enabled": true,
#   "default_tier": "coarse",
#   "interval_s": 60,
#   "source": "gps",
#   "contact_rules": []
# }

echo
echo "=== Enable location sharing with GPS, full tier, 30s interval ==="
bramble location set-config \
  --enabled \
  --default-tier full \
  --interval-s 30 \
  --source gps

echo
echo "=== Set per-peer contact rule (full precision for a specific peer) ==="
bramble location set-contact "$PEER" full
# Expected: Contact rule set for DEADBEEF tier=full

echo
echo "=== Set per-peer contact rule via set-config (batch/JSON style) ==="
bramble location set-config \
  --enabled \
  --default-tier coarse \
  --interval-s 60 \
  --source gps \
  --contact-rules "[{\"address\":\"$PEER\",\"enabled\":true,\"tier\":\"full\",\"interval_s\":30}]"

echo
echo "=== Send one-time location update to a peer ==="
bramble location share-once "$PEER"
# Sends current GPS fix to peer immediately, regardless of interval.

echo
echo "=== View location data for all known peers ==="
bramble location status
# Expected output (example):
#   DEADBEEF   lat=45.1234  lon=-93.5678  alt=312m  age=12s  tier=full
#   AABBCCDD   lat=45.2222  lon=-93.4444  alt=298m  age=5m   tier=coarse

echo
echo "=== View location status (JSON) ==="
bramble --json location status | jq '.[] | {address, lat, lon, tier}'

echo
echo "=== Remove a per-peer contact rule ==="
bramble location remove-contact "$PEER"

echo
echo "=== Monitor live location events ==="
echo "Listening for location events for 30 seconds..."
timeout 30 bramble monitor --topic location --json | jq '{address: .from, lat: .lat, lon: .lon}' || true
