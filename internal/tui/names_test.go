package tui

import (
	"path/filepath"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func newTestMsgDB(t *testing.T) *MsgDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "msg.db")
	db, err := NewMsgDB(dbPath)
	if err != nil {
		t.Fatalf("NewMsgDB() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNameResolver_ResolveAliasLifecycleAndPersistence(t *testing.T) {
	db := newTestMsgDB(t)
	const nodeAddr = "A1B2"
	const peerAddr = "0x0000BEEF"

	r := NewNameResolver(db, nodeAddr)

	if got := r.Resolve(peerAddr); got != "0000BEEF" {
		t.Fatalf("Resolve() before alias = %q, want %q", got, "0000BEEF")
	}

	if err := r.SetAlias(peerAddr, "Alice"); err != nil {
		t.Fatalf("SetAlias() error = %v", err)
	}

	if got := r.Resolve(peerAddr); got != "Alice" {
		t.Fatalf("Resolve() with alias = %q, want %q", got, "Alice")
	}

	gotAlias, ok := r.GetAlias(peerAddr)
	if !ok || gotAlias != "Alice" {
		t.Fatalf("GetAlias() = (%q, %v), want (%q, true)", gotAlias, ok, "Alice")
	}

	reloaded := NewNameResolver(db, nodeAddr)
	if err := reloaded.LoadAliases(); err != nil {
		t.Fatalf("LoadAliases() error = %v", err)
	}
	if got := reloaded.Resolve(peerAddr); got != "Alice" {
		t.Fatalf("Resolve() after LoadAliases = %q, want %q", got, "Alice")
	}

	if err := reloaded.RemoveAlias(peerAddr); err != nil {
		t.Fatalf("RemoveAlias() error = %v", err)
	}
	if got := reloaded.Resolve(peerAddr); got != "0000BEEF" {
		t.Fatalf("Resolve() after RemoveAlias = %q, want %q", got, "0000BEEF")
	}

	missing, ok := reloaded.GetAlias(peerAddr)
	if ok || missing != "" {
		t.Fatalf("GetAlias() after RemoveAlias = (%q, %v), want (\"\", false)", missing, ok)
	}
}

func TestNameResolver_UpdateFirmwareNamesAndResolveWithHash(t *testing.T) {
	r := NewNameResolver(nil, "NODE")
	addr := "0xABCD1234"

	r.UpdateFirmwareNames([]bramble.Neighbor{{Address: addr, Name: "  Lily  "}, {Address: "0xDEAD", Name: "   "}})

	if got := r.Resolve(addr); got != "Lily" {
		t.Fatalf("Resolve() firmware name = %q, want %q", got, "Lily")
	}
	if got := r.ResolveWithHash(addr); got != "Lily(1234)" {
		t.Fatalf("ResolveWithHash() firmware name = %q, want %q", got, "Lily(1234)")
	}

	// Alias should take priority over firmware name.
	if err := r.SetAlias(addr, "Bestie"); err != nil {
		t.Fatalf("SetAlias() error = %v", err)
	}
	if got := r.ResolveWithHash(addr); got != "Bestie(1234)" {
		t.Fatalf("ResolveWithHash() alias priority = %q, want %q", got, "Bestie(1234)")
	}

	if got := r.ResolveWithHash(""); got != "????" {
		t.Fatalf("ResolveWithHash(\"\") = %q, want %q", got, "????")
	}
}

func TestNameResolver_ReverseLookup_IsCaseInsensitive(t *testing.T) {
	r := NewNameResolver(nil, "NODE")

	if err := r.SetAlias("0xAA11", "CaseSensitiveAlias"); err != nil {
		t.Fatalf("SetAlias() error = %v", err)
	}
	r.UpdateFirmwareNames([]bramble.Neighbor{{Address: "0xBB22", Name: "FirmwareName"}})

	if got, ok := r.ReverseLookup("casesensitivealias"); !ok || got != "AA11" {
		t.Fatalf("ReverseLookup(alias) = (%q, %v), want (%q, true)", got, ok, "AA11")
	}
	if got, ok := r.ReverseLookup("firmwarename"); !ok || got != "BB22" {
		t.Fatalf("ReverseLookup(firmware) = (%q, %v), want (%q, true)", got, ok, "BB22")
	}
	if got, ok := r.ReverseLookup("does-not-exist"); ok || got != "" {
		t.Fatalf("ReverseLookup(missing) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestNameResolver_EdgeCases_EmptyAliasAndLoadAliasesNilDB(t *testing.T) {
	r := NewNameResolver(nil, "NODE")

	if err := r.LoadAliases(); err != nil {
		t.Fatalf("LoadAliases() with nil db error = %v", err)
	}

	if err := r.SetAlias("0xABCD", ""); err != nil {
		t.Fatalf("SetAlias() empty alias error = %v", err)
	}
	if got := r.Resolve("0xABCD"); got != "ABCD" {
		t.Fatalf("Resolve() with empty alias = %q, want %q", got, "ABCD")
	}
	if alias, ok := r.GetAlias("0xABCD"); ok || alias != "" {
		t.Fatalf("GetAlias() empty alias = (%q, %v), want (\"\", false)", alias, ok)
	}
}
