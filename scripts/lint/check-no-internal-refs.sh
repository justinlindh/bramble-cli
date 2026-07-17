#!/usr/bin/env bash
# Fail if the tracked tree leaks internal infrastructure references.
#
# Blocks: internal git host names, absolute developer home paths, and private
# project code names, plus any private 192.168.x.x LAN address. The ESP32
# SoftAP range 192.168.4.x is a documented device address and is allowed.
#
# Markdown IS scanned on purpose: a public repository's docs must never carry
# these references either. Only this script is skipped, because it necessarily
# contains the very patterns it searches for (a self-match).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# This script holds the search patterns verbatim, so it must exclude itself.
SELF="scripts/lint/check-no-internal-refs.sh"

FORBIDDEN='example|/home/user|host|internal-planning'

status=0

# 1) Literal internal tokens (case-insensitive), scanning every tracked file.
if git grep -nIiE "$FORBIDDEN" -- ":!$SELF"; then
  echo "ERROR: forbidden internal reference(s) found above." >&2
  status=1
fi

# 2) Private 192.168.x.x addresses, excluding the allowed SoftAP 192.168.4.x.
leaked_ips=$(git grep -nIE '192\.168\.[0-9]+\.[0-9]+' -- ":!$SELF" | grep -vE '192\.168\.4\.' || true)
if [ -n "$leaked_ips" ]; then
  printf '%s\n' "$leaked_ips" >&2
  echo "ERROR: non-SoftAP private 192.168.x.x address(es) found above." >&2
  status=1
fi

if [ "$status" -eq 0 ]; then
  echo "check-no-internal-refs: clean"
fi

exit "$status"
