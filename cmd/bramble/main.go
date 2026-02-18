package main

import (
	"os"

	"github.com/justinlindh/bramble-cli/internal/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
