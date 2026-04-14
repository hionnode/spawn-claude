package main

import (
	"os"

	"github.com/hionnode/spawn-claude/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
