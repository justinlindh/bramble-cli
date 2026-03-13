package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/tui"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		Long:  "Launch the Bramble terminal UI with tabbed views for Chat, Nodes, Location, Config, and Stats.",
		RunE:  runTUI,
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Use a longer context for TUI (not the short requestTimeout).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := getClient(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("bramble tui: connect: %w", err)
	}
	defer client.Close()

	// Determine transport URL for display.
	transportURL := flagTransport
	if transportURL == "" && flagPort != "" {
		transportURL = flagPort
	} else if transportURL == "" {
		transportURL = "serial (auto)"
	}

	// Build reconnect factory.
	tURL := flagTransport
	tPort := flagPort
	tBLE := flagBLE
	connectFn := func(ctx context.Context) (*bramble.Client, error) {
		var t transport.Transport
		switch {
		case tBLE != "":
			t = transport.NewBLE(tBLE)
		case tURL != "":
			t = transport.NewWebSocket(tURL)
		case tPort != "":
			t = transport.NewSerial(tPort)
		default:
			t = transport.NewWebSocket("ws://bramble.local/ws")
		}
		applyAuthToken(t)
		c := bramble.NewClient(t)
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		return c, nil
	}

	// Open message DB (non-fatal: TUI works without persistence).
	var msgdb *tui.MsgDB
	dbPath, err := tui.DefaultDBPath()
	if err == nil {
		msgdb, err = tui.NewMsgDB(dbPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: msgdb open failed: %v\n", err)
			msgdb = nil
		}
	}
	if msgdb != nil {
		defer msgdb.Close()
	}

	// Fetch identity for the header.
	node := tui.NodeInfo{
		Transport: transportURL,
		Connected: true,
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fetchCancel()

	if identity, err := client.Identity(fetchCtx); err == nil {
		node.Address = identity.Address
		if msgdb != nil {
			msgdb.SetNodeAddr(identity.Address)
		}
	}
	if cfg, err := client.Config(fetchCtx); err == nil {
		node.Name = strings.TrimSpace(cfg.NodeName)
	}

	// Pre-populate store from DB before connecting live notifications.
	model := tui.New(client, node, connectFn, msgdb)

	if msgdb != nil {
		// Load recent messages from DB into the chat tab pre-connection.
		model.PreloadFromDB(msgdb)
	}

	// Fetch live messages from node and upsert into DB.
	if msgdb != nil {
		liveCtx, liveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		msgs, err := client.Messages(liveCtx)
		liveCancel()
		if err == nil {
			for _, msg := range msgs {
				// Classify the message into a conv.
				convID := tui.ClassifyMessageConvID(msg, node.Address)
				direction := "in"
				if msg.From == node.Address || msg.From == "" {
					direction = "out"
				}
				sm := tui.StoredMessageFromBramble(msg, node.Address, convID, direction)
				_ = msgdb.UpsertMessage(sm)
			}
		}
	}

	p := tea.NewProgram(model)
	model.SetProgram(p)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("bramble tui: %w", err)
	}
	return nil
}
