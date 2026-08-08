package commands

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// A roll-call asks every member of the fleet to answer, and keeps a ledger of
// who did. Each answer carries an Ed25519 signature bound to that roll-call and
// to the responder's own identity key, so a row in the ledger is evidence a
// named member was alive and willing to say so, not just that a frame arrived
// somewhere.
//
// What the ledger may claim depends on the node's trust configuration, and both
// subcommands label it. An anchored node holds an anchor-certified roster, so
// "pinned" and "admitted to this fleet" are the same set and the ledger can
// name the members that stayed silent. An un-anchored node pins
// trust-on-first-use identities, which are free to mint: there is no
// authoritative roster, so the ledger reports the responders it observed and
// names nobody missing. Rendering the two the same way would let a partial
// answer read as a complete one.

func newRollCallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roll-call",
		Short: "Ask the fleet to sound off, and read the signed ledger",
		Long: "Start an attested roll-call and read its ledger.\n\n" +
			"The initiator floods a short operator payload; every member that hears it\n" +
			"answers with a signature bound to that roll-call and to its own identity\n" +
			"key. The ledger records who answered, how far into the roll-call, and over\n" +
			"which relay path.\n\n" +
			"A roll-call is the most expensive thing the protocol does, so the node rate\n" +
			"limits it and lets only one roll-call it started collect at a time.",
	}
	cmd.AddCommand(
		newRollCallStartCmd(),
		newRollCallStatusCmd(),
	)
	return cmd
}

func newRollCallStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start a roll-call from the connected node",
		Long: "Starts a roll-call and prints its id, schedule and expected-set size.\n\n" +
			"The payload is a flag, never a positional argument, so a stray word on the\n" +
			"command line cannot become the text the whole fleet is asked to answer. The\n" +
			"node bounds the payload size and rejects an oversized one outright; 'roll-call\n" +
			"status' reports the bound it enforces.\n\n" +
			"A node that declines says why: another roll-call it started is still\n" +
			"collecting, the start landed inside the enforced interval between two\n" +
			"roll-calls, or the announce never reached the air. The first two report how\n" +
			"long to wait, so a script waits a known interval instead of polling.\n\n" +
			"Answers arrive over the following minutes. Read them with 'roll-call status'.",
		Example: "  bramble roll-call start\n" +
			"  bramble roll-call start --text \"sound off\"\n" +
			"  bramble --port /dev/ttyUSB0 roll-call start --text \"drill\"",
		Args: cobra.NoArgs,
		RunE: runRollCallStart,
	}
	c.Flags().String("text", "", "operator payload the announce carries (the node bounds its size)")
	return c
}

func newRollCallStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the ledger of the roll-call this node started",
		Long: "Renders the ledger: which members answered with a verified signature, how\n" +
			"far into the roll-call each answered, the relay path where the broadcast\n" +
			"delivery-receipt machinery supplied one, and, on an anchored fleet, the\n" +
			"members that never answered.\n\n" +
			"The ledger stays readable after the collection window closes, so this is\n" +
			"also how a finished roll-call is read back.\n\n" +
			"It also reports for the node rather than for the roll-call: the payload size\n" +
			"it accepts, the interval it enforces between the roll-calls it starts, the\n" +
			"answers it emits in any rolling hour, and the answers it dropped or refused\n" +
			"while answering somebody else's roll-call. A node that has never started a\n" +
			"roll-call reports those and nothing else.",
		Args: cobra.NoArgs,
		RunE: runRollCallStatus,
	}
}

