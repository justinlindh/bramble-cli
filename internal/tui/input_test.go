package tui

import (
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
