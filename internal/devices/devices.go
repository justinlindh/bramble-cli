// Package devices implements the local "address book": named device aliases,
// each holding a WebSocket host and (optionally) an auth token, persisted as a
// small JSON file under the user's config directory.
//
// The token is a secret. The file is written 0600 inside a 0700 directory, but
// it is stored in plaintext; callers must never print a full token (see
// MaskToken).
package devices

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// schemaVersion is the on-disk format version. Bump only on breaking changes.
const schemaVersion = 1

// Entry is a single saved device.
type Entry struct {
	// Host is a fully-normalized transport URL (e.g. "ws://198.51.100.65/ws").
	Host string `json:"host"`
	// Token is the per-device auth token, or "" if none is stored.
	Token string `json:"token,omitempty"`
	// Name is an optional human label.
	Name string `json:"name,omitempty"`
	// Port is an optional serial port path (informational).
	Port string `json:"port,omitempty"`
}

// Book is the whole address book.
type Book struct {
	Version int              `json:"version"`
	Devices map[string]Entry `json:"devices"`
}

// NamedEntry pairs an alias with its entry for listing.
type NamedEntry struct {
	Alias string
	Entry Entry
}

var aliasRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateAlias reports whether alias is a legal address-book key: non-empty,
// no whitespace, limited to letters, digits, dot, underscore and hyphen.
func ValidateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias must not be empty")
	}
	if !aliasRE.MatchString(alias) {
		return fmt.Errorf("invalid alias %q: use only letters, digits, '.', '_' or '-'", alias)
	}
	return nil
}

// NormalizeHost turns a user-supplied host into a transport URL. A value that
// already carries a scheme ("ws://", "wss://", ...) is used as-is; a bare host
// or host:port is expanded to ws://<host>/ws.
func NormalizeHost(host string) string {
	h := strings.TrimSpace(host)
	if h == "" {
		return ""
	}
	if strings.Contains(h, "://") {
		return h
	}
	return "ws://" + h + "/ws"
}

// MaskToken renders a token for display without revealing it. Empty tokens show
// as "(none)"; short tokens are fully masked; longer tokens show the first and
// last four characters.
func MaskToken(token string) string {
	if token == "" {
		return "(none)"
	}
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// DefaultPath returns the address-book path, honoring XDG_CONFIG_HOME and
// falling back to ~/.config/bramble/devices.json.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "bramble", "devices.json"), nil
}

// Load reads the address book at path. A missing file yields an empty book (not
// an error); a malformed file is reported.
func Load(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Book{Version: schemaVersion, Devices: map[string]Entry{}}, nil
		}
		return nil, fmt.Errorf("read device book %s: %w", path, err)
	}
	var b Book
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse device book %s: %w", path, err)
	}
	if b.Devices == nil {
		b.Devices = map[string]Entry{}
	}
	if b.Version == 0 {
		b.Version = schemaVersion
	}
	return &b, nil
}

// Save writes the book to path, creating the directory 0700 and the file 0600.
func (b *Book) Save(path string) error {
	if b.Version == 0 {
		b.Version = schemaVersion
	}
	if b.Devices == nil {
		b.Devices = map[string]Entry{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode device book: %w", err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		return fmt.Errorf("write device book %s: %w", path, err)
	}
	return nil
}

// Add validates alias, normalizes the entry's host and stores it (replacing any
// existing entry for that alias).
func (b *Book) Add(alias string, e Entry) error {
	if err := ValidateAlias(alias); err != nil {
		return err
	}
	e.Host = NormalizeHost(e.Host)
	if e.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if b.Devices == nil {
		b.Devices = map[string]Entry{}
	}
	b.Devices[alias] = e
	return nil
}

// Get returns the entry for alias, if present.
func (b *Book) Get(alias string) (Entry, bool) {
	e, ok := b.Devices[alias]
	return e, ok
}

// Remove deletes alias, reporting whether it existed.
func (b *Book) Remove(alias string) bool {
	if _, ok := b.Devices[alias]; !ok {
		return false
	}
	delete(b.Devices, alias)
	return true
}

// List returns all entries sorted by alias.
func (b *Book) List() []NamedEntry {
	out := make([]NamedEntry, 0, len(b.Devices))
	for alias, e := range b.Devices {
		out = append(out, NamedEntry{Alias: alias, Entry: e})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}
