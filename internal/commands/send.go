package commands

import (
	"fmt"
	"os"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

var sendCritical bool

func newSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <address> <message>",
		Short: "Send a unicast message",
		Long: `Send a text message to a specific mesh node address.

The address can be specified as a hex string (e.g. DEADBEEF or 0xDEADBEEF)
or as a decimal integer.

Use --critical to send with critical priority (bypasses normal airtime budgets).

Example:
  bramble send DEADBEEF "hello there"
  bramble send --critical DEADBEEF "emergency alert"`,
		Args: cobra.ExactArgs(2),
		RunE: runSend,
	}
	cmd.Flags().BoolVar(&sendCritical, "critical", false, "send with critical priority")
	return cmd
}

func runSend(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()

	addrStr := args[0]
	text := args[1]

	dest, err := ParseAddress(addrStr)
	if err != nil {
		return fmt.Errorf("bramble-cli: invalid address %q: %w", addrStr, err)
	}

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	var result *bramble.SendResult
	if sendCritical {
		result, err = client.SendCritical(ctx, dest, text)
	} else {
		result, err = client.Send(ctx, dest, text)
	}
	if err != nil {
		return fmt.Errorf("bramble-cli: send: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, SendCommandResult{Dest: output.Addr(dest), Text: text, Status: result.Status})
	}

	fmt.Fprintf(os.Stdout, "Sent → %s (%s)\n", output.Addr(dest), result.Status)
	return nil
}
