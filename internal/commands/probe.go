package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newProbeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Send a network probe and wait for responses",
		Long: `Broadcast a probe packet and wait for the ack_window to collect responses.

The node broadcasts a probe and returns an ack_window — the duration during
which peer responses are expected. This command waits for that window to
elapse (or for an explicit --timeout), collecting and displaying each
ProbeResult as it arrives.

Example:
  bramble probe
  bramble probe --timeout 15s`,
		RunE: runProbe,
	}
	cmd.Flags().Duration("timeout", 0, "override ack_window wait duration (e.g. 10s); 0 = use ack_window from node")
	return cmd
}

func runProbe(cmd *cobra.Command, args []string) error {
	timeoutOverride, _ := cmd.Flags().GetDuration("timeout")

	// Use a short-lived context just for the RPC call itself.
	rpcCtx, rpcCancel := commandContext()
	defer rpcCancel()

	client, err := getClient(rpcCtx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.SendProbe(rpcCtx)
	if err != nil {
		return fmt.Errorf("bramble-cli: probe: %w", err)
	}

	// Determine how long to wait for responses.
	waitDur := probeWaitDuration(timeoutOverride, result.AckWindow)

	if !flagJSON {
		fmt.Fprintf(os.Stdout, "Probe sent: ID=%d  ack_window=%dms\n",
			result.ProbeID, result.AckWindow*1000)
		fmt.Fprintf(os.Stdout, "Waiting %s for responses...\n", waitDur)
	}

	// Collect probe responses.
	type probeResultEntry struct {
		result    bramble.ProbeResult
		arrivedAt time.Time
	}
	var responses []probeResultEntry
	resultCh := make(chan bramble.ProbeResult, 32)
	completeCh := make(chan bramble.ProbeComplete, 1)

	client.OnProbeResult(func(r bramble.ProbeResult) {
		select {
		case resultCh <- r:
		default:
		}
	})
	client.OnProbeComplete(func(c bramble.ProbeComplete) {
		select {
		case completeCh <- c:
		default:
		}
	})

	// Wait context: use background + signal handling for responsiveness.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), waitDur)
	defer waitCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

waitLoop:
	for {
		select {
		case r := <-resultCh:
			responses = append(responses, probeResultEntry{result: r, arrivedAt: time.Now()})
			if !flagJSON {
				rtt := time.Duration(r.LatencyMs) * time.Millisecond
				fmt.Fprintf(os.Stdout, "  RESPONSE  addr=%-12s  hops=%d  rssi=%d  snr=%.1f  rtt=%s\n",
					r.Address, r.Hops, r.RSSI, r.SNR, rtt)
			}
		case <-completeCh:
			// Drain any buffered results before exiting.
		drainLoop:
			for {
				select {
				case r := <-resultCh:
					responses = append(responses, probeResultEntry{result: r, arrivedAt: time.Now()})
					if !flagJSON {
						rtt := time.Duration(r.LatencyMs) * time.Millisecond
						fmt.Fprintf(os.Stdout, "  RESPONSE  addr=%-12s  hops=%d  rssi=%d  snr=%.1f  rtt=%s\n",
							r.Address, r.Hops, r.RSSI, r.SNR, rtt)
					}
				default:
					break drainLoop
				}
			}
			break waitLoop
		case <-waitCtx.Done():
			break waitLoop
		case <-sigCh:
			break waitLoop
		}
	}

	// Build result list for output.
	probeResponses := make([]bramble.ProbeResult, 0, len(responses))
	for _, e := range responses {
		probeResponses = append(probeResponses, e.result)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, ProbeCommandResult{
			ProbeID:   result.ProbeID,
			AckWindow: result.AckWindow,
			Responses: probeResponses,
		})
	}

	fmt.Fprintf(os.Stdout, "\nProbe complete: %d node(s) responded\n", len(responses))
	return nil
}

// probeWaitDuration returns the duration to wait for probe responses.
// If override is non-zero it takes precedence; otherwise ackWindowSec from the
// firmware is used. A fallback of 10s applies when both are zero.
func probeWaitDuration(override time.Duration, ackWindowSec int) time.Duration {
	if override > 0 {
		return override
	}
	if ackWindowSec > 0 {
		return time.Duration(ackWindowSec) * time.Second
	}
	return 10 * time.Second
}
