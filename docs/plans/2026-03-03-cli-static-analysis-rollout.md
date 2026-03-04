# Bramble CLI Static Analysis & Lint Rollout Implementation Plan

> **For Agent:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add practical, low-noise static analysis and lint gates to bramble-cli with staged enforcement in CI.

**Architecture:** Introduce a dedicated CI quality workflow and minimal config files first, then ratchet from advisory checks to enforced gates. Keep checks tool-specific (Go, shell, markdown) and isolate optional security scans to non-blocking stage before enforcement.

**Tech Stack:** Go 1.25.x, golangci-lint, go test, govulncheck, ShellCheck, markdownlint-cli2, Gitea Actions.

---

## Context Snapshot (from discovery report)
- Repo: `/home/user/src/bramble-cli`
- Existing CI: release-only workflow in `.gitea/workflows/release.yml`
- Existing quality controls: commitlint + semantic-release, manual `go test ./...`, helper script `scripts/check-doc-drift.sh`
- Gap: no PR/push quality gate for lint/static/security/doc checks

---

### Task 1: Add baseline quality workflow (non-blocking advisory stage)

**Files:**
- Create: `.gitea/workflows/quality.yml`
- Modify: `README.md` (section for quality checks)

**Step 1: Write failing workflow smoke expectation**
- Add workflow file with jobs scaffold but intentionally reference one missing command to confirm CI wiring in first run.

**Step 2: Run local workflow command equivalents and verify failure**
Run:
```bash
go test ./...
go vet ./...
```
Expected: local commands pass; CI scaffold failure confirms job is actually executing.

**Step 3: Implement minimal working quality workflow**
- Configure workflow triggers for `push` + `pull_request`.
- Add jobs:
  - `go-test-vet` (required)
  - `lint-go` (advisory for now)
  - `lint-shell` (advisory)
  - `lint-markdown` (advisory)
  - `security-vuln` (advisory)

**Step 4: Re-run local equivalents**
Run:
```bash
go test ./...
go vet ./...
```
Expected: pass.

**Step 5: Commit**
```bash
git add .gitea/workflows/quality.yml README.md
git commit -m "ci: add baseline quality workflow for tests, lint, and vuln scan"
```

---

### Task 2: Add Go lint configuration (targeted, low-noise)

**Files:**
- Create: `.golangci.yml`
- Modify: `README.md`

**Step 1: Write failing lint expectation**
Run:
```bash
golangci-lint run ./...
```
Expected: fail or missing config behavior.

**Step 2: Add minimal config**
- Enable initial linters: `govet`, `errcheck`, `staticcheck`, `ineffassign`, `typecheck`, `gofmt`, `goimports`.
- Add sensible exclusions for generated/vendor or explicitly justified paths.
- Set timeout and concurrency defaults.

**Step 3: Re-run lint**
Run:
```bash
golangci-lint run ./...
```
Expected: pass, or actionable list with bounded findings.

**Step 4: Document local usage**
- Add `README` “Quality Checks” section with exact commands.

**Step 5: Commit**
```bash
git add .golangci.yml README.md
git commit -m "lint(go): add golangci baseline config and docs"
```

---

### Task 3: Add shell + markdown lint configs and commands

**Files:**
- Create: `.markdownlint-cli2.yaml`
- Modify: `.gitea/workflows/quality.yml`
- Modify: `README.md`

**Step 1: Run failing shell lint expectation**
Run:
```bash
shellcheck scripts/*.sh examples/*.sh
```
Expected: either clean or actionable findings.

**Step 2: Add markdown lint config**
- Configure practical markdownlint rules (line-length relaxed for command/docs blocks if needed).

**Step 3: Wire commands into workflow**
- Ensure quality workflow executes:
  - ShellCheck over `scripts/*.sh examples/*.sh`
  - markdownlint-cli2 over `**/*.md`

**Step 4: Run local checks**
Run:
```bash
shellcheck scripts/*.sh examples/*.sh
npx --yes markdownlint-cli2 "**/*.md"
```
Expected: pass or explicit bounded issues.

**Step 5: Commit**
```bash
git add .markdownlint-cli2.yaml .gitea/workflows/quality.yml README.md
git commit -m "lint(docs,shell): add markdownlint and shellcheck checks"
```

---

### Task 4: Add govulncheck stage and staged enforcement policy

**Files:**
- Modify: `.gitea/workflows/quality.yml`
- Create: `docs/quality-policy.md`
- Modify: `README.md`

**Step 1: Add vuln scan command**
- Add `govulncheck ./...` as advisory initially.

**Step 2: Define enforcement timeline**
- Document phases:
  - Phase A (week 1): advisory only for lint/vuln/docs.
  - Phase B: required `golangci-lint` + `shellcheck`.
  - Phase C: required markdownlint and `govulncheck` (or keep vuln as advisory if signal quality demands).

**Step 3: Validate docs consistency helper remains intact**
Run:
```bash
bash scripts/check-doc-drift.sh
```
Expected: pass.

**Step 4: Commit**
```bash
git add .gitea/workflows/quality.yml docs/quality-policy.md README.md
git commit -m "ci: add vuln scan and staged lint enforcement policy"
```

---

### Task 5: Final verification + push

**Files:**
- Verify all modified files from Tasks 1-4

**Step 1: Run full local quality suite**
Run:
```bash
go test ./...
go vet ./...
golangci-lint run ./...
shellcheck scripts/*.sh examples/*.sh
npx --yes markdownlint-cli2 "**/*.md"
govulncheck ./...
bash scripts/check-doc-drift.sh
```
Expected: pass (or explicitly documented temporary exceptions).

**Step 2: Push**
```bash
git push origin <branch>
```

**Step 3: Open PR with staged rollout notes**
- Include which checks are required now vs advisory.

---

## Rollout Guardrails
- Keep first PR low-risk: config + workflow + docs; avoid unrelated refactors.
- Any linter suppression requires inline justification.
- Prefer fixing true positives over global ignores.
- If lint churn is high, split into follow-up cleanup PRs by package.

## Definition of Done
- New quality workflow exists and runs on push/PR.
- Go lint/static analysis configured and documented.
- Shell/markdown checks configured and documented.
- Govulncheck integrated with explicit enforcement policy.
- Commands reproducible locally from README.
