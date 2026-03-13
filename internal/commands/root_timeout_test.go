package commands

import (
	"testing"
	"time"
)

// reset restores shared flag state after each test.
func resetTimeoutFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		flagBLE = ""
		flagTimeout = 0
	})
}

func TestEffectiveTimeout_DefaultIsRequestTimeout(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	flagBLE = ""
	flagTimeout = 0

	if got := effectiveTimeout(); got != requestTimeout {
		t.Fatalf("expected requestTimeout (%v), got %v", requestTimeout, got)
	}
}

func TestEffectiveTimeout_BLEUsesBLETimeout(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	flagBLE = "Bramble"
	flagTimeout = 0

	if got := effectiveTimeout(); got != bleTimeout {
		t.Fatalf("expected bleTimeout (%v) for BLE transport, got %v", bleTimeout, got)
	}
}

func TestEffectiveTimeout_FlagOverridesTakesPrecedence(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	// Even with BLE set, an explicit --timeout flag must win.
	flagBLE = "Bramble"
	flagTimeout = 60 * time.Second

	if got := effectiveTimeout(); got != 60*time.Second {
		t.Fatalf("expected 60s from flag override, got %v", got)
	}
}

func TestEffectiveTimeout_FlagOverridesSerial(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	flagBLE = ""
	flagTimeout = 5 * time.Second

	if got := effectiveTimeout(); got != 5*time.Second {
		t.Fatalf("expected 5s from flag override, got %v", got)
	}
}

func TestCommandContext_HasDeadline(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	flagBLE = ""
	flagTimeout = 0

	ctx, cancel := commandContext()
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("commandContext() returned a context with no deadline")
	}
}

func TestCommandContext_BLEDeadlineIsLonger(t *testing.T) {
	t.Parallel()
	resetTimeoutFlags(t)

	// Serial/default
	flagBLE = ""
	flagTimeout = 0
	serialCtx, cancelSerial := commandContext()
	defer cancelSerial()

	serialDeadline, _ := serialCtx.Deadline()

	// BLE
	flagBLE = "Bramble"
	bleCtx, cancelBLE := commandContext()
	defer cancelBLE()

	bleDeadline, _ := bleCtx.Deadline()

	if !bleDeadline.After(serialDeadline) {
		t.Fatalf("BLE deadline (%v) should be after serial deadline (%v)", bleDeadline, serialDeadline)
	}
}

func TestBLETimeoutIsAtLeast30Seconds(t *testing.T) {
	t.Parallel()

	if bleTimeout < 30*time.Second {
		t.Fatalf("bleTimeout must be at least 30s (scan+connect+RPC); got %v", bleTimeout)
	}
}
