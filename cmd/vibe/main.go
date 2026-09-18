// Command vibe is the VibeConform CLI entrypoint.
package main

import (
	"os"

	"github.com/Manual-debuger/VibeConform/internal/cli"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := cli.NewRootCmd(version)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
