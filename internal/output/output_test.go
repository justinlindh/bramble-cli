package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestPrintJSON_Success(t *testing.T) {
	var buf bytes.Buffer
	in := map[string]any{"name": "bramble", "n": 1}

	if err := PrintJSON(&buf, in); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "\"name\": \"bramble\"") {
		t.Fatalf("missing JSON field in output: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected newline suffix, got: %q", got)
	}
}

func TestPrintJSON_MarshalError(t *testing.T) {
	bad := map[string]any{"ch": make(chan int)}
	err := PrintJSON(&bytes.Buffer{}, bad)
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "json marshal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintJSON_WriteError(t *testing.T) {
	err := PrintJSON(errWriter{}, map[string]string{"ok": "y"})
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTable_BasicAlignment(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"NAME", "VALUE"}
	rows := [][]string{{"a", "1"}, {"longer", "22"}}

	Table(&buf, headers, rows)
	got := buf.String()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[1], "------") {
		t.Fatalf("expected separator line, got: %q", lines[1])
	}
}

func TestTable_ZeroColumnsAndExtraCells(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, nil, [][]string{{"x", "y"}})
	got := buf.String()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines for empty headers + row, got %d (%q)", len(lines), got)
	}
	if lines[0] != "" || lines[1] != "" {
		t.Fatalf("expected empty header and separator lines, got %q / %q", lines[0], lines[1])
	}
	if lines[2] != "x  y" {
		t.Fatalf("expected row with raw cells, got %q", lines[2])
	}
}

func TestAddrAndDurations(t *testing.T) {
	if got := Addr(0x1EE6); got != "00001EE6" {
		t.Fatalf("Addr mismatch: %q", got)
	}

	casesMs := map[int64]string{
		-1:      "unknown",
		0:       "0ms",
		999:     "999ms",
		1000:    "1s",
		61000:   "1m1s",
		3661000: "1h1m",
	}
	for in, want := range casesMs {
		if got := FormatMs(in); got != want {
			t.Fatalf("FormatMs(%d)=%q want %q", in, got, want)
		}
	}

	casesUp := map[int]string{
		0:     "0s",
		59:    "59s",
		60:    "1m0s",
		3661:  "1h1m",
		90061: "1d1h1m",
		-10:   "-10s",
	}
	for in, want := range casesUp {
		if got := FormatUptime(in); got != want {
			t.Fatalf("FormatUptime(%d)=%q want %q", in, got, want)
		}
	}
}
