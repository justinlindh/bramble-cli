package commands

import (
	"fmt"
	"strconv"
)

// ParseAddress parses a mesh address from a hex or decimal string.
// Accepts: "DEADBEEF", "0xDEADBEEF", "3735928559".
func ParseAddress(s string) (uint32, error) {
	hex := s
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		hex = s[2:]
	}

	n, err := strconv.ParseUint(hex, 16, 32)
	if err == nil {
		return uint32(n), nil
	}

	n, err = strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("bramble-cli: expected hex (e.g. DEADBEEF) or decimal integer")
	}
	return uint32(n), nil
}
