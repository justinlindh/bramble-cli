package commands

import (
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    uint32
		wantErr bool
	}{
		// Valid hex
		{"hex upper", "DEADBEEF", 0xDEADBEEF, false},
		{"hex lower", "deadbeef", 0xDEADBEEF, false},
		{"hex mixed", "DeAdBeEf", 0xDEADBEEF, false},
		{"hex short", "FF", 0xFF, false},
		{"hex zero", "0", 0, false},
		{"hex with 0x prefix", "0xDEADBEEF", 0xDEADBEEF, false},
		{"hex with 0X prefix", "0XDEADBEEF", 0xDEADBEEF, false},
		{"hex with 0x short", "0xFF", 0xFF, false},
		{"hex max uint32", "FFFFFFFF", 0xFFFFFFFF, false},
		{"hex CAFEBABE", "CAFEBABE", 0xCAFEBABE, false},

		// Invalid
		{"empty string", "", 0, true},
		{"not hex or decimal", "ZZZZ", 0, true},
		{"negative", "-1", 0, true},
		{"overflow hex", "1FFFFFFFF", 0, true},
		{"spaces", " DEAD BEEF ", 0, true},
		{"special chars", "DEAD!BEEF", 0, true},
		{"0x alone", "0x", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAddress(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAddress(%q) = %d, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseAddress(%q) = 0x%X, want 0x%X", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewSendCmd_NoArgs(t *testing.T) {
	t.Parallel()

	cmd := newSendCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestNewSendCmd_ThreeArgs(t *testing.T) {
	t.Parallel()

	cmd := newSendCmd()
	cmd.SetArgs([]string{"DEAD", "hello", "extra"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestChannelsAddCmd_ThreeArgs(t *testing.T) {
	t.Parallel()

	cmd := newChannelsAddCmd()
	cmd.SetArgs([]string{"name", "psk", "extra"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many args to channels add")
	}
}

func TestConfigSetName_ExactlyThirtyTwoChars(t *testing.T) {
	t.Parallel()

	// 32 chars should be fine (no error from validation, will fail on client connect)
	err := runConfigSetName(newConfigSetNameCmd(), []string{"12345678901234567890123456789012"})
	// Should NOT get "too long" error — it will fail on getClient instead
	if err != nil && err.Error() != "" {
		// Acceptable: either no error or a client connection error
		if strings.Contains(err.Error(), "too long") {
			t.Fatalf("32-char name should be allowed, got: %v", err)
		}
	}
}

func TestConfigSetRadio_SingleFlag(t *testing.T) {
	t.Parallel()

	cmd := newConfigSetRadioCmd()
	cmd.SetArgs([]string{"--sf", "10"})
	// Parse flags manually
	if err := cmd.ParseFlags([]string{"--sf", "10"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	err := runConfigSetRadio(cmd, nil)
	// Should pass the "no flags" check but fail on getClient
	if err != nil && strings.Contains(err.Error(), "specify at least one") {
		t.Fatalf("single flag should be accepted, got: %v", err)
	}
}


