// Package discovery provides USB serial port auto-detection for Bramble nodes.
package discovery

import (
	"fmt"
	"path/filepath"
	"sort"
)

// Detect scans /dev/ttyUSB* and /dev/ttyACM* for potential Bramble nodes.
// Returns the port path if exactly one device is found.
// Returns an error if zero or more than one device is found.
func Detect() (string, error) {
	var ports []string

	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("glob %s: %w", pattern, err)
		}
		ports = append(ports, matches...)
	}
	sort.Strings(ports)

	switch len(ports) {
	case 0:
		return "", fmt.Errorf(
			"no USB serial devices found\n" +
				"  Connect your Bramble node and try again, or specify a port:\n" +
				"    bramble --port /dev/ttyUSB0 <command>\n" +
				"  Or use a WebSocket transport:\n" +
				"    bramble --transport ws://192.168.4.1/rpc <command>",
		)
	case 1:
		return ports[0], nil
	default:
		list := ""
		for _, p := range ports {
			list += "\n    " + p
		}
		return "", fmt.Errorf(
			"multiple USB serial devices found — specify one with --port:%s", list,
		)
	}
}
