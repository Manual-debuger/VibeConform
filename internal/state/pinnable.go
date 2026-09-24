package state

import "strings"

// Pinnable reports whether v, a vibe_version as recorded in state.yaml, is
// one the Go toolchain can fetch again: a release tag or a pseudo-version,
// starting "v<digit>" and carrying no "+build" suffix. "dev", "(devel)",
// and "+dirty" builds are not — no module proxy serves them.
//
// Taskfile.vibe.yml's task audit applies the same rule in shell to decide
// whether it can install the recorded version when vibe is not on PATH,
// and vibe sync warns when it is about to record a version that fails it
// (spec 0022). The two implementations share one test table,
// testdata/pinnable_versions.json, so they cannot drift apart.
func Pinnable(v string) bool {
	if len(v) < 2 || v[0] != 'v' || v[1] < '0' || v[1] > '9' {
		return false
	}
	return !strings.Contains(v, "+")
}
