package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/devices"
	"github.com/justinlindh/bramble-cli/internal/tui"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui [alias]",
		Short: "Launch interactive terminal UI",
		Long: `Launch the Bramble terminal UI with tabbed views for Chat, Nodes, Location, Config, and Stats.

With a saved-device alias (bramble tui <alias>), it connects straight to that
device using its stored host and token. With no target and an interactive
terminal, a picker lists your saved devices (see: bramble devices).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runTUI,
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Resolve which device to connect to: explicit alias arg, an already-set
	// transport flag, or (interactive + saved devices) a graphical picker.
	cancelled, err := resolveTUITarget(cmd, args)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}

	client, err := connectTUIClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	// Determine transport URL for display now that the target is resolved.
	transportURL := flagTransport
	switch {
	case transportURL != "":
	case resolvedDevice != nil:
		transportURL = resolvedDevice.Host
	case flagPort != "":
		transportURL = flagPort
	default:
		transportURL = "serial (auto)"
	}

	// Build reconnect factory. Capture the resolved alias host so reconnects
	// target the same device as the initial connect.
	tURL := flagTransport
	tPort := flagPort
	tBLE := flagBLE
	tDev := ""
	if resolvedDevice != nil {
		tDev = resolvedDevice.Host
	}
	connectFn := func(ctx context.Context) (*bramble.Client, error) {
		var t transport.Transport
		switch {
		case tBLE != "":
			t = transport.NewBLE(tBLE)
		case tURL != "":
			t = transport.NewWebSocket(tURL)
		case tPort != "":
			t = transport.NewSerial(tPort)
		case tDev != "":
			t = transport.NewWebSocket(tDev)
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

	// Fetch identity for the header (auth was already verified during connect).
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

// resolveTUITarget decides what the TUI connects to. It may set flagDevice
// (from a positional alias or the picker). It returns cancelled=true when the
// user dismissed the picker without choosing, meaning the caller should exit
// cleanly.
func resolveTUITarget(cmd *cobra.Command, args []string) (cancelled bool, err error) {
	if len(args) == 1 {
		flagDevice = args[0]
		return false, nil
	}
	// An explicit target already selects a device; skip the picker.
	if flagDevice != "" || flagTransport != "" || flagPort != "" || flagBLE != "" {
		return false, nil
	}
	if !isInteractive() {
		return false, nil
	}

	path, err := devices.DefaultPath()
	if err != nil {
		return false, err
	}
	book, err := devices.Load(path)
	if err != nil {
		return false, err
	}
	if len(book.List()) == 0 {
		// Empty book: keep the previous behavior (serial auto-detect).
		return false, nil
	}

	// The picker can select, add, and delete devices; it persists changes to
	// path itself, so getClient re-reads the book when it resolves the alias.
	res, runErr := tea.NewProgram(tui.NewPicker(book, path)).Run()
	if runErr != nil {
		return false, fmt.Errorf("bramble tui: device picker: %w", runErr)
	}
	pm, ok := res.(tui.PickerModel)
	if !ok || pm.Quit() || pm.Choice() == "" {
		return true, nil
	}
	flagDevice = pm.Choice()
	return false, nil
}

// connectTUIClient connects to the resolved target and verifies the connection
// is authorized. If the node requires a token we do not have and the terminal
// is interactive, it prompts (hidden) for the token, retries, and offers to
// save it. Otherwise it fails fast with an actionable message rather than
// launching a TUI that would flap forever on "Unauthorized" polls.
func connectTUIClient(cmd *cobra.Command) (*bramble.Client, error) {
	client, err := dialTUIClient()
	if err != nil {
		return nil, fmt.Errorf("bramble tui: connect: %w", err)
	}

	verr := verifyTUIAuth(client)
	if verr == nil {
		return client, nil
	}
	if !isAuthError(verr) {
		// Transient/non-auth probe failure: keep the client and let the TUI
		// proceed, matching the previous lenient behavior.
		return client, nil
	}

	// The node rejected an authenticated RPC.
	if resolvedAuthToken() != "" {
		// A token was supplied but rejected: prompting again would not help.
		client.Close()
		return nil, fmt.Errorf("bramble tui: token rejected by node (check --token, BRAMBLE_TOKEN, or the saved device)")
	}
	if !isInteractive() {
		client.Close()
		return nil, authRequiredErr()
	}

	token, perr := promptSecret(cmd, "Device auth token: ")
	if perr != nil {
		client.Close()
		return nil, fmt.Errorf("bramble tui: %w", perr)
	}
	if token == "" {
		client.Close()
		return nil, authRequiredErr()
	}
	client.Close()

	// Thread the entered token through applyAuthToken on the retry.
	flagAuthToken = token
	client, err = dialTUIClient()
	if err != nil {
		return nil, fmt.Errorf("bramble tui: connect: %w", err)
	}
	if verr := verifyTUIAuth(client); verr != nil {
		client.Close()
		if isAuthError(verr) {
			return nil, fmt.Errorf("bramble tui: token rejected by node")
		}
		return nil, fmt.Errorf("bramble tui: %w", verr)
	}

	offerSaveToken(cmd, token)
	return client, nil
}

// dialTUIClient connects a client using the standard transport/token resolution.
func dialTUIClient() (*bramble.Client, error) {
	ctx, cancel := commandContext()
	defer cancel()
	return getClient(ctx)
}

// verifyTUIAuth probes a non-allowlisted RPC to confirm the connection is
// authorized. A nil result means authorized; isAuthError distinguishes an auth
// rejection from other failures.
func verifyTUIAuth(client *bramble.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Identity(ctx)
	return err
}

func authRequiredErr() error {
	return fmt.Errorf("bramble tui: node requires authentication; provide a token with " +
		"--token, the BRAMBLE_TOKEN environment variable, or a saved device (bramble devices add)")
}

// offerSaveToken asks (interactively) whether to persist a freshly-entered
// token to the address book. Failures to save are reported but never fatal.
func offerSaveToken(cmd *cobra.Command, token string) {
	host := ""
	switch {
	case resolvedDevice != nil:
		host = resolvedDevice.Host
	case flagTransport != "":
		host = flagTransport
	}
	if host == "" {
		return
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(cmd.ErrOrStderr(), "Save this token for next time? [y/N]: ")
	ans, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(ans)) != "y" {
		return
	}

	alias := flagDevice
	if alias == "" {
		fmt.Fprint(cmd.ErrOrStderr(), "Alias to save under: ")
		a, _ := reader.ReadString('\n')
		alias = strings.TrimSpace(a)
	}

	path, err := devices.DefaultPath()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Not saved: %v\n", err)
		return
	}
	book, err := devices.Load(path)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Not saved: %v\n", err)
		return
	}
	entry := devices.Entry{Host: host, Token: token}
	if existing, ok := book.Get(alias); ok {
		entry.Name = existing.Name
		entry.Port = existing.Port
	}
	if err := book.Add(alias, entry); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Not saved: %v\n", err)
		return
	}
	if err := book.Save(path); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Not saved: %v\n", err)
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Saved token under alias %q.\n", alias)
}
