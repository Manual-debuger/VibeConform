// Command vibe is the VibeConform CLI entrypoint.
package main

import (
	"os"
	"runtime/debug"

	"github.com/Manual-debuger/VibeConform/internal/cli"
)

// version is overridden at build time via -ldflags "-X main.version=...".
// Only GoReleaser does that, so a binary built any other way falls back to
// resolveVersion's build-info lookup.
var version = "dev"

// devVersion is what an unstamped build reports. It is deliberately not
// valid semver: state.CompareWriters refuses to order it, so spec 0019's
// downgrade guard stays dormant rather than guessing.
const devVersion = "dev"

// resolveVersion reports the version this binary should call itself.
//
// ldflags wins when present, but it is absent for the way most people get
// vibe: "go install github.com/.../cmd/vibe@latest" compiles from source
// without GoReleaser, so main.version keeps its default and the binary
// would report "dev" despite being a specific published release. That is
// not cosmetic — the generated CI conformance job installs vibe exactly
// that way, so every adopter's binary would be unorderable and spec 0019's
// stale-binary guard would never fire for the population it exists to
// protect.
//
// The module version is recorded in the build info regardless, so read it
// from there. It is "(devel)" for a plain "go build" from a work tree,
// which is no more orderable than "dev" and is reported as such.
func resolveVersion(ldflags string) string {
	if ldflags != "" && ldflags != devVersion {
		return ldflags
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return devVersion
	}
	return bi.Main.Version
}

func main() {
	root := cli.NewRootCmd(resolveVersion(version))
	// Exit code carries meaning: 2 means "audited, not conformant" and 3
	// "audited, merely out of date", which CI needs to tell apart from 1,
	// "the tool could not answer".
	os.Exit(cli.ExitCode(root.Execute()))
}
