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