func runRollCallStart(cmd *cobra.Command, args []string) error {
	text, _ := cmd.Flags().GetString("text")

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.StartRollCall(ctx, text)
	if err != nil {
		return fmt.Errorf("bramble-cli: start roll-call: %w", err)
	}

	// A refusal is a successful call carrying ok:false, so the JSON shape is
	// the same either way and is printed before the outcome is judged: a
	// scripted caller gets the reason, the retry interval and the enforced
	// floor as data, and still gets a non-zero exit from the error below.
	if flagJSON {
		if err := output.PrintJSON(cmd.OutOrStdout(), resp); err != nil {
			return err
		}
	}
	if !resp.OK {
		return errors.New(describeRollCallRefusal(resp))
	}
	if flagJSON {
		return nil
	}
	printRollCallStarted(cmd.OutOrStdout(), resp)
	return nil
}

func runRollCallStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	ledger, err := client.RollCall(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get roll-call ledger: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(cmd.OutOrStdout(), ledger)
	}
	renderRollCallLedger(cmd.OutOrStdout(), ledger)
	return nil
}

// describeRollCallRefusal turns an ok:false start into one operator-readable
// line. Split out so every refusal reason is checked without a node: the reason
// is the whole value of the response, and a caller that cannot tell "wait five
// minutes" from "the radio never sent it" will retry the wrong one.
func describeRollCallRefusal(resp *bramble.StartRollCallResponse) string {
	var what string
	switch resp.Reason {
	case bramble.RollCallRefusalBusy:
		what = "a roll-call this node started is still collecting"
	case bramble.RollCallRefusalRateLimited:
		what = "rate limited"
		if resp.MinIntervalMs > 0 {
			what += fmt.Sprintf(", the node enforces %s between the roll-calls it starts",
				output.FormatMs(int64(resp.MinIntervalMs)))
		}
	case bramble.RollCallRefusalNotTransmitted:
		// Nothing went out, so nothing is owed an answer and the rate limiter
		// was not charged: a retry is free, unlike the two above.
		return "bramble-cli: roll-call refused: the announce never reached the air, " +
			"so nothing was asked of the fleet and the interval was not charged"
	case "":
		what = "the node gave no reason"
	default:
		what = fmt.Sprintf("the node reported %q", resp.Reason)
	}

	if resp.RetryAfterMs > 0 {
		return fmt.Sprintf("bramble-cli: roll-call refused: %s; retry in %s",
			what, output.FormatMs(int64(resp.RetryAfterMs)))
	}
	return fmt.Sprintf("bramble-cli: roll-call refused: %s", what)
}

// printRollCallStarted reports what was actually started: the id to read the
// ledger under, the schedule the answers will arrive over, and which of the two
// things this fleet can prove.
func printRollCallStarted(w io.Writer, resp *bramble.StartRollCallResponse) {
	fmt.Fprintf(w, "Roll-call %s started.\n", resp.RollCallID)
	fmt.Fprintf(w, "Rounds:  %d\n", resp.RoundsTotal)
	fmt.Fprintf(w, "Window:  %s until the ledger closes\n", output.FormatMs(int64(resp.WindowMs)))
	fmt.Fprintf(w, "Fleet:   %s\n", rollCallFleetLine(resp.Anchored, resp.Expected))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Answers arrive over the window above. Read them with: bramble roll-call status")
}

// rollCallFleetLine names the two modes the same way the web client does, so a
// ledger read in either place says the same thing about what it can prove.
func rollCallFleetLine(anchored bool, expected int) string {
	if anchored {
		return fmt.Sprintf("Anchored fleet, %d member(s) expected", expected)
	}
	return "Observed only, this node holds no anchor-certified roster so nobody can be called missing"
}

