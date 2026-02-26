// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"strings"
	"sync"

	bramble "github.com/justinlindh/bramble-go"
)

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
		r.aliases[peer] = alias
	}
	return nil
}

// Resolve returns the best available name for addr.
func (r *NameResolver) Resolve(addr string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if alias, ok := r.aliases[addr]; ok && alias != "" {
		return alias
	}
	if name, ok := r.firmware[addr]; ok && name != "" {
		return name
	}
	// Fall back to short hex: last 8 chars of address (drop 0x prefix if present)
	s := strings.TrimPrefix(addr, "0x")
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	return s
}

// GetAlias returns just the local alias for addr, if any.
func (r *NameResolver) GetAlias(addr string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.aliases[addr]
	return a, ok && a != ""
}

// SetAlias sets (and persists) a local alias for addr.
func (r *NameResolver) SetAlias(addr, alias string) error {
	r.mu.Lock()
	r.aliases[addr] = alias
	r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	return r.db.upsertAlias(r.nodeAddr, addr, alias)
}

// RemoveAlias removes the local alias for addr.
func (r *NameResolver) RemoveAlias(addr string) error {
	r.mu.Lock()
	delete(r.aliases, addr)
	r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	return r.db.deleteAlias(r.nodeAddr, addr)
}

// UpdateFirmwareNames refreshes firmware-advertised names from a neighbor list.
func (r *NameResolver) UpdateFirmwareNames(neighbors []bramble.Neighbor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range neighbors {
		if n.Name != "" {
			r.firmware[n.Address] = n.Name
		}
	}
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
