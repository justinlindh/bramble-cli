package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"

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
	ctx := context.Background()

	addrStr := args[0]
	text := args[1]

	dest, err := parseAddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addrStr, err)
	}

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.Send(ctx, dest, text)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{
			"dest":      output.Addr(dest),
			"text":      text,
			"packet_id": result.PacketID,
		})
	}

	fmt.Fprintf(os.Stdout, "Sent: packet#%d → %s\n", result.PacketID, output.Addr(dest))
	return nil
}

// parseAddr parses a mesh address from a hex or decimal string.
// Accepts: "DEADBEEF", "0xDEADBEEF", "3735928559"
func parseAddr(s string) (uint32, error) {
	// Strip leading 0x or 0X.
	hex := s
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		hex = s[2:]
	}

	// Try hex first (most common for mesh addresses).
	n, err := strconv.ParseUint(hex, 16, 32)
	if err == nil {
		return uint32(n), nil
	}

	// Fall back to decimal.
	n, err = strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("expected hex (e.g. DEADBEEF) or decimal integer")
	}
	return uint32(n), nil
}
