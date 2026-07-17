package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/justinlindh/bramble-cli/internal/devices"
	"github.com/justinlindh/bramble-cli/internal/output"
)

func newDevicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Manage saved device aliases (address book)",
		Long: `Manage the local address book of Bramble devices.

Each entry maps a short alias to a WebSocket host and, optionally, that device's
auth token. Connect by alias from any command with --device/-d <alias>, or launch
the TUI with: bramble tui <alias>.

The book is stored as JSON under ~/.config/bramble/devices.json (respecting
XDG_CONFIG_HOME), file mode 0600. Tokens are stored in plaintext on disk, so
protect that file accordingly; token values are always masked in output here.

Subcommands: add, list, rm`,
	}
	cmd.AddCommand(newDevicesAddCmd(), newDevicesListCmd(), newDevicesRmCmd())
	return cmd
}

func newDevicesAddCmd() *cobra.Command {
	var token, name, port string
	cmd := &cobra.Command{
		Use:   "add <alias> <host>",
		Short: "Save a device alias",
		Long: `Save a device under a short alias.

<host> may be a bare address (198.51.100.65), a host:port, or a full ws:// URL;
a bare address is expanded to ws://<host>/ws.

If --token is omitted and the terminal is interactive, you are prompted for the
token (input hidden). Provide --token "" or run non-interactively to store no
token.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, host := args[0], args[1]
			if err := devices.ValidateAlias(alias); err != nil {
				return fmt.Errorf("bramble devices add: %w", err)
			}

			// Prompt for the token when not supplied and interactive.
			if !cmd.Flags().Changed("token") && isInteractive() {
				entered, err := promptSecret(cmd, fmt.Sprintf("Token for %s (leave blank for none): ", alias))
				if err != nil {
					return fmt.Errorf("bramble devices add: %w", err)
				}
				token = entered
			}

			path, err := devices.DefaultPath()
			if err != nil {
				return err
			}
			book, err := devices.Load(path)
			if err != nil {
				return err
			}
			if err := book.Add(alias, devices.Entry{Host: host, Token: token, Name: name, Port: port}); err != nil {
				return fmt.Errorf("bramble devices add: %w", err)
			}
			if err := book.Save(path); err != nil {
				return err
			}

			e, _ := book.Get(alias)
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %q -> %s (token: %s)\n", alias, e.Host, devices.MaskToken(e.Token))
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "auth token for the device")
	cmd.Flags().StringVar(&name, "name", "", "human-friendly name")
	cmd.Flags().StringVar(&port, "port", "", "serial port path (informational)")
	return cmd
}

func newDevicesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved device aliases (tokens masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := devices.DefaultPath()
			if err != nil {
				return err
			}
			book, err := devices.Load(path)
			if err != nil {
				return err
			}
			entries := book.List()

			if flagJSON {
				type row struct {
					Alias string `json:"alias"`
					Host  string `json:"host"`
					Name  string `json:"name,omitempty"`
					Token string `json:"token"`
				}
				rows := make([]row, 0, len(entries))
				for _, ne := range entries {
					rows = append(rows, row{
						Alias: ne.Alias,
						Host:  ne.Entry.Host,
						Name:  ne.Entry.Name,
						Token: devices.MaskToken(ne.Entry.Token), // never emit the real token
					})
				}
				return output.PrintJSON(cmd.OutOrStdout(), rows)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No saved devices. Add one with: bramble devices add <alias> <host>")
				return nil
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ALIAS\tNAME\tHOST\tTOKEN")
			for _, ne := range entries {
				name := ne.Entry.Name
				if name == "" {
					name = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ne.Alias, name, ne.Entry.Host, devices.MaskToken(ne.Entry.Token))
			}
			return tw.Flush()
		},
	}
}

func newDevicesRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <alias>",
		Aliases: []string{"remove"},
		Short:   "Remove a saved device alias",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
			path, err := devices.DefaultPath()
			if err != nil {
				return err
			}
			book, err := devices.Load(path)
			if err != nil {
				return err
			}
			if !book.Remove(alias) {
				return fmt.Errorf("bramble devices rm: no such alias %q", alias)
			}
			if err := book.Save(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %q\n", alias)
			return nil
		},
	}
}

// isInteractive reports whether stdin is a terminal (so prompting is sensible).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptSecret writes prompt to stderr and reads one line from stdin with echo
// disabled. The trailing newline the user types is consumed.
func promptSecret(cmd *cobra.Command, prompt string) (string, error) {
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
