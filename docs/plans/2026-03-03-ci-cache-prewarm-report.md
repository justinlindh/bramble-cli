# CI Cache + Prewarm Optimization Report (Gitea)

Date: 2026-03-03
Repo: `bramble-cli`
Branch: `feature/quality-workflow-advisory-baseline`

## Summary
This update makes CI runs more deterministic and faster on Gitea `linux` runners by pinning tool versions, caching stable tool install paths, and prewarming dependency caches for both quality and release workflows.

## Files Updated
- `.gitea/workflows/quality.yml`
- `.gitea/workflows/release.yml`

## Runner Assumptions
- Gitea self-hosted runner label is `linux`.
- Docker is not required for these jobs.
- `shellcheck` may already be available; workflow falls back to `apt-get` install only if missing.

## Concrete Cache Strategy

### `quality.yml`
1. **Go module/build cache via `actions/setup-go@v5`**
   - `cache: true`
   - `cache-dependency-path: go.sum`
   - Added `go mod download` prewarm step for deterministic module population.

2. **Go tool cache (`golangci-lint`, `govulncheck`, `actionlint`)**
   - Cache path: `~/.cache/bramble-cli/go-tools`
   - Install location: `GOBIN=$HOME/.cache/bramble-cli/go-tools/bin`
   - Keys:
     - Required quality tools:
       - `${{ runner.os }}-go-tools-${{ env.GOLANGCI_LINT_VERSION }}-${{ env.GOVULNCHECK_VERSION }}-${{ hashFiles('.gitea/workflows/quality.yml') }}`
     - Advisory actionlint:
       - `${{ runner.os }}-go-tools-advisory-${{ env.ACTIONLINT_VERSION }}-${{ hashFiles('.gitea/workflows/quality.yml') }}`

3. **npm/markdownlint cache**
   - Cache paths:
     - `~/.npm` (tarball cache)
     - `~/.cache/bramble-cli/npm-tools` (installed markdownlint binary)
   - Key:
     - `${{ runner.os }}-npm-tools-${{ env.MARKDOWNLINT_VERSION }}-${{ hashFiles('.gitea/workflows/quality.yml') }}`

4. **Apt churn reduction**
   - `shellcheck` install only when missing on runner.

### `release.yml`
1. **Pinned Node release toolchain prewarm**
   - Install path: `~/.cache/bramble-cli/release-tools`
   - Tools pinned:
     - `@commitlint/cli@20.4.3`
     - `@commitlint/config-conventional@20.4.3`
     - `semantic-release@25.0.3`
     - `@semantic-release/commit-analyzer@13.0.1`
     - `@semantic-release/release-notes-generator@14.1.0`
     - `@saithodev/semantic-release-gitea@2.1.0`

2. **Release npm cache key**
   - Cache paths:
     - `~/.npm`
     - `~/.cache/bramble-cli/release-tools`
   - Key:
     - `${{ runner.os }}-node${{ env.NODE_VERSION }}-release-tools-${{ env.COMMITLINT_VERSION }}-${{ env.SEMANTIC_RELEASE_VERSION }}-${{ env.SEMREL_COMMIT_ANALYZER_VERSION }}-${{ env.SEMREL_NOTES_GENERATOR_VERSION }}-${{ env.SEMREL_GITEA_PLUGIN_VERSION }}-${{ hashFiles('.gitea/workflows/release.yml', '.commitlintrc.cjs', '.releaserc.cjs') }}`

## Invalidation Strategy
- **Go deps/build cache:** invalidated by `go.sum` change.
- **Go tool cache:** invalidated by pinned tool version changes and workflow hash changes.
- **npm markdownlint cache:** invalidated by markdownlint version change and quality workflow hash.
- **Release tools cache:** invalidated by node major/tool version changes and workflow/release config hash changes.

## Before/After Timing Evidence (best-effort local benchmark)
No direct Gitea run timing API evidence was captured in this revisit session, so measurements were taken locally using the same install logic as workflows.

Measured on 2026-03-03 (America/Los_Angeles), in `bramble-cli` workspace:
- `quality_tools_cold`: **4639 ms**
- `quality_tools_warm`: **2 ms**
- `release_tools_cold`: **5189 ms**
- `release_tools_warm`: **1 ms**

Interpretation:
- Warm-cache runs skip tool download/install work almost entirely for pinned tools.
- This reduces repeated `go install`/`npm install` overhead on subsequent CI runs.

Raw local timing artifacts:
- `.tmp/ci-perf/quality-cold.txt`
- `.tmp/ci-perf/quality-warm.txt`
- `.tmp/ci-perf/release-cold.txt`
- `.tmp/ci-perf/release-warm.txt`

## Verification Performed
- `actionlint .gitea/workflows/quality.yml .gitea/workflows/release.yml`
- `go test ./...`
- `golangci-lint run ./...`
- `shellcheck scripts/*.sh examples/*.sh` (if available)
- `bash scripts/check-doc-drift.sh`
- `markdownlint-cli2 "**/*.md"`
- `govulncheck ./...`

All checks passed locally.
