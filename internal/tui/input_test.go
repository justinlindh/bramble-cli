package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
