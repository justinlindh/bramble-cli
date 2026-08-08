package commands

import (
	"bytes"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewRollCallCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRollCallCmd()
	for _, name := range []string{"start", "status"} {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

// The payload is what the whole fleet is asked to answer, so it is a flag and
// never a positional: a stray word after 'start' must fail loudly rather than
// become the announce text.
func TestRollCallStart_RejectsPositionalPayload(t *testing.T) {
	t.Parallel()

	cmd := newRollCallStartCmd()
	if err := cmd.Args(cmd, []string{"sound off"}); err == nil {
		t.Fatal("expected the command to reject a positional payload")
	}
	if cmd.Flags().Lookup("text") == nil {
		t.Fatal("expected a --text flag to carry the payload")
	}
}

func TestDescribeRollCallRefusal_Busy(t *testing.T) {
	t.Parallel()

	got := describeRollCallRefusal(&bramble.StartRollCallResponse{
		Reason:       bramble.RollCallRefusalBusy,
		RetryAfterMs: 95000,
	})
	if !strings.Contains(got, "still collecting") {
		t.Errorf("refusal %q does not say a roll-call is already collecting", got)
	}
	if !strings.Contains(got, "1m35s") {
		t.Errorf("refusal %q does not carry the retry interval", got)
	}
}

func TestDescribeRollCallRefusal_RateLimitedCarriesBothIntervals(t *testing.T) {
	t.Parallel()

	// Two different numbers, and an operator needs both: how long until this
	// start would be accepted, and the floor that will apply to the next one.
	got := describeRollCallRefusal(&bramble.StartRollCallResponse{
		Reason:        bramble.RollCallRefusalRateLimited,
		RetryAfterMs:  200000,
		MinIntervalMs: 300000,
	})
	if !strings.Contains(got, "retry in 3m20s") {
		t.Errorf("refusal %q does not carry the retry interval", got)
	}
	if !strings.Contains(got, "5m0s") {
		t.Errorf("refusal %q does not carry the enforced interval", got)
	}
}

// Nothing reached the air, so nothing is owed an answer and the interval was
// not charged. Reporting a wait here would send a caller off to sleep through
// an interval it does not owe.
func TestDescribeRollCallRefusal_NotTransmittedDoesNotAskForAWait(t *testing.T) {
	t.Parallel()

	got := describeRollCallRefusal(&bramble.StartRollCallResponse{
		Reason: bramble.RollCallRefusalNotTransmitted,
	})
	if !strings.Contains(got, "never reached the air") {
		t.Errorf("refusal %q does not say the announce was not transmitted", got)
	}
	if strings.Contains(got, "retry in") {
		t.Errorf("refusal %q asks for a wait that is not owed", got)
	}
}

func TestDescribeRollCallRefusal_UnknownReasonIsReportedVerbatim(t *testing.T) {
	t.Parallel()

	// A reason this build does not know is still the node's answer; swallowing
	// it would leave the operator with a refusal and no cause at all.
	got := describeRollCallRefusal(&bramble.StartRollCallResponse{Reason: "quiet_hours"})
	if !strings.Contains(got, "quiet_hours") {
		t.Errorf("refusal %q dropped the reason the node gave", got)
	}

	got = describeRollCallRefusal(&bramble.StartRollCallResponse{})
	if !strings.Contains(got, "no reason") {
		t.Errorf("reasonless refusal rendered as %q", got)
	}
}

func TestPrintRollCallStarted_AnchoredReportsTheExpectedSet(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printRollCallStarted(&buf, &bramble.StartRollCallResponse{
		OK:          true,
		RollCallID:  "3F2A1B0C",
		WindowMs:    135000,
		RoundsTotal: 3,
		Expected:    5,
		Anchored:    true,
	})
	out := buf.String()
	for _, want := range []string{"3F2A1B0C", "2m15s", "Anchored fleet", "5 member(s) expected", "roll-call status"} {
		if !strings.Contains(out, want) {
			t.Errorf("start output is missing %q:\n%s", want, out)
		}
	}
}

func TestPrintRollCallStarted_UnanchoredSaysNobodyCanBeCalledMissing(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printRollCallStarted(&buf, &bramble.StartRollCallResponse{
		OK:          true,
		RollCallID:  "3F2A1B0C",
		WindowMs:    135000,
		RoundsTotal: 3,
	})
	out := buf.String()
	if !strings.Contains(out, "Observed only") {
		t.Errorf("un-anchored start is not labeled observed-only:\n%s", out)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("un-anchored start does not say nobody can be called missing:\n%s", out)
	}
}

func TestRenderRollCallLedger_NeverStarted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, &bramble.RollCallLedger{Active: false})
	out := buf.String()
	if !strings.Contains(out, "never started a roll-call") {
		t.Errorf("inactive ledger rendered as:\n%s", out)
	}
	if strings.Contains(out, "ADDRESS") {
		t.Errorf("inactive ledger printed an empty table:\n%s", out)
	}
}

