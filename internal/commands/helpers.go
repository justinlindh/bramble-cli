package commands

import (
	"fmt"
	"strconv"
	"strings"
)

// parseIntArg parses a string argument as an integer, returning a user-friendly
// error message that includes the parameter name.
func parseIntArg(s, name string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be an integer", name, s)
	}
	return v, nil
}

// isAuthError reports whether err is the node's "Unauthorized" RPC rejection
// (RPC_ERR_UNAUTHORIZED, -1005). The node accepts the WebSocket/BLE handshake
// even without a valid token (unauthenticated clients reach a tiny pairing
// allowlist), then rejects every real RPC with this error. The bramble-go SDK
// surfaces it as a wrapped error whose message carries the -1005 code and the
// "Unauthorized" text; the concrete error type is unexported, so we match on
// the message.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "-1005")
}

// parseBoolArg parses a string argument as a boolean (true/false), returning a
// user-friendly error message that includes the parameter name.
func parseBoolArg(s, name string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s %q: must be true or false", name, s)
	}
}
