#!/usr/bin/env bash
# 01-connect.sh — BLE vs WiFi vs Serial connection examples
#
# Bramble CLI supports three transport modes. This script shows each.
# Run with: bash examples/01-connect.sh

set -euo pipefail

echo "=== Auto-detect USB serial (scans /dev/ttyUSB* and /dev/ttyACM*) ==="
bramble status
# Expected output (example):
#   Address:    CAFEBABE
#   Name:       my-node
#   Firmware:   0.9.2
#   Uptime:     1h23m
#   Peers:      3
#   Radio:      915.0 MHz SF10 BW125 CR5 TxPower 20 dBm

echo
echo "=== Explicit serial port ==="
bramble --port /dev/ttyUSB0 status

echo
echo "=== WiFi / WebSocket transport (node in AP mode) ==="
bramble --transport ws://192.168.4.1/ws status
# The node creates a WiFi AP named "Bramble" by default.
# Connect your machine to that AP, then use the gateway IP.

echo
echo "=== WiFi / WebSocket transport (node on your LAN) ==="
# Use bramble discover to find nodes advertising via mDNS
bramble discover
# Expected output (example):
#   Found: Bramble-CAFEBABE at ws://192.0.2.0:8080/ws

# Then connect directly:
# bramble --transport ws://192.0.2.0:8080/ws status

echo
echo "=== Bluetooth Low Energy (BLE) ==="
bramble --ble Bramble status
# Scans for a BLE device advertising "Bramble" and connects.
# Use the exact advertised name; check node config if different.

echo
echo "=== JSON output (any transport) ==="
bramble --json status | jq '{address: .address, firmware: .firmware, peers: (.peers | length)}'
# Expected output (example):
# {
#   "address": "CAFEBABE",
#   "firmware": "0.9.2",
#   "peers": 3
# }
