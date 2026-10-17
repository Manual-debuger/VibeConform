// Command vibe-guard is the reference agent guard (spec 0053): Claude
// Code's PreToolUse hook refuses what contract 1.0.0's baseline rules and
// the repository's .claude/hooks/policy.json refuse. It opens no network
// connection.
package main

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// resolveVersion prefers the ldflags version, then the module version a
// "go install …@version" build records, then "dev".
func resolveVersion(ldflags string) string {
	if ldflags != "" && ldflags != "dev" {
		return ldflags
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return "dev"
	}
	return bi.Main.Version
}

func main() {
	dir, _ := os.Getwd()
	os.Exit(guard.Main(os.Args[1:], guard.Env{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
		Dir:    dir,
		Now:    time.Now,
	}, resolveVersion(version)))
}
