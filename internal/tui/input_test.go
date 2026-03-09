package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	bramble "github.com/justinlindh/bramble-go"
)

func TestCommandSuggestionSuffix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "help prefix", input: "/he", want: "lp"},
		{name: "exact command has no suffix", input: "/help", want: ""},
		{name: "non-command text", input: "hello", want: ""},
		{name: "command with args no suffix", input: "/msg DEADBEEF hi", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandSuggestionSuffix(tt.input); got != tt.want {
				t.Fatalf("commandSuggestionSuffix(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInputLineTabAutocompletesCommand(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("/he")

	updated, _ := il.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := updated.textarea.Value(); got != "/help" {
		t.Fatalf("tab autocomplete value=%q want %q", got, "/help")
	}
}

func TestInputLineEnterBlockedWhenLockoutActive(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("hello")
	il.SetLockout(&InputLockout{Tier: "broadcast", RefillInSecs: 9})

	updated, cmd := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected blocked-send cmd")
	}
	msg := cmd()
	blocked, ok := msg.(InputBlockedMsg)
	if !ok {
		t.Fatalf("expected InputBlockedMsg, got %T", msg)
	}
	if blocked.Tier != "broadcast" || blocked.RefillInSecs != 9 {
		t.Fatalf("unexpected block details: %+v", blocked)
	}
	if got := updated.textarea.Value(); got != "hello" {
		t.Fatalf("message should remain in input; got %q", got)
	}
}

func TestInputLineRefillReenablesEnterSend(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("hello")
	il.SetLockout(&InputLockout{Tier: "broadcast", RefillInSecs: 2})
	il.SetLockout(nil)

	updated, cmd := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected send cmd")
	}
	msg := cmd()
	inputMsg, ok := msg.(InputMsg)
	if !ok {
		t.Fatalf("expected InputMsg, got %T", msg)
	}
	if inputMsg.Text != "hello" {
		t.Fatalf("unexpected text: %q", inputMsg.Text)
	}
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("input should clear after send, got %q", got)
	}
}

func TestAirtimeTierMatchingByConversationAndCriticalPrefix(t *testing.T) {
	m := Model{activeConv: "broadcast"}
	if got := m.airtimeTierForText("hello", false); got != "broadcast" {
		t.Fatalf("broadcast tier=%q want broadcast", got)
	}

	m.activeConv = "ch:2"
	if got := m.airtimeTierForText("hello", false); got != "broadcast" {
		t.Fatalf("channel tier=%q want broadcast", got)
	}

	m.activeConv = "dm:ABCD"
	if got := m.airtimeTierForText("hello", false); got != "normal" {
		t.Fatalf("dm tier=%q want normal", got)
	}
	if got := m.airtimeTierForText("/critical urgent", false); got != "critical" {
		t.Fatalf("critical prefix tier=%q want critical", got)
	}
	if got := m.airtimeTierForText("hello", true); got != "critical" {
		t.Fatalf("explicit critical tier=%q want critical", got)
	}
}

func TestCurrentAirtimeLockoutUsesRelevantTier(t *testing.T) {
	now := time.Unix(100, 0)
	refill := now.Add(8 * time.Second).UnixMilli()
	m := Model{activeConv: "dm:ABCD", store: NewStore()}
	m.store.UpdateAirtime(&bramble.AirtimeStats{Tiers: []bramble.AirtimeTier{
		{Name: "broadcast", RemainingMs: 0, RefillAtMs: refill},
		{Name: "normal", RemainingMs: 1000, RefillAtMs: refill},
	}})

	if lock := m.currentAirtimeLockout("hello", false, now); lock != nil {
		t.Fatalf("expected dm send unlocked when normal tier has budget, got %+v", lock)
	}

	m.store.UpdateAirtime(&bramble.AirtimeStats{Tiers: []bramble.AirtimeTier{
		{Name: "broadcast", RemainingMs: 0, RefillAtMs: refill},
		{Name: "normal", RemainingMs: 0, RefillAtMs: refill},
	}})
	lock := m.currentAirtimeLockout("hello", false, now)
	if lock == nil {
		t.Fatal("expected lockout when normal tier is depleted")
	}
	if lock.Tier != "normal" || lock.RefillInSecs <= 0 {
		t.Fatalf("unexpected lockout: %+v", lock)
	}
}

func TestMessageByteMetaMatchesWebLimit(t *testing.T) {
	meta := messageByteMeta(strings.Repeat("a", 617), false)
	if !meta.OverLimit {
		t.Fatalf("expected over-limit for 617 bytes")
	}
	if meta.MaxBytes != fragmentedMaxBytes {
		t.Fatalf("max bytes=%d want %d", meta.MaxBytes, fragmentedMaxBytes)
	}
	if meta.Label != "too long" {
		t.Fatalf("label=%q want too long", meta.Label)
	}
}

