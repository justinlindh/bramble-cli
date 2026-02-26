// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	_ "modernc.org/sqlite"
)

// StoredMessage maps to the messages DB row and can convert to/from bramble.Message.
type StoredMessage struct {
	ID        string
	NodeAddr  string
	ConvID    string
	Direction string // "in" or "out"
	Sender    string
	Text      string
	Timestamp int64
	Status    string
	Channel   int
	RelayPath []string // JSON-encoded hop addresses
	CreatedAt int64
}

// ToBramble converts a StoredMessage to a bramble.Message.
func (s StoredMessage) ToBramble() bramble.Message {
	msg := bramble.Message{
		Text:      s.Text,
		Timestamp: s.Timestamp,
		MsgID:     s.ID,
	}
	if s.Direction == "out" {
		msg.From = s.NodeAddr
		msg.To = convIDToTo(s.ConvID)
	} else {
		msg.From = s.Sender
		msg.To = convIDToTo(s.ConvID)
	}
	return msg
}

// StoredMessageFromBramble converts a bramble.Message + metadata to a StoredMessage.
func StoredMessageFromBramble(msg bramble.Message, nodeAddr, convID, direction string) StoredMessage {
	id := msg.MsgID
	if id == "" {
		id = fmt.Sprintf("local-%d-%s", time.Now().UnixNano(), msg.From)
	}
	return StoredMessage{
		ID:        id,
		NodeAddr:  nodeAddr,
		ConvID:    convID,
		Direction: direction,
		Sender:    msg.From,
		Text:      msg.Text,
		Timestamp: msg.Timestamp,
		Status:    "sent",
		CreatedAt: time.Now().Unix(),
	}
}

// ConversationSummary holds summary info for listing conversations.
type ConversationSummary struct {
	ID            string
	Label         string
	LastMessage   string
	LastTimestamp int64
	UnreadCount   int
}

// MsgDB is a SQLite-backed message store scoped to a node address.
type MsgDB struct {
	mu       sync.Mutex
	db       *sql.DB
	nodeAddr string
}

// DefaultDBPath returns the default path for the messages DB.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "bramble", "messages.db"), nil
}

// NewMsgDB opens (or creates) the SQLite database at dbPath and runs migrations.
func NewMsgDB(dbPath string) (*MsgDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("msgdb: mkdir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000&_fk=true")
	if err != nil {
		return nil, fmt.Errorf("msgdb: open: %w", err)
	}

	// Single writer, multiple readers.
	db.SetMaxOpenConns(1)

	m := &MsgDB{db: db}
	if err := m.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("msgdb: migrate: %w", err)
	}
	return m, nil
}

func (m *MsgDB) migrate() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id          TEXT PRIMARY KEY,
			node_addr   TEXT NOT NULL,
			conv_id     TEXT NOT NULL,
			direction   TEXT NOT NULL,
			sender      TEXT,
			text        TEXT,
			timestamp   INTEGER,
			status      TEXT DEFAULT 'sent',
			channel     INTEGER DEFAULT 0,
			relay_path  TEXT,
			created_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_conv ON messages(node_addr, conv_id, timestamp);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON messages(node_addr, timestamp);

		CREATE TABLE IF NOT EXISTS peer_aliases (
			node_addr TEXT NOT NULL,
			peer_addr TEXT NOT NULL,
			alias     TEXT NOT NULL,
			PRIMARY KEY (node_addr, peer_addr)
		);
	`)
	return err
}

// upsertAlias inserts or updates a peer alias for a node.
func (m *MsgDB) upsertAlias(nodeAddr, peerAddr, alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		INSERT INTO peer_aliases (node_addr, peer_addr, alias)
		VALUES (?, ?, ?)
		ON CONFLICT(node_addr, peer_addr) DO UPDATE SET alias = excluded.alias
	`, nodeAddr, peerAddr, alias)
	return err
}

// deleteAlias removes a peer alias.
func (m *MsgDB) deleteAlias(nodeAddr, peerAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`DELETE FROM peer_aliases WHERE node_addr = ? AND peer_addr = ?`, nodeAddr, peerAddr)
	return err
}

