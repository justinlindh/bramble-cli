#!/usr/bin/env bash
# Companion for tui-demo.tape: the second bench node replies while the TUI
# records on the first, so incoming traffic appears live in the demo.
# Usage: run in the background just before launching the TUI.
PORT="${BRAMBLE_DEMO_PEER_PORT:-/dev/ttyACM1}"
CLI="$(dirname "$0")/../../bramble"
sleep 14
"$CLI" broadcast --port "$PORT" "Loud and clear from the ridge repeater" >/dev/null 2>&1
sleep 5
"$CLI" broadcast --port "$PORT" "Good signal tonight, seeing you at 2 hops" >/dev/null 2>&1
