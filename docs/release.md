# Release Process (Semantic Release)

This repository uses **semantic-release** with **Conventional Commits** to publish version tags and GitHub releases.

## Trigger

Releases run from GitHub Actions on:
- `push` to `main`
- manual `workflow_dispatch`

## Required repository secrets

No extra secrets are required. The `release` workflow authenticates with the
built-in `GITHUB_TOKEN`, which has permission to create tags and releases in
this repository.

## Commit format

Use Conventional Commits, e.g.:

- `feat(cli): add monitor --topic filter` → **minor**
- `fix(serial): handle reconnect race` → **patch**
- `feat!: remove legacy transport flag` or `BREAKING CHANGE:` in body → **major**

Other commit types (like `docs`, `chore`, `refactor` without `!`) typically do not trigger a release.

## How to release

1. Merge Conventional Commit-formatted changes into `main`.
2. The `release` workflow runs automatically.
3. If releasable commits exist, semantic-release creates:
   - a tag like `vX.Y.Z`
   - release notes
   - a GitHub Release
