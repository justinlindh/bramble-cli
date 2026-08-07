// Package discovery provides USB serial port auto-detection for Bramble nodes.
package discovery

import (
	"fmt"
	"path/filepath"
	"sort"
)

var globPaths = filepath.Glob

// Detect scans /dev/ttyUSB* and /dev/ttyACM* for potential Bramble nodes.
// Returns the port path if exactly one device is found.
// Returns an error if zero or more than one device is found.
func Detect() (string, error) {
	ports, err := List()
	if err != nil {
		return "", err
	}

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

// List returns every USB serial port that could be a Bramble node, sorted.
// An empty result is not an error: a caller sweeping a fleet decides for itself
// what to say about finding nothing, and Detect turns it into its own guidance.
func List() ([]string, error) {
	var ports []string

	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, err := globPaths(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		ports = append(ports, matches...)
	}
	sort.Strings(ports)
	return ports, nil
}