func anchoredLedger() *bramble.RollCallLedger {
	return &bramble.RollCallLedger{
		Active:           true,
		RollCallID:       "3F2A1B0C",
		Open:             true,
		Text:             "sound off",
		RoundsSent:       1,
		RoundsTotal:      3,
		WindowMs:         135000,
		ElapsedMs:        70000,
		MinIntervalMs:    300000,
		MaxTextBytes:     48,
		Anchored:         true,
		Expected:         5,
		Responded:        2,
		MissingCount:     1,
		Missing:          []string{"4E5F6071"},
		AnswerMaxPerHour: 12,
		Responders: []bramble.RollCallResponder{
			{Address: "0A1B2C3D", Responded: true, AtMs: 4200, Round: 1, Hops: 2,
				Path: []string{"3D4E5F60", "0A1B2C3D"}},
			{Address: "1B2C3D4E", Responded: true, AtMs: 9100, Round: 1},
			{Address: "2C3D4E5F", Responded: false, Hops: 1, Path: []string{"2C3D4E5F"}},
		},
	}
}

func TestRenderRollCallLedger_AnchoredNamesTheSilentMember(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, anchoredLedger())
	out := buf.String()

	for _, want := range []string{
		"3F2A1B0C",
		"Anchored fleet",
		"2 of 5 expected member(s)",
		"collecting, 1m10s of 2m15s elapsed, round 1 of 3 sent",
		`"sound off" (this node accepts up to 48 bytes)`,
		"once every 5m0s",
		"No answer (1): 4E5F6071",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ledger is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRollCallLedger_RowsCarryTimeRoundAndPath(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, anchoredLedger())
	out := buf.String()

	answered := ledgerRow(t, out, "0A1B2C3D")
	for _, want := range []string{"verified", "4s", "1", "2", "3D4E5F60 > 0A1B2C3D"} {
		if !strings.Contains(answered, want) {
			t.Errorf("the row for 0A1B2C3D is missing %q: %q", want, answered)
		}
	}

	// A receipt named this member; nothing was signed. The row exists because
	// the announce was seen to travel there, and it must not read as an answer.
	unanswered := ledgerRow(t, out, "2C3D4E5F")
	if !strings.Contains(unanswered, "no answer") {
		t.Errorf("a receipt-only row does not say it carries no answer: %q", unanswered)
	}
}

// ledgerRow returns the rendered table row for one address, so an assertion
// reads the row it means rather than the column padding around it.
func ledgerRow(t *testing.T, rendered, address string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, address) {
			return line
		}
	}
	t.Fatalf("no row for %s in:\n%s", address, rendered)
	return ""
}

// An un-anchored node pins trust-on-first-use identities, which are free to
// mint, so it has no roster to count against and names nobody missing. A ledger
// rendered like the anchored one would read as a complete fleet answer.
func TestRenderRollCallLedger_UnanchoredIsLabeledObservedOnly(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, &bramble.RollCallLedger{
		Active:      true,
		RollCallID:  "3F2A1B0C",
		RoundsSent:  3,
		RoundsTotal: 3,
		WindowMs:    135000,
		Responded:   2,
		Responders: []bramble.RollCallResponder{
			{Address: "0A1B2C3D", Responded: true, AtMs: 4200, Round: 1},
			{Address: "1B2C3D4E", Responded: true, AtMs: 9100, Round: 2},
		},
	})
	out := buf.String()

	if !strings.Contains(out, "Observed only") {
		t.Errorf("un-anchored ledger is not labeled observed-only:\n%s", out)
	}
	if !strings.Contains(out, "2 member(s), out of an unknown total") {
		t.Errorf("un-anchored ledger claims a denominator it does not have:\n%s", out)
	}
	if strings.Contains(out, "No answer") {
		t.Errorf("un-anchored ledger named somebody missing:\n%s", out)
	}
	if !strings.Contains(out, "closed after 2m15s and 3 of 3 round(s)") {
		t.Errorf("a closed ledger does not report itself closed:\n%s", out)
	}
}

