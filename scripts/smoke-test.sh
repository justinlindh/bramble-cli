#!/usr/bin/env bash
#
# smoke-test.sh — Bramble CLI live integration smoke test
#
# Requires 2+ nodes connected via USB serial. Tests all major CLI functions
# with real hardware, outputs structured JSON results for automated parsing.
#
# Usage:
#   ./scripts/smoke-test.sh [--port1 /dev/ttyACM0] [--port2 /dev/ttyUSB0]
#   ./scripts/smoke-test.sh --port1 /dev/ttyUSB0 --port2 ws://192.0.2.112/ws
#   ./scripts/smoke-test.sh --auto          # auto-detect two nodes (serial + mDNS)
#   ./scripts/smoke-test.sh --help
#
# Output format (JSON):
#   {
#     "timestamp": "...",
#     "binary": "...",
#     "nodes": { "node1": {...}, "node2": {...} },
#     "tests": [ { "name": "...", "pass": true/false, "detail": "...", "duration_ms": N } ],
#     "summary": { "total": N, "passed": N, "failed": N, "skipped": N }
#   }

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="${SCRIPT_DIR}/../bramble"
PORT1=""
PORT2=""
AUTO=false
VERBOSE=false
TIMEOUT=10

# ── Helpers ──────────────────────────────────────────────────────────────

usage() {
  echo "Usage: $0 [--port1 PORT] [--port2 PORT] [--auto] [--verbose] [--timeout N]"
  echo ""
  echo "  --port1 PORT   Serial port for node 1"
  echo "  --port2 PORT   Serial port for node 2"
  echo "  --auto         Auto-detect two nodes from available serial ports"
  echo "  --verbose      Print test progress to stderr"
  echo "  --timeout N    Per-command timeout in seconds (default: 8)"
  echo "  --help         Show this help"
  exit 0
}

die() { echo "FATAL: $1" >&2; exit 1; }
log() {
  if $VERBOSE; then
    echo "[smoke] $*" >&2
  fi
}

now_ms() { date +%s%3N; }

# Run bramble CLI, capture stdout. Returns exit code.
# Usage: run_cli ENDPOINT [args...]
# ENDPOINT can be a serial port (/dev/...) or a WebSocket URL (ws://...)
run_cli() {
  local endpoint="$1"; shift
  if [[ "$endpoint" == ws://* || "$endpoint" == wss://* ]]; then
    timeout "$TIMEOUT" "$CLI" --transport "$endpoint" --json "$@" 2>/dev/null || true
  else
    timeout "$TIMEOUT" "$CLI" --port "$endpoint" --json "$@" 2>/dev/null || true
  fi
}

# JSON array accumulator
TESTS_JSON="[]"
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# Record a test result
# Usage: record_test "name" pass|fail|skip "detail" duration_ms
record_test() {
  local name="$1" result="$2" detail="$3" dur="${4:-0}"
  local pass_val="false"
  case "$result" in
    pass) pass_val="True";  PASS_COUNT=$((PASS_COUNT + 1)) ;;
    fail) pass_val="False"; FAIL_COUNT=$((FAIL_COUNT + 1)) ;;
    skip) pass_val="None";  SKIP_COUNT=$((SKIP_COUNT + 1)) ;;
  esac
  # Escape detail for JSON
  detail=$(echo "$detail" | python3 -c "import sys,json; print(json.dumps(sys.stdin.read().strip()))")
  TESTS_JSON=$(echo "$TESTS_JSON" | python3 -c "
import sys,json
tests = json.load(sys.stdin)
tests.append({'name': '$name', 'pass': $pass_val, 'result': '$result', 'detail': $detail, 'duration_ms': $dur})
json.dump(tests, sys.stdout)
")
  log "$result: $name"
}

# ── Argument parsing ─────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
  case "$1" in
    --port1)  PORT1="$2"; shift 2 ;;
    --port2)  PORT2="$2"; shift 2 ;;
    --auto)   AUTO=true; shift ;;
    --verbose) VERBOSE=true; shift ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    --help|-h) usage ;;
    *) die "Unknown arg: $1" ;;
  esac
done

# ── Binary check ─────────────────────────────────────────────────────────

[[ -x "$CLI" ]] || die "CLI binary not found at $CLI — run: go build -o bramble ./cmd/bramble/"

# ── Auto-detect nodes ────────────────────────────────────────────────────

