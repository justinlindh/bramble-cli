package commands

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"go.bug.st/serial"
)

func TestConsoleSerialModeLeavesDTRAndRTSDeasserted(t *testing.T) {
	// The whole point of this command is attaching to a running node without
	// disturbing it. go.bug.st/serial documents a nil InitialStatusBits as
	// DTR=true and RTS=true, and on a CP2102 those lines drive EN and BOOT, so
	// a nil here reboots the device and destroys the log the user came to read.
	mode := consoleSerialMode(115200)
	if mode.InitialStatusBits == nil {
		t.Fatal("InitialStatusBits is nil, which the serial library reads as DTR=true and RTS=true; opening would reset the node")
	}
	if mode.InitialStatusBits.DTR {
		t.Error("DTR asserted at open: this reboots a CP2102 board")
	}
	if mode.InitialStatusBits.RTS {
		t.Error("RTS asserted at open: this reboots a CP2102 board")
	}
	if mode.BaudRate != 115200 || mode.DataBits != 8 || mode.Parity != serial.NoParity || mode.StopBits != serial.OneStopBit {
		t.Errorf("port settings were disturbed: %+v", mode)
	}
}

func TestStreamConsoleHidesRPCFramesByDefault(t *testing.T) {
	// The device multiplexes log output and JSON-RPC onto one port. When you
	// are reading logs the frames are noise, and a wall of them buries the one
	// line that explains the failure.
	in := strings.Join([]string{
		`I (1234) mesh: TX location (channel) to FFFFFFFF`,
		`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
		`W (1240) mesh: no session for DEADBEEF, dropping`,
		`  {"jsonrpc":"2.0","method":"bramble.onMessage"}`,
	}, "\n")

	var out bytes.Buffer
	if err := streamConsole(strings.NewReader(in), &out, consoleFilter{}); err != nil {
		t.Fatalf("streamConsole: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "jsonrpc") {
		t.Errorf("RPC frames leaked into the output:\n%s", got)
	}
	if !strings.Contains(got, "TX location") || !strings.Contains(got, "no session for DEADBEEF") {
		t.Errorf("log lines were dropped:\n%s", got)
	}
}

func TestStreamConsoleRawKeepsEverything(t *testing.T) {
	in := "I (1) mesh: hello\n{\"jsonrpc\":\"2.0\"}\n"

	var out bytes.Buffer
	if err := streamConsole(strings.NewReader(in), &out, consoleFilter{raw: true}); err != nil {
		t.Fatalf("streamConsole: %v", err)
	}
	if !strings.Contains(out.String(), "jsonrpc") {
		t.Errorf("--raw dropped the RPC frame:\n%s", out.String())
	}
}

func TestStreamConsoleAppliesGrep(t *testing.T) {
	in := "I (1) mesh: beacon sent\nI (2) location: share skipped\nI (3) mesh: beacon sent\n"
	grep := regexp.MustCompile("location")

	var out bytes.Buffer
	if err := streamConsole(strings.NewReader(in), &out, consoleFilter{grep: grep}); err != nil {
		t.Fatalf("streamConsole: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != "I (2) location: share skipped" {
		t.Errorf("grep output was %q", got)
	}
}

func TestStreamConsoleStripsCarriageReturnsAndBlankLines(t *testing.T) {
	// The firmware terminates lines with CRLF. Leaving the CR on makes every
	// line end in a stray character that breaks downstream grep and diff.
	in := "I (1) mesh: one\r\n\r\nI (2) mesh: two\r\n"

	var out bytes.Buffer
	if err := streamConsole(strings.NewReader(in), &out, consoleFilter{}); err != nil {
		t.Fatalf("streamConsole: %v", err)
	}
	if got := out.String(); got != "I (1) mesh: one\nI (2) mesh: two\n" {
		t.Errorf("got %q", got)
	}
}

func TestStreamConsoleHandlesLongBacktraceLines(t *testing.T) {
	// ESP-IDF backtraces run past bufio.Scanner's 64 KiB default, and hitting
	// that limit aborts the tail at exactly the crash you were trying to read.
	long := "E (1) panic: " + strings.Repeat("0x40081234:0x3ffb1234 ", 8000)

	var out bytes.Buffer
	if err := streamConsole(strings.NewReader(long+"\n"), &out, consoleFilter{}); err != nil {
		t.Fatalf("streamConsole: %v", err)
	}
	if !strings.Contains(out.String(), "panic") {
		t.Error("the long line was dropped")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("port went away") }

func TestStreamConsoleReportsReadFailure(t *testing.T) {
	var out bytes.Buffer
	err := streamConsole(failingReader{}, &out, consoleFilter{})
	if err == nil || !strings.Contains(err.Error(), "read console") {
		t.Fatalf("error was %v, want a read failure", err)
	}
}

func TestIsRPCFrame(t *testing.T) {
	cases := map[string]bool{
		`{"jsonrpc":"2.0"}`:      true,
		`   {"jsonrpc":"2.0"}`:   true,
		`I (123) mesh: hello`:    false,
		`not json {but has one}`: false,
		``:                       false,
	}
	for line, want := range cases {
		if got := isRPCFrame(line); got != want {
			t.Errorf("isRPCFrame(%q) = %v, want %v", line, got, want)
		}
	}
}