func TestRenderRollCallLedger_ReportsWhatNeverBecameARow(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, &bramble.RollCallLedger{
		Active:           true,
		RollCallID:       "3F2A1B0C",
		RoundsTotal:      3,
		WindowMs:         135000,
		Anchored:         true,
		Expected:         30,
		Responded:        24,
		Unattested:       2,
		Overflow:         3,
		Late:             1,
		PendingDropped:   1,
		AnswerLimited:    4,
		AnswerMaxPerHour: 12,
	})
	out := buf.String()

	for _, want := range []string{
		"2 answer(s) could not be attested",
		"3 answer(s) did not fit",
		"1 answer(s) arrived after the ledger closed",
		"1 answer(s) THIS node could not queue",
		"4 answer(s) THIS node refused",
		"budget of 12 per rolling hour",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ledger is missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "No member has answered yet.") {
		t.Errorf("an empty responder table is not explained:\n%s", out)
	}
}

func TestRenderRollCallLedger_QuietWhenNothingWasDropped(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, anchoredLedger())
	if strings.Contains(buf.String(), "Note:") {
		t.Errorf("a clean ledger printed a counter note:\n%s", buf.String())
	}
}

// Silence is not proof, and a relay path is not part of the attestation. Both
// are printed with the numbers so a copied ledger cannot lose them.
func TestRenderRollCallLedger_CarriesWhatItDoesNotProve(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, anchoredLedger())
	out := buf.String()
	if !strings.Contains(out, "Silence proves nothing on its own") {
		t.Errorf("ledger does not bound what a missing answer means:\n%s", out)
	}
	if !strings.Contains(out, "not part of the attestation") {
		t.Errorf("ledger does not bound what a relay path means:\n%s", out)
	}
}

// The count and the names are separate fields on the wire. A member named as
// silent must appear in the header count even if the count field disagrees,
// since the names are the part an operator acts on.
func TestRenderRollCallLedger_MissingHeaderCountsEveryNamedMember(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, &bramble.RollCallLedger{
		Active:      true,
		RollCallID:  "3F2A1B0C",
		RoundsTotal: 3,
		WindowMs:    135000,
		Anchored:    true,
		Expected:    3,
		Responded:   1,
		Missing:     []string{"4E5F6071", "2C3D4E5F"},
	})
	if !strings.Contains(buf.String(), "No answer (2): 4E5F6071, 2C3D4E5F") {
		t.Errorf("named silent members were undercounted:\n%s", buf.String())
	}
}

// A count with no names is what an overflowing table reports, and it is still
// the honest answer: say how many, and do not invent addresses for them.
func TestRenderRollCallLedger_MissingCountWithoutNames(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderRollCallLedger(&buf, &bramble.RollCallLedger{
		Active:       true,
		RollCallID:   "3F2A1B0C",
		RoundsTotal:  3,
		WindowMs:     135000,
		Anchored:     true,
		Expected:     9,
		Responded:    4,
		MissingCount: 5,
	})
	if !strings.Contains(buf.String(), "No answer (5): the node reported the count without naming them") {
		t.Errorf("an unnamed missing set rendered as:\n%s", buf.String())
	}
}

func TestFormatRelayPath(t *testing.T) {
	t.Parallel()

	if got := formatRelayPath([]string{"3D4E5F60", "0A1B2C3D"}); got != "3D4E5F60 > 0A1B2C3D" {
		t.Errorf("path rendered as %q", got)
	}
	// No receipt supplied a path, which is not the same as a direct link.
	if got := formatRelayPath(nil); got != "-" {
		t.Errorf("absent path rendered as %q, want a dash", got)
	}
}

func TestRollCallResponderRow_UnansweredRowHasNoTimeOrRound(t *testing.T) {
	t.Parallel()

	row := rollCallResponderRow(bramble.RollCallResponder{Address: "2C3D4E5F"})
	want := []string{"2C3D4E5F", "no answer", "-", "-", "-", "-"}
	if len(row) != len(want) {
		t.Fatalf("row has %d columns, want %d", len(row), len(want))
	}
	for i := range want {
		if row[i] != want[i] {
			t.Errorf("column %d is %q, want %q", i, row[i], want[i])
		}
	}
}
