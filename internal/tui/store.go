// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"fmt"
	"sync"

	bramble "github.com/justinlindh/bramble-go"
)

// Conversation holds messages and metadata for a single conversation thread.
type Conversation struct {
	ID       string
	Label    string
	Messages []bramble.Message
	Unread   int
}

// Store is a thread-safe state container for the TUI.
type Store struct {
	mu sync.RWMutex

	Identity      *bramble.IdentityResponse
	Status        *bramble.StatusResponse
	Neighbors     []bramble.Neighbor
	Routes        []bramble.Route
	Airtime       *bramble.AirtimeStats
	PeerLocations []bramble.LocationPeer
	OwnGPS        *bramble.GpsEvent

	// Conversations keyed by conv ID: "broadcast", "ch:N", "dm:ADDR"
	Conversations map[string]*Conversation
	// ConvOrder tracks insertion order for display.
	ConvOrder    []string
	ActiveConvID string
	ShowRoutes   bool
}

// NewStore creates an initialized Store.
func NewStore() *Store {
	s := &Store{
		Conversations: make(map[string]*Conversation),
	}
	s.addConvLocked("broadcast", "Broadcast")
	s.ActiveConvID = "broadcast"
	return s
}

func (s *Store) addConvLocked(id, label string) {
	if _, ok := s.Conversations[id]; !ok {
		s.Conversations[id] = &Conversation{ID: id, Label: label}
		s.ConvOrder = append(s.ConvOrder, id)
	}
}

// UpdateStatus replaces the cached status.
func (s *Store) UpdateStatus(st *bramble.StatusResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = st
}

// UpdateIdentity replaces the cached identity.
func (s *Store) UpdateIdentity(id *bramble.IdentityResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Identity = id
}

// UpdateNeighbors replaces the neighbor list.
func (s *Store) UpdateNeighbors(neighbors []bramble.Neighbor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Neighbors = neighbors
}

// UpdateRoutes replaces the route list.
func (s *Store) UpdateRoutes(routes []bramble.Route) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Routes = routes
}

// UpdateAirtime replaces airtime stats.
func (s *Store) UpdateAirtime(a *bramble.AirtimeStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Airtime = a
}

// UpdateOwnGPS stores the latest GPS event.
func (s *Store) UpdateOwnGPS(evt bramble.GpsEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OwnGPS = &evt
}

// UpdatePeerLocations replaces peer location list.
func (s *Store) UpdatePeerLocations(peers []bramble.LocationPeer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PeerLocations = peers
}

// AddMessage routes an incoming message to the correct conversation,
// auto-creating it if needed.
func (s *Store) AddMessage(msg bramble.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	convID := s.convIDForMessage(msg)
	label := s.convLabelForMessage(convID)
	s.addConvLocked(convID, label)

	conv := s.Conversations[convID]
	conv.Messages = append(conv.Messages, msg)
	if convID != s.ActiveConvID {
		conv.Unread++
	}
}

// UpdateAck updates message status after an ack (best-effort).
func (s *Store) UpdateAck(ack bramble.Ack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// TODO: store acks separately for delivery badge rendering
	_ = ack
}

// SetActiveConv switches the active conversation and clears unread.
func (s *Store) SetActiveConv(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveConvID = id
	if conv, ok := s.Conversations[id]; ok {
		conv.Unread = 0
	}
}

// GetOwnGPS returns a copy of the latest GPS event (or nil).
func (s *Store) GetOwnGPS() *bramble.GpsEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.OwnGPS == nil {
		return nil
	}
	cp := *s.OwnGPS
	return &cp
}

// GetPeerLocations returns a snapshot of peer locations.
func (s *Store) GetPeerLocations() []bramble.LocationPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]bramble.LocationPeer, len(s.PeerLocations))
	copy(out, s.PeerLocations)
	return out
}

// GetConversations returns a snapshot of conversations in order.
func (s *Store) GetConversations() []*Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Conversation, 0, len(s.ConvOrder))
	for _, id := range s.ConvOrder {
		if c, ok := s.Conversations[id]; ok {
			cp := *c
			msgs := make([]bramble.Message, len(c.Messages))
			copy(msgs, c.Messages)
			cp.Messages = msgs
			out = append(out, &cp)
		}
	}
	return out
}

// GetActiveConversation returns a snapshot of the active conversation.
func (s *Store) GetActiveConversation() *Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.Conversations[s.ActiveConvID]
	if !ok {
		return nil
	}
	cp := *c
	msgs := make([]bramble.Message, len(c.Messages))
	copy(msgs, c.Messages)
	cp.Messages = msgs
	return &cp
}

// convIDForMessage derives the conversation ID for a message.
func (s *Store) convIDForMessage(msg bramble.Message) string {
	if msg.To == "" || msg.To == "broadcast" {
		return "broadcast"
	}
	if len(msg.To) > 3 && msg.To[:3] == "ch:" {
		return msg.To
	}
	// DM: key by peer address
	if s.Identity != nil && msg.From == s.Identity.Address {
		return fmt.Sprintf("dm:%s", msg.To)
	}
	return fmt.Sprintf("dm:%s", msg.From)
}

func (s *Store) convLabelForMessage(convID string) string {
	switch {
	case convID == "broadcast":
		return "Broadcast"
	case len(convID) > 3 && convID[:3] == "ch:":
		return convID
	default:
		// DM: use peer address as label for now; future phases can enrich
		return convID[3:]
	}
}
