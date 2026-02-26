// Package tabs contains individual tab models for the Bramble TUI.
package tabs

// PeerResolver resolves a peer address to a human-readable name.
// Priority: local alias → firmware name → short hex.
type PeerResolver interface {
	Resolve(addr string) string
	GetAlias(addr string) (string, bool)
	SetAlias(addr, alias string) error
}

// noopResolver always returns the raw address unchanged.
type noopResolver struct{}

func (noopResolver) Resolve(addr string) string            { return addr }
func (noopResolver) GetAlias(addr string) (string, bool)   { return "", false }
func (noopResolver) SetAlias(addr, alias string) error     { return nil }

// defaultResolver is used when no real resolver is set.
var defaultResolver PeerResolver = noopResolver{}