func TestMessageByteMetaShowsFragmentCount(t *testing.T) {
	meta := messageByteMeta(strings.Repeat("a", 308), false)
	if meta.FragmentCount != 2 {
		t.Fatalf("fragment count=%d want 2", meta.FragmentCount)
	}
	if meta.Label != "2 fragments" {
		t.Fatalf("label=%q want %q", meta.Label, "2 fragments")
	}
}

func TestInputLineTypeaheadRendersInline(t *testing.T) {
	il := NewInputLine()
	il.SetWidth(80)
	il.textarea.SetValue("/no")

	view := il.View()
	if strings.Contains(view, "\ndes") {
		t.Fatalf("expected typeahead suffix to render inline, got view:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("unexpected view shape:\n%s", view)
	}
	row := stripANSI(lines[1])
	if !strings.Contains(row, "/nodes") {
		t.Fatalf("expected inline completion '/nodes' on input row, got row:\n%s", row)
	}
}

func TestInputLineTypeaheadUsesDimStyle(t *testing.T) {
	il := NewInputLine()
	il.SetWidth(80)
	il.textarea.SetValue("/he")

	view := il.View()
	if !strings.Contains(view, "38;2;102;102;136m") {
		t.Fatalf("expected dim typeahead color escape in view, got:\n%s", view)
	}
	if !strings.Contains(stripANSI(view), "/help") {
		t.Fatalf("expected rendered completion '/help', got:\n%s", stripANSI(view))
	}
}

func TestInputHistoryRecallBasic(t *testing.T) {
	il := NewInputLine()

	il.textarea.SetValue("first")
	updated, cmd := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	il = updated
	if cmd == nil {
		t.Fatal("expected send cmd for first message")
	}

	il.textarea.SetValue("second")
	updated, cmd = il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	il = updated
	if cmd == nil {
		t.Fatal("expected send cmd for second message")
	}

	updated, _ = il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "second" {
		t.Fatalf("up recall=%q want %q", got, "second")
	}
}

func TestInputHistoryCycleAndDraftRestore(t *testing.T) {
	il := NewInputLine()
	for _, msg := range []string{"one", "two", "three"} {
		il.textarea.SetValue(msg)
		updated, cmd := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		il = updated
		if cmd == nil {
			t.Fatalf("expected send cmd for %q", msg)
		}
	}

	il.textarea.SetValue("draft")
	updated, _ := il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "three" {
		t.Fatalf("first up=%q want %q", got, "three")
	}

	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "two" {
		t.Fatalf("second up=%q want %q", got, "two")
	}

	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := updated.textarea.Value(); got != "three" {
		t.Fatalf("down=%q want %q", got, "three")
	}

	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := updated.textarea.Value(); got != "draft" {
		t.Fatalf("down past newest should restore draft=%q want %q", got, "draft")
	}
}

func TestInputHistoryDedupsOnSend(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("hello")
	updated, cmd := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	il = updated
	if cmd == nil {
		t.Fatal("expected send cmd")
	}

	updated, _ = il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "hello" {
		t.Fatalf("up recall=%q want hello", got)
	}
	updated, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected send cmd from recalled message")
	}

	if got := len(updated.history); got != 1 {
		t.Fatalf("history length=%d want 1 (dedup)", got)
	}
}

func TestInputHistoryBoundaryCases(t *testing.T) {
	il := NewInputLine()

	updated, _ := il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("up with empty history changed input=%q", got)
	}

	il.textarea.SetValue("typed")
	updated, _ = il.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := updated.textarea.Value(); got != "typed" {
		t.Fatalf("down while not browsing changed input=%q", got)
	}
}

func TestInputHistoryUpRequiresTopLeftForMultiline(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("prev")
	updated, _ := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	il = updated

	il.textarea.SetValue("a\nb")
	il.textarea.CursorDown()
	updated, _ = il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "a\nb" {
		t.Fatalf("up should not trigger history off first line, got=%q", got)
	}

	updated.textarea.CursorStart()
	updated.textarea.CursorUp()
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "prev" {
		t.Fatalf("up should trigger history at top-left on multiline, got=%q", got)
	}
}

func TestInputHistoryEscClearsAndExitsBrowse(t *testing.T) {
	il := NewInputLine()
	il.textarea.SetValue("one")
	updated, _ := il.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	il = updated

	updated, _ = il.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := updated.textarea.Value(); got != "one" {
		t.Fatalf("up recall=%q want one", got)
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("esc should clear input, got=%q", got)
	}
	if updated.historyIdx != -1 {
		t.Fatalf("esc should exit history browse, historyIdx=%d", updated.historyIdx)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
