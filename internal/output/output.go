// Package output provides table and JSON formatters for CLI output.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// PrintJSON marshals v to indented JSON and writes it to w.
func PrintJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// Table renders a simple fixed-width table to w.
// headers and rows must have the same number of columns.
func Table(w io.Writer, headers []string, rows [][]string) {
	// Compute column widths (at least as wide as the header).
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow := func(cells []string) {
		for i, cell := range cells {
			if i < len(widths) {
				fmt.Fprintf(w, "%-*s", widths[i], cell)
			} else {
				fmt.Fprint(w, cell)
			}
			if i < len(cells)-1 {
				fmt.Fprint(w, "  ")
			}
		}
		fmt.Fprintln(w)
	}

	printRow(headers)

	// Separator line.
	parts := make([]string, len(headers))
	for i, w := range widths {
		parts[i] = strings.Repeat("-", w)
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))

	for _, row := range rows {
		printRow(row)
	}
}

// Addr formats a uint32 mesh address as an 8-char uppercase hex string.
func Addr(addr uint32) string {
	return fmt.Sprintf("%08X", addr)
}

// FormatMs formats milliseconds as a human-readable duration string.
func FormatMs(ms int64) string {
	if ms < 0 {
		return "unknown"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	secs := ms / 1000
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	secs = secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := mins / 60
	mins = mins % 60
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// FormatUptime formats an uptime in seconds as a human-readable string.
func FormatUptime(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	secs = secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := mins / 60
	mins = mins % 60
	if hours < 24 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd%dh%dm", days, hours, mins)
}
