package quality

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../.."))
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestPreCommitConfigIncludesRequiredHooks(t *testing.T) {
	cfg := readRepoFile(t, ".pre-commit-config.yaml")

	required := []string{
		"go-fmt",
		"goimports",
		"golangci-lint",
		"shellcheck",
		"markdownlint-cli2",
		"check-doc-drift",
	}
	for _, needle := range required {
		if !strings.Contains(cfg, needle) {
			t.Fatalf("missing hook marker %q in .pre-commit-config.yaml", needle)
		}
	}
}

func TestDocsMentionPreCommitSetupAndBypass(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	policy := readRepoFile(t, "docs/quality-policy.md")

	for _, tc := range []struct {
		name string
		text string
		hay  string
	}{
		{"readme install", "pre-commit install", readme},
		{"readme pre-push", "pre-commit install -t pre-push", readme},
		{"readme bypass", "SKIP=", readme},
		{"policy install", "pre-commit install", policy},
		{"policy bypass", "--no-verify", policy},
	} {
		if !strings.Contains(tc.hay, tc.text) {
			t.Fatalf("%s: expected to find %q", tc.name, tc.text)
		}
	}
}
