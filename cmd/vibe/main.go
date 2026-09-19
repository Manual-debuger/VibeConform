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
	// Exit code carries meaning: 2 means "audited, not conformant", which CI
	// needs to tell apart from 1, "the tool could not answer".
	os.Exit(cli.ExitCode(root.Execute()))
}
