# Quality Policy (Staged Enforcement)

This repository uses a staged quality rollout so we can tighten checks without blocking delivery on day one.

## Current State (Phase C)

CI workflow: `.github/workflows/quality.yml`

Recent hardening additions:
- workflow-level concurrency cancellation for superseded runs on the same ref
- per-job timeouts to prevent hung quality runs
- `workflow_dispatch` trigger for manual reruns

## Required vs Advisory Matrix

| Check | Phase A | Phase B | Phase C (current) |
|---|---|---|---|
| `go test ./...` | Required | Required | Required |
| `go vet ./...` | Required | Required | Required |
| `golangci-lint run ./...` | Advisory | Required | Required |
| `shellcheck scripts/*.sh examples/*.sh` | Advisory | Required | Required |
| `bash scripts/check-doc-drift.sh` | Advisory | Advisory | Required |
| `markdownlint-cli2 "**/*.md"` | Advisory | Advisory | Required |
| `govulncheck ./...` | Advisory | Advisory | Required |
| `actionlint` | Advisory | Advisory | Advisory (re-evaluate once workflow set stabilizes) |

### Required (blocking) in Phase C

- `go test ./...`
- `go vet ./...`
- `golangci-lint run ./...`
- `shellcheck scripts/*.sh examples/*.sh`
- `bash scripts/check-doc-drift.sh`
- `npx --yes markdownlint-cli2 "**/*.md"`
- `govulncheck ./...`

### Advisory (non-blocking) in Phase C

- `actionlint`

Advisory checks run with `continue-on-error: true` while we calibrate lower-signal checks.

## Migration Notes (Phase B → C)

- **What changed:** `scripts/check-doc-drift.sh`, `markdownlint-cli2`, and `govulncheck` moved from advisory to required gates.
- **Expected contributor impact:** PRs now fail on docs drift, markdown lint, or vulnerability scan violations in addition to existing Phase B required checks.
- **How to adapt locally:** run the full required set before pushing:
  - `go test ./...`
  - `go vet ./...`
  - `golangci-lint run ./...`
  - `shellcheck scripts/*.sh examples/*.sh`
  - `bash scripts/check-doc-drift.sh`
  - `npx --yes markdownlint-cli2 "**/*.md"`
- **Known non-blocking checks:** workflow lint remains advisory in Phase C.

## govulncheck Enforcement Decision (Phase C)

Decision: promote `govulncheck ./...` to required in Phase C.

Rationale:

1. Current baseline has no demonstrated vuln backlog in this repo.
2. Existing local/CI runs have produced actionable, low-noise signal.
3. Security regressions should fail fast once staged lint and docs gates are already stable.

Command evidence (local):

```bash
govulncheck ./...
# No vulnerabilities found.
```

Fallback condition:

- If future runs reveal concrete high-noise behavior or a legitimate external backlog that cannot be quickly remediated, temporarily downgrade only `govulncheck` to advisory with a tracked follow-up issue and target re-enforcement date.

## Rollback Plan (if Phase C creates unacceptable friction)

If required gates create widespread false positives or materially block delivery, use the explicit rollback levers below.

### Explicit rollback levers

| Lever | How to apply | Scope | Guardrail |
|---|---|---|---|
| Downgrade one noisy check | Move that check from `phase-c-quality` to `advisory-quality` (or add `continue-on-error: true` on the step) in `.github/workflows/quality.yml` | Single check only | Must keep `go test` and `go vet` in required jobs |
| Temporary govulncheck de-escalation | Mark `Go vuln check` step advisory while triaging upstream/backlog findings | `govulncheck` only | Track owner + target date to re-enforce |
| Temporary docs gate de-escalation | Mark `Docs drift` and/or `Markdown lint` advisory if docs tooling causes broad false positives | Docs gates only | Keep lint/test/vet/shellcheck required |

### Rollback execution checklist

1. Record trigger and impact in PR/issue (what failed, how often, and why signal is low).
2. Apply the narrowest lever possible in `.github/workflows/quality.yml`.
3. Keep `go test` + `go vet` required at all times.
4. Open a follow-up issue with owner and target date for re-enforcement.
5. Restore required status once baseline cleanup is complete and failure signal is high-confidence.

This rollback model is intentionally narrow: it downgrades only noisy checks and preserves the required baseline.

## Promotion Criteria

Checks should be promoted from advisory to required only when:

1. Baseline failures are triaged and either fixed or explicitly accepted.
2. New failures are actionable and high-signal.
3. Local developer workflow and README commands are aligned with CI behavior.

## Local Repro Commands

```bash
go test ./...
go vet ./...
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run ./...
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
shellcheck scripts/*.sh examples/*.sh
npx --yes markdownlint-cli2 "**/*.md"
bash scripts/check-doc-drift.sh
```

## Local Git Hooks (pre-commit)

This repo includes `.pre-commit-config.yaml` so contributors can run the same core checks before commit/push.

Install once per clone:

```bash
pre-commit install
pre-commit install -t pre-push
```

Hook mapping:

- **pre-commit:** `go-fmt`, `goimports`, `golangci-lint --fast`, `shellcheck` (scripts/examples), `markdownlint-cli2`
- **pre-push:** `bash scripts/check-doc-drift.sh`

Manual invocation:

```bash
pre-commit run --all-files
pre-commit run --hook-stage pre-push check-doc-drift
```

Bypass guidance (only for urgent exceptions; follow up with a fixing commit):

```bash
SKIP=golangci-lint git commit -m "temporary bypass"
git commit --no-verify -m "temporary bypass"
git push --no-verify
```