// renderRollCallLedger writes the human-readable ledger. It takes a writer
// rather than a client so every branch (never started, still collecting,
// closed, anchored, observed-only) is rendered in a test without a node.
func renderRollCallLedger(w io.Writer, l *bramble.RollCallLedger) {
	if !l.Active {
		fmt.Fprintln(w, "This node has never started a roll-call.")
		fmt.Fprintln(w, "Start one with: bramble roll-call start")
		// The bounds and the member-side counters describe the node, not any
		// roll-call it started, so the firmware reports them here too. A member
		// that only ever answers somebody else's roll-call never leaves this
		// branch, and its dropped or refused answers are the whole evidence of
		// why it failed to take part.
		fmt.Fprintln(w)
		if l.MaxTextBytes > 0 {
			fmt.Fprintf(w, "Payload:    this node accepts up to %d bytes\n", l.MaxTextBytes)
		}
		renderRollCallSelfLimits(w, l)
		renderRollCallCounters(w, l)
		return
	}

	fmt.Fprintf(w, "Roll-call:  %s\n", l.RollCallID)
	fmt.Fprintf(w, "State:      %s\n", describeRollCallState(l))
	fmt.Fprintf(w, "Payload:    %s\n", describeRollCallPayload(l))
	fmt.Fprintf(w, "Fleet:      %s\n", rollCallFleetLine(l.Anchored, l.Expected))
	fmt.Fprintf(w, "Answered:   %s\n", describeRollCallAnswered(l))
	renderRollCallSelfLimits(w, l)
	fmt.Fprintln(w)

	if len(l.Responders) == 0 {
		fmt.Fprintln(w, "No member has answered yet.")
	} else {
		headers := []string{"ADDRESS", "ANSWER", "AT", "ROUND", "HOPS", "RELAY PATH"}
		rows := make([][]string, len(l.Responders))
		for i, r := range l.Responders {
			rows[i] = rollCallResponderRow(r)
		}
		output.Table(w, headers, rows)
	}

	renderRollCallMissing(w, l)
	renderRollCallCounters(w, l)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "A verified answer proves the holder of that address's identity key heard this")
	fmt.Fprintln(w, "roll-call and chose to answer. Silence proves nothing on its own: a member may")
	fmt.Fprintln(w, "be switched off, out of range, out of airtime budget, or behind a relay that")
	fmt.Fprintln(w, "dropped the frame. A relay path comes from the delivery-receipt machinery and")
	fmt.Fprintln(w, "is a hint about how the announce travelled, not part of the attestation.")
}

// renderRollCallSelfLimits reports the two budgets this node spends on its own
// behalf: how often it will start a roll-call, and how many answers it will
// emit for everybody else's. Both are node properties rather than properties of
// a roll-call, so they read the same whether or not one is running.
func renderRollCallSelfLimits(w io.Writer, l *bramble.RollCallLedger) {
	if l.MinIntervalMs > 0 {
		fmt.Fprintf(w, "Interval:   this node starts a roll-call at most once every %s\n",
			output.FormatMs(int64(l.MinIntervalMs)))
	}
	if l.AnswerMaxPerHour > 0 {
		fmt.Fprintf(w, "Answers:    this node emits at most %d answer(s) in any rolling hour\n",
			l.AnswerMaxPerHour)
	}
}

// describeRollCallState reports where the roll-call is against its own
// schedule. Times are milliseconds into the roll-call, never device uptime,
// which is boot-relative and means nothing to a client.
func describeRollCallState(l *bramble.RollCallLedger) string {
	if l.Open {
		return fmt.Sprintf("collecting, %s of %s elapsed, round %d of %d sent",
			output.FormatMs(l.ElapsedMs), output.FormatMs(int64(l.WindowMs)),
			l.RoundsSent, l.RoundsTotal)
	}
	return fmt.Sprintf("closed after %s and %d of %d round(s)",
		output.FormatMs(int64(l.WindowMs)), l.RoundsSent, l.RoundsTotal)
}

// describeRollCallPayload reports the operator payload alongside the bound the
// node enforces on it, which is the only place a client learns that bound: an
// oversized payload is rejected as a malformed request, not refused with a
// number to aim at.
func describeRollCallPayload(l *bramble.RollCallLedger) string {
	text := "none"
	if l.Text != "" {
		text = strconv.Quote(l.Text)
	}
	if l.MaxTextBytes > 0 {
		return fmt.Sprintf("%s (this node accepts up to %d bytes)", text, l.MaxTextBytes)
	}
	return text
}

