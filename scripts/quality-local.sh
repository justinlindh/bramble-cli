#!/usr/bin/env bash
# quality-local.sh — Run the same quality gates locally that CI enforces.
# Usage: bash scripts/quality-local.sh [--fix]
#
# Prerequisites (installed automatically if missing):
#   - golangci-lint v2+
#   - govulncheck
#   - shellcheck
#   - markdownlint-cli2

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

FIX_FLAG=""
if [[ "${1:-}" == "--fix" ]]; then
  FIX_FLAG="--fix"
fi

PASS=0
FAIL=0
SKIP=0

run_step() {
  local name="$1"
  shift
  printf "\n\033[1;34m▶ %s\033[0m\n" "$name"
  if "$@"; then
    printf "\033[1;32m  ✓ %s passed\033[0m\n" "$name"
    PASS=$((PASS + 1))
  else
    printf "\033[1;31m  ✗ %s failed\033[0m\n" "$name"
    FAIL=$((FAIL + 1))
  fi
}

skip_step() {
  local name="$1"
  local reason="$2"
  printf "\n\033[1;33m▶ %s — SKIPPED (%s)\033[0m\n" "$name" "$reason"
  SKIP=$((SKIP + 1))
}

# ---------------------------------------------------------------------------
# 1. Go test
# ---------------------------------------------------------------------------
run_step "Go test" go test ./...

# ---------------------------------------------------------------------------
# 2. Go vet
# ---------------------------------------------------------------------------
run_step "Go vet" go vet ./...

# ---------------------------------------------------------------------------
# 3. Go lint (golangci-lint)
# ---------------------------------------------------------------------------
if command -v golangci-lint >/dev/null 2>&1; then
  if [[ -n "$FIX_FLAG" ]]; then
    run_step "Go lint" golangci-lint run --fix ./...
  else
    run_step "Go lint" golangci-lint run ./...
  fi
else
  skip_step "Go lint" "golangci-lint not found — install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
fi

# ---------------------------------------------------------------------------
# 4. govulncheck
# ---------------------------------------------------------------------------
if command -v govulncheck >/dev/null 2>&1; then
  run_step "Vuln check" govulncheck ./...
else
  skip_step "Vuln check" "govulncheck not found — install: go install golang.org/x/vuln/cmd/govulncheck@latest"
fi

# ---------------------------------------------------------------------------
# 5. Shellcheck
# ---------------------------------------------------------------------------
if command -v shellcheck >/dev/null 2>&1; then
  if compgen -G "scripts/*.sh" >/dev/null 2>&1 || compgen -G "examples/*.sh" >/dev/null 2>&1; then
    run_step "Shellcheck" shellcheck scripts/*.sh examples/*.sh
  else
    skip_step "Shellcheck" "no scripts found"
  fi
else
  skip_step "Shellcheck" "shellcheck not found — install via package manager"
fi

# ---------------------------------------------------------------------------
# 6. Doc drift
# ---------------------------------------------------------------------------
if [[ -x bramble ]] || go build -o bramble ./cmd/bramble 2>/dev/null; then
  run_step "Doc drift" bash scripts/check-doc-drift.sh
  rm -f bramble
else
  skip_step "Doc drift" "could not build CLI binary"
fi

# ---------------------------------------------------------------------------
# 7. Markdown lint
# ---------------------------------------------------------------------------
if command -v markdownlint-cli2 >/dev/null 2>&1; then
  run_step "Markdown lint" markdownlint-cli2 "**/*.md"
else
  skip_step "Markdown lint" "markdownlint-cli2 not found — install: npm i -g markdownlint-cli2"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
printf "\n\033[1m━━━ Summary ━━━\033[0m\n"
printf "  \033[32m%d passed\033[0m" "$PASS"
[[ $SKIP -gt 0 ]] && printf "  \033[33m%d skipped\033[0m" "$SKIP"
[[ $FAIL -gt 0 ]] && printf "  \033[31m%d failed\033[0m" "$FAIL"
printf "\n"

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
