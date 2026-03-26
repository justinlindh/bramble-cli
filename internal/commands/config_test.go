package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewConfigCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newConfigCmd()
	want := []string{"get", "set-name", "set-radio"}

	for _, name := range want {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

func TestRunConfigSetName_TooLong(t *testing.T) {
	t.Parallel()

	err := runConfigSetName(newConfigSetNameCmd(), []string{"123456789012345678901234567890123"})
	if err == nil {
		t.Fatal("expected too-long name error")
	}
	if !strings.Contains(err.Error(), `is too long (max 32 characters)`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfigSetRadio_NoFlagsChanged(t *testing.T) {
	t.Parallel()

	cmd := newConfigSetRadioCmd()
	err := runConfigSetRadio(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no radio flags are provided")
	}
	if !strings.Contains(err.Error(), "specify at least one radio parameter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func setIntFlag(cmd *cobra.Command, name string, value int) {
	_ = cmd.Flags().Set(name, fmt.Sprintf("%d", value))
}

func TestRunConfigSetRadio_SFValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sf      int
		wantErr bool
		errMsg  string
	}{
		{sf: 6, wantErr: true, errMsg: "spreading factor must be 7–12"},
		{sf: 7, wantErr: false},
		{sf: 10, wantErr: false},
		{sf: 12, wantErr: false},
		{sf: 13, wantErr: true, errMsg: "spreading factor must be 7–12"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("sf=%d", tc.sf), func(t *testing.T) {
			t.Parallel()
			cmd := newConfigSetRadioCmd()
			setIntFlag(cmd, "sf", tc.sf)
			err := runConfigSetRadio(cmd, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("sf=%d: expected error but got nil", tc.sf)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("sf=%d: expected error containing %q, got: %v", tc.sf, tc.errMsg, err)
				}
			} else {
				// Valid SF should not fail on range validation (may fail on RPC — that's OK)
				if err != nil && strings.Contains(err.Error(), "spreading factor") {
					t.Fatalf("sf=%d: unexpected range error: %v", tc.sf, err)
				}
			}
		})
	}
}

func TestRunConfigSetRadio_CRValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cr      int
		wantErr bool
		errMsg  string
	}{
		{cr: 4, wantErr: true, errMsg: "coding rate must be 5–8"},
		{cr: 5, wantErr: false},
		{cr: 6, wantErr: false},
		{cr: 8, wantErr: false},
		{cr: 9, wantErr: true, errMsg: "coding rate must be 5–8"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("cr=%d", tc.cr), func(t *testing.T) {
			t.Parallel()
			cmd := newConfigSetRadioCmd()
			setIntFlag(cmd, "cr", tc.cr)
			err := runConfigSetRadio(cmd, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("cr=%d: expected error but got nil", tc.cr)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("cr=%d: expected error containing %q, got: %v", tc.cr, tc.errMsg, err)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "coding rate") {
					t.Fatalf("cr=%d: unexpected range error: %v", tc.cr, err)
				}
			}
		})
	}
}

func TestRunConfigSetRadio_TXPowerValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		txpower int
		wantErr bool
		errMsg  string
	}{
		{txpower: -21, wantErr: true, errMsg: "TX power must be"},
		{txpower: -20, wantErr: false},
		{txpower: 0, wantErr: false},
		{txpower: 22, wantErr: false},
		{txpower: 23, wantErr: true, errMsg: "TX power must be"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("txpower=%d", tc.txpower), func(t *testing.T) {
			t.Parallel()
			cmd := newConfigSetRadioCmd()
			setIntFlag(cmd, "txpower", tc.txpower)
			err := runConfigSetRadio(cmd, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("txpower=%d: expected error but got nil", tc.txpower)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("txpower=%d: expected error containing %q, got: %v", tc.txpower, tc.errMsg, err)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "TX power") {
					t.Fatalf("txpower=%d: unexpected range error: %v", tc.txpower, err)
				}
			}
		})
	}
}

func TestNewConfigSetNameCmd_ArgValidation(t *testing.T) {
	t.Parallel()

	cmd := newConfigSetNameCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected argument validation error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
