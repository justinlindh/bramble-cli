package tui

import "strings"

// termLink wraps text in an OSC 8 terminal hyperlink sequence.
// If url is empty, text is returned unchanged.
func termLink(url, text string) string {
	if strings.TrimSpace(url) == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