// loadAliases returns all aliases for a node as a map[peerAddr]alias.
func (m *MsgDB) loadAliases(nodeAddr string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT peer_addr, alias FROM peer_aliases WHERE node_addr = ?`, nodeAddr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var peer, alias string
		if err := rows.Scan(&peer, &alias); err != nil {
			return nil, err
		}
		out[peer] = alias
	}
	return out, rows.Err()
}

// SetNodeAddr sets the node address scope for all queries.
func (m *MsgDB) SetNodeAddr(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeAddr = addr
}

// UpsertMessage inserts or updates a message. On conflict, updates status and relay_path.
func (m *MsgDB) UpsertMessage(msg StoredMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	relayJSON := "[]"
	if len(msg.RelayPath) > 0 {
		b, _ := json.Marshal(msg.RelayPath)
		relayJSON = string(b)
	}

	_, err := m.db.Exec(`
		INSERT INTO messages (id, node_addr, conv_id, direction, sender, text, timestamp, status, channel, relay_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status     = CASE WHEN excluded.status != '' THEN excluded.status ELSE status END,
			relay_path = CASE WHEN excluded.relay_path != '[]' THEN excluded.relay_path ELSE relay_path END
	`,
		msg.ID, msg.NodeAddr, msg.ConvID, msg.Direction, msg.Sender,
		msg.Text, msg.Timestamp, msg.Status, msg.Channel, relayJSON, msg.CreatedAt,
	)
	return err
}

// UpdateStatus updates just the status field for a message by ID.
func (m *MsgDB) UpdateStatus(id string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`UPDATE messages SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateRelayPath updates the relay_path field for a message by ID.
func (m *MsgDB) UpdateRelayPath(id string, path []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.Marshal(path)
	_, err := m.db.Exec(`UPDATE messages SET relay_path = ? WHERE id = ?`, string(b), id)
	return err
}

// LoadConversation returns paginated messages for a conversation, ordered by timestamp ASC.
// If beforeTimestamp is 0, returns the most recent `limit` messages.
func (m *MsgDB) LoadConversation(convID string, limit int, beforeTimestamp int64) ([]StoredMessage, error) {
	m.mu.Lock()
	nodeAddr := m.nodeAddr
	m.mu.Unlock()

	var rows *sql.Rows
	var err error
	if beforeTimestamp > 0 {
		rows, err = m.db.Query(`
			SELECT id, node_addr, conv_id, direction, sender, text, timestamp, status, channel, relay_path, created_at
			FROM messages
			WHERE node_addr = ? AND conv_id = ? AND timestamp < ?
			ORDER BY timestamp DESC
			LIMIT ?
		`, nodeAddr, convID, beforeTimestamp, limit)
	} else {
		rows, err = m.db.Query(`
			SELECT id, node_addr, conv_id, direction, sender, text, timestamp, status, channel, relay_path, created_at
			FROM messages
			WHERE node_addr = ? AND conv_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		`, nodeAddr, convID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse so oldest-first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// LoadConversations returns a summary for each known conversation.
func (m *MsgDB) LoadConversations() ([]ConversationSummary, error) {
	m.mu.Lock()
	nodeAddr := m.nodeAddr
	m.mu.Unlock()

	rows, err := m.db.Query(`
		SELECT conv_id,
			   MAX(timestamp) AS last_ts,
			   (SELECT text FROM messages m2 WHERE m2.node_addr = m.node_addr AND m2.conv_id = m.conv_id ORDER BY timestamp DESC LIMIT 1) AS last_text
		FROM messages m
		WHERE node_addr = ?
		GROUP BY conv_id
		ORDER BY last_ts DESC
	`, nodeAddr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ConversationSummary
	for rows.Next() {
		var s ConversationSummary
		var lastText sql.NullString
		if err := rows.Scan(&s.ID, &s.LastTimestamp, &lastText); err != nil {
			return nil, err
		}
		s.LastMessage = lastText.String
		s.Label = convIDToLabel(s.ID)
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// LoadRecent returns the most recent N messages across all conversations.
func (m *MsgDB) LoadRecent(limit int) ([]StoredMessage, error) {
	m.mu.Lock()
	nodeAddr := m.nodeAddr
	m.mu.Unlock()

	rows, err := m.db.Query(`
		SELECT id, node_addr, conv_id, direction, sender, text, timestamp, status, channel, relay_path, created_at
		FROM messages
		WHERE node_addr = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, nodeAddr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse so oldest-first.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// Close closes the underlying database.
func (m *MsgDB) Close() error {
	return m.db.Close()
}

func scanMessages(rows *sql.Rows) ([]StoredMessage, error) {
	var msgs []StoredMessage
	for rows.Next() {
		var msg StoredMessage
		var relayJSON sql.NullString
		err := rows.Scan(
			&msg.ID, &msg.NodeAddr, &msg.ConvID, &msg.Direction,
			&msg.Sender, &msg.Text, &msg.Timestamp, &msg.Status,
			&msg.Channel, &relayJSON, &msg.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if relayJSON.Valid && relayJSON.String != "" && relayJSON.String != "[]" {
			_ = json.Unmarshal([]byte(relayJSON.String), &msg.RelayPath)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

// convIDToLabel (local helper; mirrors the one in chat.go to avoid import cycle).
func convIDToLabel(id string) string {
	switch {
	case id == "broadcast":
		return "Broadcast"
	case strings.HasPrefix(id, "ch:"):
		return id
	case strings.HasPrefix(id, "dm:"):
		return id[3:]
	}
	return id
}

// convIDToTo (local helper; mirrors the one in chat.go to avoid import cycle).
func convIDToTo(id string) string {
	switch {
	case id == "broadcast" || id == "":
		return "broadcast"
	case strings.HasPrefix(id, "ch:"):
		return id
	case strings.HasPrefix(id, "dm:"):
		return id[3:]
	}
	return id
}
