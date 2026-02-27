// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"fmt"
	"strings"
	"sync"

	bramble "github.com/justinlindh/bramble-go"
)

func canonicalAddr(addr string) string {
	a := strings.TrimSpace(addr)
	a = strings.TrimPrefix(a, "0x")
	a = strings.TrimPrefix(a, "0X")
	return strings.ToUpper(a)
}

func shortHash(addr string) string {
	a := canonicalAddr(addr)
	if len(a) > 4 {
		return a[len(a)-4:]
	}
	return a
}

// NameResolver resolves peer addresses to human-readable names.
// Priority: local alias → firmware-advertised name → short hex address.
type NameResolver struct {
	mu       sync.RWMutex
	db       *MsgDB
	aliases  map[string]string // local aliases (node-scoped), keyed by peer addr
	firmware map[string]string // firmware-advertised names, keyed by peer addr
	nodeAddr string
}

// NewNameResolver creates a NameResolver. Call LoadAliases after setting nodeAddr.
func NewNameResolver(db *MsgDB, nodeAddr string) *NameResolver {
	r := &NameResolver{
		db:       db,
		aliases:  make(map[string]string),
		firmware: make(map[string]string),
		nodeAddr: nodeAddr,
	}
	return r
}

// LoadAliases loads persisted aliases from the DB.
func (r *NameResolver) LoadAliases() error {
	if r.db == nil {
		return nil
	}
	rows, err := r.db.loadAliases(r.nodeAddr)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for peer, alias := range rows {
		r.aliases[canonicalAddr(peer)] = alias
	}
	return nil
}

// Resolve returns the best available name for addr.
func (r *NameResolver) Resolve(addr string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := canonicalAddr(addr)
	if alias, ok := r.aliases[key]; ok && alias != "" {
		return alias
	}
	if name, ok := r.firmware[key]; ok && name != "" {
		return name
	}
	// Fall back to short hex: last 8 chars of canonical address.
	s := key
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	return s
}

// GetAlias returns just the local alias for addr, if any.
func (r *NameResolver) GetAlias(addr string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.aliases[canonicalAddr(addr)]
	return a, ok && a != ""
}

// SetAlias sets (and persists) a local alias for addr.
func (r *NameResolver) SetAlias(addr, alias string) error {
	key := canonicalAddr(addr)
	r.mu.Lock()
	r.aliases[key] = alias
	r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	return r.db.upsertAlias(r.nodeAddr, key, alias)
}

// RemoveAlias removes the local alias for addr.
func (r *NameResolver) RemoveAlias(addr string) error {
	key := canonicalAddr(addr)
	r.mu.Lock()
	delete(r.aliases, key)
	r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	return r.db.deleteAlias(r.nodeAddr, key)
}

// UpdateFirmwareNames refreshes firmware-advertised names from a neighbor list.
func (r *NameResolver) UpdateFirmwareNames(neighbors []bramble.Neighbor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range neighbors {
		name := strings.TrimSpace(n.Name)
		if name == "" {
			continue
		}
		r.firmware[canonicalAddr(n.Address)] = name
	}
}

// ResolveWithHash returns the best available display label in name+hash form.
// Example: "Lily(3079)". If no name is known, returns canonical hex address.
func (r *NameResolver) ResolveWithHash(addr string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := canonicalAddr(addr)
	name := ""
	if alias, ok := r.aliases[key]; ok && alias != "" {
		name = alias
	} else if fw, ok := r.firmware[key]; ok && fw != "" {
		name = fw
	}
	if name != "" {
		return fmt.Sprintf("%s(%s)", name, shortHash(key))
	}
	if key == "" {
		return "????"
	}
	return key
}

// ReverseLookup finds an address by alias or firmware name (case-insensitive).
func (r *NameResolver) ReverseLookup(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lower := strings.ToLower(name)
	for addr, alias := range r.aliases {
		if strings.ToLower(alias) == lower {
			return addr, true
		}
	}
	for addr, fw := range r.firmware {
		if strings.ToLower(fw) == lower {
			return addr, true
		}
	}
	return "", false
}
