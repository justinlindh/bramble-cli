#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN="${BIN:-./bramble}"
README="README.md"

if [[ ! -x "$BIN" ]]; then
  echo "error: $BIN not found or not executable" >&2
  echo "hint: go build -o bramble ./cmd/bramble" >&2
  exit 1
fi

failures=0

check_help_flag() {
  local cmd="$1"
  local flag="$2"
  local out
  local -a cmd_parts=()
  if [[ -n "$cmd" ]]; then
    read -r -a cmd_parts <<<"$cmd"
  fi
  out="$("$BIN" "${cmd_parts[@]}" --help 2>/dev/null || true)"
  if ! grep -Fq -- "$flag" <<<"$out"; then
    echo "[FAIL] '$BIN $cmd --help' missing: $flag"
    failures=$((failures + 1))
  else
    echo "[OK]   $cmd includes $flag"
  fi
}

check_readme_text() {
  local text="$1"
  if ! grep -Fq -- "$text" "$README"; then
    echo "[FAIL] README missing text: $text"
    failures=$((failures + 1))
  else
    echo "[OK]   README mentions: $text"
  fi
}

# Help-surface checks against current CLI behavior.
check_help_flag "" "--ble"
check_help_flag "" "--transport"
check_help_flag "broadcast" "--wait-delivery"
check_help_flag "monitor" "--events"
check_help_flag "traffic monitor" "--category"
check_help_flag "traffic monitor" "--tx-only"
check_help_flag "traffic export" "--since"
check_help_flag "traffic export" "--limit"

# README coverage checks for critical command snippets.
check_readme_text "bramble --ble Bramble status"
check_readme_text "bramble broadcast --wait-delivery"
check_readme_text "bramble monitor --events"
check_readme_text "bramble traffic monitor"
check_readme_text "bramble traffic export"

if (( failures > 0 )); then
  echo
  echo "Doc drift detected: $failures check(s) failed."
  exit 1
fi

echo
echo "Doc drift check passed."