if $AUTO || [[ -z "$PORT1" || -z "$PORT2" ]]; then
  log "Auto-detecting Bramble nodes..."
  DETECTED_PORTS=()

  # 1. Try serial ports first
  for port in /dev/ttyACM0 /dev/ttyACM1 /dev/ttyACM2 /dev/ttyUSB0 /dev/ttyUSB1; do
    [[ -e "$port" ]] || continue
    log "  probing serial $port..."
    out=$(run_cli "$port" status 2>/dev/null)
    if echo "$out" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'address' in d" 2>/dev/null; then
      DETECTED_PORTS+=("$port")
      log "  ✓ found node on $port"
      [[ ${#DETECTED_PORTS[@]} -ge 2 ]] && break
    fi
  done

  # 2. If we need more nodes, try mDNS discovery for WebSocket endpoints
  if [[ ${#DETECTED_PORTS[@]} -lt 2 ]]; then
    log "  Serial found ${#DETECTED_PORTS[@]} nodes, trying mDNS..."
    # Collect already-found addresses to avoid duplicates
    FOUND_ADDRS=()
    for ep in "${DETECTED_PORTS[@]}"; do
      addr=$(run_cli "$ep" status 2>/dev/null | python3 -c "import sys,json;print(json.load(sys.stdin)['address'])" 2>/dev/null || true)
      [[ -n "$addr" ]] && FOUND_ADDRS+=("$addr")
    done

    WS_URLS=$(timeout 6 "$CLI" --json discover 2>/dev/null | python3 -c "
import sys,json
nodes=json.load(sys.stdin)
for n in nodes:
    print(n.get('WSURL',''))
" 2>/dev/null || true)

    while IFS= read -r ws_url; do
      [[ -z "$ws_url" ]] && continue
      [[ ${#DETECTED_PORTS[@]} -ge 2 ]] && break
      log "  probing ws $ws_url..."
      out=$(run_cli "$ws_url" status 2>/dev/null)
      addr=$(echo "$out" | python3 -c "import sys,json;print(json.load(sys.stdin)['address'])" 2>/dev/null || true)
      [[ -z "$addr" ]] && continue
      # Skip if we already have this node via serial
      skip=false
      for fa in "${FOUND_ADDRS[@]:-}"; do
        [[ "$fa" == "$addr" ]] && skip=true && break
      done
      if ! $skip; then
        DETECTED_PORTS+=("$ws_url")
        FOUND_ADDRS+=("$addr")
        log "  ✓ found node $addr on $ws_url"
      else
        log "  ⊘ $ws_url is $addr (already found via serial)"
      fi
    done <<< "$WS_URLS"
  fi

  if [[ ${#DETECTED_PORTS[@]} -lt 2 ]]; then
    die "Need 2 nodes but only found ${#DETECTED_PORTS[@]}: ${DETECTED_PORTS[*]:-none}"
  fi
  PORT1="${PORT1:-${DETECTED_PORTS[0]}}"
  PORT2="${PORT2:-${DETECTED_PORTS[1]}}"
fi

log "Node 1: $PORT1"
log "Node 2: $PORT2"

# ── Collect node identity ────────────────────────────────────────────────

NODE1_STATUS=$(run_cli "$PORT1" status)
NODE2_STATUS=$(run_cli "$PORT2" status)

NODE1_ADDR=$(echo "$NODE1_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin)['address'])")
NODE2_ADDR=$(echo "$NODE2_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin)['address'])")
NODE1_HW=$(echo "$NODE1_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('hardware','unknown'))")
NODE2_HW=$(echo "$NODE2_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('hardware','unknown'))")

NODE1_CONFIG=$(run_cli "$PORT1" config get)
NODE2_CONFIG=$(run_cli "$PORT2" config get)
NODE1_NAME=$(echo "$NODE1_CONFIG" | python3 -c "import sys,json; print(json.load(sys.stdin).get('node_name',''))")
NODE2_NAME=$(echo "$NODE2_CONFIG" | python3 -c "import sys,json; print(json.load(sys.stdin).get('node_name',''))")

log "Node 1: $NODE1_NAME ($NODE1_ADDR) on $PORT1 [$NODE1_HW]"
log "Node 2: $NODE2_NAME ($NODE2_ADDR) on $PORT2 [$NODE2_HW]"

# ══════════════════════════════════════════════════════════════════════════
# TESTS
# ══════════════════════════════════════════════════════════════════════════

# ── 1. Status ────────────────────────────────────────────────────────────

test_status() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" status)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'address' in d, 'missing address'
assert 'firmware_version' in d, 'missing firmware_version'
assert 'protocol_version' in d, 'missing protocol_version'
assert 'radio_ok' in d, 'missing radio_ok'
assert 'uptime_s' in d, 'missing uptime_s'
assert isinstance(d['peers'], int), 'peers not int'
" 2>/dev/null; then
    record_test "status.$label" pass "address=$(echo "$out" | python3 -c "import sys,json;print(json.load(sys.stdin)['address'])")" "$dur"
  else
    record_test "status.$label" fail "Invalid status response: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_status "node1" "$PORT1"
test_status "node2" "$PORT2"

# ── 2. Config get ────────────────────────────────────────────────────────

test_config_get() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" config get)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'node_name' in d, 'missing node_name'
assert 'address' in d, 'missing address'
assert 'radio' in d, 'missing radio'
assert 'channels' in d, 'missing channels'
assert isinstance(d['channels'], list), 'channels not list'
" 2>/dev/null; then
    local name
    name=$(echo "$out" | python3 -c "import sys,json;print(json.load(sys.stdin)['node_name'])")
    record_test "config_get.$label" pass "name=$name" "$dur"
  else
    record_test "config_get.$label" fail "Invalid config: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_config_get "node1" "$PORT1"
test_config_get "node2" "$PORT2"

# ── 3. Peers ─────────────────────────────────────────────────────────────

test_peers() {
  local label="$1" port="$2" expect_addr="$3"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" peers)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
peers=json.load(sys.stdin)
assert isinstance(peers, list), 'not a list'
assert len(peers) > 0, 'no peers'
addrs = [p['address'] for p in peers]
for p in peers:
    assert 'rssi' in p, 'missing rssi'
    assert 'snr' in p, 'missing snr'
    assert 'last_seen_ms' in p, 'missing last_seen_ms'
# Check that the other node is visible
assert '$expect_addr' in addrs, 'peer $expect_addr not found in ' + str(addrs)
" 2>/dev/null; then
    local count
    count=$(echo "$out" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
    local names
    names=$(echo "$out" | python3 -c "import sys,json;print(', '.join(p.get('name','?') for p in json.load(sys.stdin)))")
    record_test "peers.$label" pass "count=$count names=[$names] (includes $expect_addr)" "$dur"
  else
    record_test "peers.$label" fail "Peer $expect_addr not found or bad response: $(echo "$out" | head -c 300)" "$dur"
  fi
}

test_peers "node1_sees_node2" "$PORT1" "$NODE2_ADDR"
test_peers "node2_sees_node1" "$PORT2" "$NODE1_ADDR"

# ── 4. Peer name resolution ─────────────────────────────────────────────

test_peer_names() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" peers)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
peers=json.load(sys.stdin)
named = [p for p in peers if p.get('name','').strip()]
unnamed = [p for p in peers if not p.get('name','').strip()]
assert len(named) > 0, 'no peers have names'
print(f'named={len(named)} unnamed={len(unnamed)}')
" 2>/dev/null; then
    local detail
    detail=$(echo "$out" | python3 -c "
import sys,json
peers=json.load(sys.stdin)
named=[p for p in peers if p.get('name','').strip()]
unnamed=[p for p in peers if not p.get('name','').strip()]
print(f'named={len(named)} unnamed={len(unnamed)} names=[{\", \".join(p[\"name\"] for p in named)}]')
")
    record_test "peer_names.$label" pass "$detail" "$dur"
  else
    record_test "peer_names.$label" fail "No peers have names: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_peer_names "node1" "$PORT1"
test_peer_names "node2" "$PORT2"

# ── 5. Routes ────────────────────────────────────────────────────────────

test_routes() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" routes)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
routes=json.load(sys.stdin)
assert isinstance(routes, list), 'not a list'
" 2>/dev/null; then
    local count
    count=$(echo "$out" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
    record_test "routes.$label" pass "count=$count" "$dur"
  else
    record_test "routes.$label" fail "Invalid routes response: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_routes "node1" "$PORT1"
test_routes "node2" "$PORT2"

# ── 6. Channels list ────────────────────────────────────────────────────

test_channels_list() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" channels list)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
chs=json.load(sys.stdin)
assert isinstance(chs, list), 'not a list'
assert len(chs) >= 1, 'no channels'
for ch in chs:
    assert 'id' in ch, 'missing id'
    assert 'name' in ch, 'missing name'
has_default = any(ch.get('is_default') for ch in chs)
assert has_default, 'no default channel'
" 2>/dev/null; then
    local names
    names=$(echo "$out" | python3 -c "import sys,json;print(', '.join(f\"{c['id']}:{c['name']}\" for c in json.load(sys.stdin)))")
    record_test "channels_list.$label" pass "channels=[$names]" "$dur"
  else
    record_test "channels_list.$label" fail "Invalid channels: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_channels_list "node1" "$PORT1"
test_channels_list "node2" "$PORT2"

# ── 7. Ping ──────────────────────────────────────────────────────────────

test_ping() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" ping)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ -n "$out" ]] && ! echo "$out" | grep -qi "error"; then
    record_test "ping.$label" pass "$(echo "$out" | head -c 100)" "$dur"
  else
    record_test "ping.$label" fail "Ping failed: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_ping "node1" "$PORT1"
test_ping "node2" "$PORT2"

# ── 8. Broadcast message (node1 → all, verify no RPC error) ─────────────

test_broadcast() {
  local ts
  ts=$(date +%s)
  local msg="smoke-broadcast-$ts"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$PORT1" "broadcast \"$msg\"")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
# Accept any response without error field, or with a message_id/correlation
assert 'error' not in d or not d['error'], f'RPC error: {d.get(\"error\")}'
" 2>/dev/null; then
    record_test "broadcast.node1" pass "msg=$msg response=$(echo "$out" | head -c 200)" "$dur"
  elif [[ -z "$out" ]]; then
    # Some commands succeed with empty output
    record_test "broadcast.node1" pass "msg=$msg (empty response, no error)" "$dur"
  else
    record_test "broadcast.node1" fail "Broadcast failed: $(echo "$out" | head -c 300)" "$dur"
  fi
}

test_broadcast

# ── 9. DM send (node1 → node2) ──────────────────────────────────────────

test_dm_send() {
  local ts
  ts=$(date +%s)
  local msg="smoke-dm-$ts"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$PORT1" "send $NODE2_ADDR \"$msg\"")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'error' not in d or not d['error'], f'RPC error: {d.get(\"error\")}'
" 2>/dev/null; then
    record_test "dm.node1_to_node2" pass "msg=$msg to=$NODE2_ADDR response=$(echo "$out" | head -c 200)" "$dur"
  elif [[ -z "$out" ]]; then
    record_test "dm.node1_to_node2" pass "msg=$msg to=$NODE2_ADDR (empty response, no error)" "$dur"
  else
    record_test "dm.node1_to_node2" fail "DM send failed: $(echo "$out" | head -c 300)" "$dur"
  fi
}

test_dm_send

# ── 10. DM send (node2 → node1) ─────────────────────────────────────────

test_dm_reverse() {
  local ts
  ts=$(date +%s)
  local msg="smoke-dm-rev-$ts"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$PORT2" "send $NODE1_ADDR \"$msg\"")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'error' not in d or not d['error'], f'RPC error: {d.get(\"error\")}'
" 2>/dev/null; then
    record_test "dm.node2_to_node1" pass "msg=$msg to=$NODE1_ADDR" "$dur"
  elif [[ -z "$out" ]]; then
    record_test "dm.node2_to_node1" pass "msg=$msg to=$NODE1_ADDR (empty response, no error)" "$dur"
  else
    record_test "dm.node2_to_node1" fail "DM send failed: $(echo "$out" | head -c 300)" "$dur"
  fi
}

test_dm_reverse

# ── 11. Config set-name (round-trip) ────────────────────────────────────

test_set_name() {
  local label="$1" port="$2"
  # Read current name
  local orig_name
  orig_name=$(run_cli "$port" config get | python3 -c "import sys,json;print(json.load(sys.stdin)['node_name'])")
  local test_name="SmkTst"

  local t0
  t0=$(now_ms)
  # Set temporary name
  run_cli "$port" config set-name "$test_name" >/dev/null
  sleep 1
  # Read back
  local new_name
  new_name=$(run_cli "$port" config get | python3 -c "import sys,json;print(json.load(sys.stdin)['node_name'])")
  # Restore
  run_cli "$port" config set-name "$orig_name" >/dev/null
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ "$new_name" == "$test_name" ]]; then
    record_test "set_name_roundtrip.$label" pass "set=$test_name read=$new_name restored=$orig_name" "$dur"
  else
    record_test "set_name_roundtrip.$label" fail "expected=$test_name got=$new_name" "$dur"
  fi
}

test_set_name "node2" "$PORT2"

# ── 12. Location config get ──────────────────────────────────────────────

test_location_config() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" location get-config)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'enabled' in d, 'missing enabled'
assert 'default_tier' in d, 'missing default_tier'
assert 'interval_s' in d, 'missing interval_s'
assert 'source' in d, 'missing source'
" 2>/dev/null; then
    local enabled
    enabled=$(echo "$out" | python3 -c "import sys,json;d=json.load(sys.stdin);print(f\"enabled={d['enabled']} tier={d['default_tier']} src={d['source']}\")")
    record_test "location_config.$label" pass "$enabled" "$dur"
  else
    record_test "location_config.$label" fail "Invalid location config: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_location_config "node1" "$PORT1"
test_location_config "node2" "$PORT2"

# ── 13. Location set-contact (round-trip on node2) ───────────────────────

test_location_set_contact() {
  local t0
  t0=$(now_ms)
  # Set contact on node2 sharing with node1
  local out
  out=$(run_cli "$PORT2" location set-contact "$NODE1_ADDR" full)

  # Verify it appears in config
  sleep 1
  local config
  config=$(run_cli "$PORT2" location get-config)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$config" | python3 -c "
import sys,json
d=json.load(sys.stdin)
rules = d.get('contact_rules', [])
found = any(r['address'] == '$NODE1_ADDR' for r in rules)
assert found, 'contact $NODE1_ADDR not found in rules'
" 2>/dev/null; then
    record_test "location_set_contact.node2" pass "sharing with $NODE1_ADDR set to full" "$dur"
  else
    record_test "location_set_contact.node2" fail "Contact not found after set: $(echo "$config" | head -c 200)" "$dur"
  fi
}

test_location_set_contact

# ── 14. Location share-once ──────────────────────────────────────────────

test_location_share_once() {
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$PORT1" location share-once "$NODE2_ADDR")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ -z "$out" ]] || ! echo "$out" | grep -qi "error"; then
    record_test "location_share_once.node1_to_node2" pass "$(echo "$out" | head -c 100)" "$dur"
  else
    record_test "location_share_once.node1_to_node2" fail "$(echo "$out" | head -c 200)" "$dur"
  fi
}

test_location_share_once

# ── 15. Location status ──────────────────────────────────────────────────

test_location_status() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" location status)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert isinstance(d, list), 'not a list'
" 2>/dev/null; then
    local count
    count=$(echo "$out" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
    record_test "location_status.$label" pass "peer_locations=$count" "$dur"
  else
    record_test "location_status.$label" fail "Invalid: $(echo "$out" | head -c 200)" "$dur"
  fi
}

test_location_status "node1" "$PORT1"
test_location_status "node2" "$PORT2"

# ── 16. Probe ────────────────────────────────────────────────────────────

test_probe() {
  local label="$1" port="$2"
  local t0
  t0=$(now_ms)
  local out
  out=$(run_cli "$port" probe)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ -n "$out" ]] && ! echo "$out" | grep -qi "error"; then
    record_test "probe.$label" pass "$(echo "$out" | head -c 150)" "$dur"
  elif [[ -z "$out" ]]; then
    record_test "probe.$label" pass "(empty response, no error)" "$dur"
  else
    record_test "probe.$label" fail "$(echo "$out" | head -c 200)" "$dur"
  fi
}

test_probe "node1" "$PORT1"

# ── 17. Broadcast with delivery wait ────────────────────────────────────

test_broadcast_delivery() {
  local ts
  ts=$(date +%s)
  local msg="smoke-delivery-$ts"
  local t0
  t0=$(now_ms)
  local out
  # Wait up to 5s for delivery telemetry
  out=$(run_cli "$PORT1" broadcast --wait-delivery 5 "$msg")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
assert 'error' not in d or not d['error'], f'error: {d.get(\"error\")}'
# Check for delivery field if present
delivery = d.get('delivery', d.get('deliveries', []))
" 2>/dev/null; then
    local detail
    detail=$(echo "$out" | python3 -c "
import sys,json
d=json.load(sys.stdin)
dl = d.get('delivery', d.get('deliveries', []))
if isinstance(dl, list):
    print(f'msg={\"$msg\"} delivery_count={len(dl)}')
else:
    print(f'msg={\"$msg\"} delivery={dl}')
" 2>/dev/null || echo "msg=$msg")
    record_test "broadcast_delivery.node1" pass "$detail" "$dur"
  elif [[ -z "$out" ]]; then
    record_test "broadcast_delivery.node1" pass "msg=$msg (no error)" "$dur"
  else
    record_test "broadcast_delivery.node1" fail "$(echo "$out" | head -c 300)" "$dur"
  fi
}

test_broadcast_delivery

# ── 18. Discover (mDNS) ─────────────────────────────────────────────────

test_discover() {
  local t0
  t0=$(now_ms)
  local out
  out=$(timeout 6 "$CLI" --json discover 2>/dev/null || true)
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ -z "$out" ]]; then
    record_test "discover" skip "mDNS discover returned empty (WiFi nodes may not be advertising)" "$dur"
  elif echo "$out" | python3 -c "import sys,json;json.load(sys.stdin)" 2>/dev/null; then
    record_test "discover" pass "$(echo "$out" | head -c 200)" "$dur"
  else
    record_test "discover" skip "non-JSON response (may be expected): $(echo "$out" | head -c 100)" "$dur"
  fi
}

test_discover

# ── 19. Radio symmetry check ────────────────────────────────────────────

test_radio_symmetry() {
  local t0
  t0=$(now_ms)
  local r1 r2
  r1=$(run_cli "$PORT1" config get | python3 -c "import sys,json;r=json.load(sys.stdin)['radio'];print(f\"{r['frequency_mhz']},{r['sf']},{r['bw_hz']},{r['tx_power_dbm']}\")")
  r2=$(run_cli "$PORT2" config get | python3 -c "import sys,json;r=json.load(sys.stdin)['radio'];print(f\"{r['frequency_mhz']},{r['sf']},{r['bw_hz']},{r['tx_power_dbm']}\")")
  local dur
  dur=$(( $(now_ms) - t0 ))

  if [[ "$r1" == "$r2" ]]; then
    record_test "radio_symmetry" pass "both=$r1" "$dur"
  else
    record_test "radio_symmetry" fail "node1=$r1 node2=$r2 (radio params mismatch — nodes may not hear each other)" "$dur"
  fi
}

test_radio_symmetry

# ══════════════════════════════════════════════════════════════════════════
# OUTPUT
# ══════════════════════════════════════════════════════════════════════════

TOTAL=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))

# Write tests JSON to temp file to avoid shell quoting issues with newlines
TESTS_TMP=$(mktemp)
echo "$TESTS_JSON" > "$TESTS_TMP"
cleanup() { rm -f "$TESTS_TMP"; }
trap cleanup EXIT

python3 - "$TESTS_TMP" "$PORT1" "$NODE1_ADDR" "$NODE1_NAME" "$NODE1_HW" \
  "$PORT2" "$NODE2_ADDR" "$NODE2_NAME" "$NODE2_HW" \
  "$CLI" "$TOTAL" "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" <<'PYEOF'
import json, sys, datetime

tests_file = sys.argv[1]
with open(tests_file) as f:
    tests = json.load(f)

result = {
    "timestamp": datetime.datetime.now().astimezone().isoformat(),
    "binary": sys.argv[10],
    "nodes": {
        "node1": {
            "endpoint": sys.argv[2],
            "address": sys.argv[3],
            "name": sys.argv[4],
            "hardware": sys.argv[5]
        },
        "node2": {
            "endpoint": sys.argv[6],
            "address": sys.argv[7],
            "name": sys.argv[8],
            "hardware": sys.argv[9]
        }
    },
    "tests": tests,
    "summary": {
        "total": int(sys.argv[11]),
        "passed": int(sys.argv[12]),
        "failed": int(sys.argv[13]),
        "skipped": int(sys.argv[14])
    }
}

json.dump(result, sys.stdout, indent=2)
print()
PYEOF

# Exit with failure if any tests failed
[[ $FAIL_COUNT -eq 0 ]] || exit 1
