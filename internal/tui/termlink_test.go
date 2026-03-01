package tui

import "testing"

func TestTermLinkWrapsTextWithOSC8EscapeSequence(t *testing.T) {
	url := "https://example.com/path"
	text := "Click me"
	want := "\x1b]8;;https://example.com/path\x1b\\Click me\x1b]8;;\x1b\\"

	if got := termLink(url, text); got != want {
		t.Fatalf("termLink() mismatch\nwant: %q\n got: %q", want, got)
	}
}
