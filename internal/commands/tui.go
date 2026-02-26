package commands

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/justinlindh/bramble-cli/internal/tui"
	"github.com/spf13/cobra"
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
			t = transport.NewBLE(transport.BLEConfig{DeviceName: tBLE})
		case tURL != "":
			t = transport.NewWebSocket(tURL)
		case tPort != "":
			t = transport.NewSerial(tPort)
		default:
			t = transport.NewWebSocket("ws://bramble.local/ws")
		}
		c := bramble.NewClient(t)
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		return c, nil
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
	}

	model := tui.New(client, node, connectFn)

	p := tea.NewProgram(model)
	model.SetProgram(p)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("bramble tui: %w", err)
	}
	return nil
}
