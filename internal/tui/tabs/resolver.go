// Package tabs contains individual tab models for the Bramble TUI.
package tabs

// PeerResolver resolves a peer address to a human-readable name.
// Priority: local alias → firmware name → short hex.
type PeerResolver interface {
	Resolve(addr string) string
	GetAlias(addr string) (string, bool)
	SetAlias(addr, alias string) error
	// ReverseLookup finds an address by alias or firmware name.
	ReverseLookup(name string) (string, bool)
}
