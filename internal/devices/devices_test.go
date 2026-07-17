package devices

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"198.51.100.65":      "ws://198.51.100.65/ws",
		"198.51.100.65:8080": "ws://198.51.100.65:8080/ws",
		"ws://foo/ws":        "ws://foo/ws",
		"wss://foo/ws":       "wss://foo/ws",
		"  10.0.0.1  ":       "ws://10.0.0.1/ws",
		"":                   "",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskToken(t *testing.T) {
	cases := map[string]string{
		"":                                 "(none)",
		"short":                            "****",
		"3EF9F51ACB994C7D3783D7882429DFD8": "3EF9...DFD8",
	}
	for in, want := range cases {
		if got := MaskToken(in); got != want {
			t.Errorf("MaskToken(%q) = %q, want %q", in, got, want)
		}
	}
	// A masked token must never contain the full secret.
	full := "3EF9F51ACB994C7D3783D7882429DFD8"
	if MaskToken(full) == full {
		t.Fatal("MaskToken returned the full token")
	}
}

func TestValidateAlias(t *testing.T) {
	good := []string{"v3", "V4", "node-1", "my.node_2"}
	for _, a := range good {
		if err := ValidateAlias(a); err != nil {
			t.Errorf("ValidateAlias(%q) unexpected error: %v", a, err)
		}
	}
	bad := []string{"", "has space", "tab\t", "weird/slash", "amp&"}
	for _, a := range bad {
		if err := ValidateAlias(a); err == nil {
			t.Errorf("ValidateAlias(%q) expected error, got nil", a)
		}
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(b.Devices) != 0 {
		t.Fatalf("expected empty book, got %d entries", len(b.Devices))
	}
	if b.Version != schemaVersion {
		t.Fatalf("expected version %d, got %d", schemaVersion, b.Version)
	}
}

func TestAddGetRemoveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "devices.json")

	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Add("v4", Entry{Host: "198.51.100.146", Token: "tok", Name: "V4"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}

	// File must be 0600, dir 0700.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 700", perm)
	}

	// Reload and verify normalization + persistence.
	b2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := b2.Get("v4")
	if !ok {
		t.Fatal("expected v4 entry after reload")
	}
	if e.Host != "ws://198.51.100.146/ws" {
		t.Errorf("host = %q, want normalized ws url", e.Host)
	}
	if e.Token != "tok" {
		t.Errorf("token = %q, want tok", e.Token)
	}

	// Remove.
	if !b2.Remove("v4") {
		t.Fatal("Remove returned false for existing alias")
	}
	if b2.Remove("v4") {
		t.Fatal("Remove returned true for missing alias")
	}
}

func TestAddRejectsBadAliasAndEmptyHost(t *testing.T) {
	b := &Book{Devices: map[string]Entry{}}
	if err := b.Add("bad alias", Entry{Host: "1.2.3.4"}); err == nil {
		t.Error("expected error for bad alias")
	}
	if err := b.Add("ok", Entry{Host: ""}); err == nil {
		t.Error("expected error for empty host")
	}
}

func TestListSorted(t *testing.T) {
	b := &Book{Devices: map[string]Entry{}}
	_ = b.Add("zeta", Entry{Host: "1.1.1.1"})
	_ = b.Add("alpha", Entry{Host: "2.2.2.2"})
	_ = b.Add("mike", Entry{Host: "3.3.3.3"})
	list := b.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].Alias != "alpha" || list[1].Alias != "mike" || list[2].Alias != "zeta" {
		t.Fatalf("not sorted: %v", []string{list[0].Alias, list[1].Alias, list[2].Alias})
	}
}
