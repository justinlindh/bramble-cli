# Public Publishing Checklist

Use this checklist before publishing docs, releases, or announcements that reference this repository.

## 1) Example hygiene

- [ ] Replace machine-specific hosts/IPs with placeholders where practical (for example `<node-host>`, `<ota-host>`).
- [ ] Keep examples realistic and reproducible for users in arbitrary environments.

## 2) Sensitive content scan

- [ ] Search for accidental secrets/tokens/private keys.
- [ ] Search for absolute local paths (for example `/home/...`, `/Users/...`, `C:\Users\...`).
- [ ] Confirm CI secret handling uses environment/secret variables, not plaintext values.

## 3) Docs discoverability

- [ ] Ensure operational docs are linked from `README.md` or `docs/README.md`.
- [ ] Keep historical implementation docs under `docs/plans/` and clearly labeled as historical.

## 4) Validation

Run:

```bash
bash scripts/check-doc-drift.sh
go test ./...
go vet ./...
npx --yes markdownlint-cli2 "**/*.md"
```

## 5) Final review

- [ ] Check `git status` for accidental local artifacts.
- [ ] Verify examples use placeholders or clearly document that values are environment-specific.