// describeRollCallAnswered puts the count in the only frame it is true in. An
// un-anchored node has no denominator to report against, so it does not invent
// one out of the addresses it happens to have heard from.
func describeRollCallAnswered(l *bramble.RollCallLedger) string {
	if l.Anchored {
		return fmt.Sprintf("%d of %d expected member(s)", l.Responded, l.Expected)
	}
	return fmt.Sprintf("%d member(s), out of an unknown total", l.Responded)
}

// rollCallResponderRow renders one ledger row. A row can exist without an
// answer: a delivery receipt named a pinned member that never answered, which
// is a hint about the announce's path and not evidence of a response, so the
// answer column says so rather than leaving the row to read as a half answer.
func rollCallResponderRow(r bramble.RollCallResponder) []string {
	answer, at, round := "no answer", "-", "-"
	if r.Responded {
		answer = "verified"
		at = output.FormatMs(r.AtMs)
		round = strconv.Itoa(r.Round)
	}
	hops := "-"
	if r.Hops > 0 {
		hops = strconv.Itoa(r.Hops)
	}
	return []string{r.Address, answer, at, round, hops, formatRelayPath(r.Path)}
}

// formatRelayPath renders the initiator-to-responder path a delivery receipt
// reported. An empty path means no receipt supplied one, which is not the same
// as a direct link, so it renders as a dash rather than as zero hops.
func formatRelayPath(path []string) string {
	if len(path) == 0 {
		return "-"
	}
	return strings.Join(path, " > ")
}

// renderRollCallMissing names the silent members, and only when the node can
// honestly do so. On an un-anchored fleet the firmware leaves the set empty by
// construction and this prints nothing.
func renderRollCallMissing(w io.Writer, l *bramble.RollCallLedger) {
	// The count and the names are separate fields, so the header takes whichever
	// is larger: a named member must never be left out of the count, and a count
	// larger than the names is a table that could not hold them all.
	count := l.MissingCount
	if len(l.Missing) > count {
		count = len(l.Missing)
	}
	if count == 0 {
		return
	}
	fmt.Fprintln(w)
	if len(l.Missing) == 0 {
		fmt.Fprintf(w, "No answer (%d): the node reported the count without naming them\n", count)
		return
	}
	fmt.Fprintf(w, "No answer (%d): %s\n", count, strings.Join(l.Missing, ", "))
}

// renderRollCallCounters reports the answers that did not become rows. Each one
// is a different reason a member is absent from the table above, and a ledger
// that dropped them silently would read as a smaller fleet rather than an
// incomplete count.
func renderRollCallCounters(w io.Writer, l *bramble.RollCallLedger) {
	var lines []string
	if l.Unattested > 0 {
		lines = append(lines, fmt.Sprintf(
			"%d answer(s) could not be attested (no pinned key for the responder, a signature that did not verify, or a responder that did not match its envelope); counted, never recorded",
			l.Unattested))
	}
	if l.Overflow > 0 {
		lines = append(lines, fmt.Sprintf(
			"%d answer(s) did not fit this ledger's table: more members answered than it holds", l.Overflow))
	}
	if l.Late > 0 {
		lines = append(lines, fmt.Sprintf(
			"%d answer(s) arrived after the ledger closed", l.Late))
	}
	if l.PendingDropped > 0 {
		lines = append(lines, fmt.Sprintf(
			"%d answer(s) THIS node could not queue: its own pending-answer queue was full", l.PendingDropped))
	}
	if l.AnswerLimited > 0 {
		lines = append(lines, fmt.Sprintf(
			"%d answer(s) THIS node refused: it had already spent its budget of %d per rolling hour",
			l.AnswerLimited, l.AnswerMaxPerHour))
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(w)
	for _, line := range lines {
		fmt.Fprintf(w, "Note: %s\n", line)
	}
}
