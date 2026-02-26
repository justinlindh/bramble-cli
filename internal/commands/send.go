package commands

import (
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send <address> <message>",
		Short: "Send a unicast message",
		Long: `Send a text message to a specific mesh node address.

The address can be specified as a hex string (e.g. DEADBEEF or 0xDEADBEEF)
or as a decimal integer.

Example:
  bramble send DEADBEEF "hello there"
  bramble send 0xCAFEBABE "check in"`,
		Args: cobra.ExactArgs(2),
		RunE: runSend,
	}
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

	result, err := client.Send(ctx, dest, text)
	if err != nil {
		return fmt.Errorf("bramble-cli: send: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, SendCommandResult{Dest: output.Addr(dest), Text: text, Status: result.Status})
	}

	fmt.Fprintf(os.Stdout, "Sent → %s (%s)\n", output.Addr(dest), result.Status)
	return nil
}
