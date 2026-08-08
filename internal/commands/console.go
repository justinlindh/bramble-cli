package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"go.bug.st/serial"

	"github.com/justinlindh/bramble-cli/internal/discovery"
)

// serialOpen is indirected for tests.
var serialOpen = serial.Open

func newConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Tail the firmware serial console (log output)",
		Long: `Stream the node's serial console: the ESP-IDF log output, not RPC events.

This is a different stream from "bramble monitor", which subscribes to RPC
notifications. Plenty of firmware behavior is only ever reported to the console,
including the reason a directed send was dropped, so this is where a silent
failure shows up.

Opening the port does NOT reset the node. On a CP2102 board, DTR and RTS drive
EN and BOOT, and asserting either at open reboots the device, which makes a
healthy node look like it is stuck in a boot loop. Both lines are left
deasserted, so this is a read-only attach to a running device.

By default the JSON-RPC frames the device also writes to this port are hidden,
since they are noise when you are reading logs; pass --raw to see everything.

Examples:
  bramble console
  bramble --port /dev/ttyUSB0 console
  bramble console --grep 'location|dm_session'
  bramble console --duration 30s`,
		RunE: runConsole,
	}
	cmd.Flags().Int("baud", 115200, "serial baud rate")
	cmd.Flags().String("grep", "", "regex filter applied to each console line")
	cmd.Flags().Duration("duration", 0, "stop after this long (default: run until Ctrl+C)")
	cmd.Flags().Bool("raw", false, "also print the JSON-RPC frames on the port")
	return cmd
}

func runConsole(cmd *cobra.Command, args []string) error {
	baud, _ := cmd.Flags().GetInt("baud")
	grepPattern, _ := cmd.Flags().GetString("grep")
	duration, _ := cmd.Flags().GetDuration("duration")
	raw, _ := cmd.Flags().GetBool("raw")

	var grep *regexp.Regexp
	if grepPattern != "" {
		var err error
		grep, err = regexp.Compile(grepPattern)
		if err != nil {
			return fmt.Errorf("bramble-cli: invalid --grep regex: %w", err)
		}
	}

	port := flagPort
	if port == "" {
		detected, err := discovery.Detect()
		if err != nil {
			return err
		}
		port = detected
		fmt.Fprintf(cmd.ErrOrStderr(), "Auto-detected device: %s\n", port)
	}

	conn, err := serialOpen(port, consoleSerialMode(baud))
	if err != nil {
		return fmt.Errorf("bramble-cli: open %s: %w", port, err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if duration > 0 {
		var timerCancel context.CancelFunc
		ctx, timerCancel = context.WithTimeout(ctx, duration)
		defer timerCancel()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Closing the port unblocks the read in streamConsole, which otherwise
	// sits in Scan until the device says something.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	err = streamConsole(conn, cmd.OutOrStdout(), consoleFilter{grep: grep, raw: raw})
	if ctx.Err() != nil {
		// The read failed because we closed the port on purpose.
		return nil
	}
	return err
}

// consoleSerialMode is the port configuration for a read-only attach.
//
// InitialStatusBits is the load-bearing field. go.bug.st/serial defaults it to
// nil, which it documents as DTR=true and RTS=true, and on a CP2102 those lines
// drive EN and BOOT: asserting them at open reboots the node. A log tail that
// reset the device every time it attached would destroy the evidence it was
// opened to read.
func consoleSerialMode(baud int) *serial.Mode {
	return &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		InitialStatusBits: &serial.ModemOutputBits{
			DTR: false,
			RTS: false,
		},
	}
}

type consoleFilter struct {
	grep *regexp.Regexp
	raw  bool
}

// keep decides whether a line from the port is shown.
func (f consoleFilter) keep(line string) bool {
	if !f.raw && isRPCFrame(line) {
		return false
	}
	if f.grep != nil && !f.grep.MatchString(line) {
		return false
	}
	return true
}

// isRPCFrame reports whether a line is a JSON-RPC frame rather than a log line.
// The device multiplexes both onto one port, and a JSON object is the only
// thing the RPC layer ever writes.
func isRPCFrame(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "{")
}

// streamConsole copies filtered lines from r to w until r fails or closes.
// Split from runConsole so the filtering and line handling are testable against
// an ordinary reader, with no port involved.
func streamConsole(r io.Reader, w io.Writer, f consoleFilter) error {
	scanner := bufio.NewScanner(r)
	// ESP-IDF backtraces and long log lines exceed bufio's 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		if !f.keep(line) {
			continue
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("bramble-cli: write console output: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("bramble-cli: read console: %w", err)
	}
	return nil
}
