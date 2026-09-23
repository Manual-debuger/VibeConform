package main

import "testing"

// TestResolveVersionPrefersLdflags covers a GoReleaser build, where the
// stamped value is authoritative.
func TestResolveVersionPrefersLdflags(t *testing.T) {
	if got := resolveVersion("v0.3.0"); got != "v0.3.0" {
		t.Errorf("resolveVersion(v0.3.0) = %q, want v0.3.0", got)
	}
}

// TestResolveVersionFallsBackToBuildInfo pins the gap that makes spec
// 0019's guard reach real users. "go install .../cmd/vibe@latest" — the
// exact command the generated CI conformance job runs — compiles from
// source without GoReleaser, so main.version keeps its "dev" default even
// though the binary is a specific published release. Reporting "dev" there
// would leave every adopter's version unorderable, and the stale-binary
// guard permanently dormant.
//
// Under "go test" the build info reports the test binary, so this asserts
// the contract rather than a specific version: an unstamped build must not
// simply echo "dev" back when the build info knows better, and must fall
// back to "dev" when it does not.
func TestResolveVersionFallsBackToBuildInfo(t *testing.T) {
	got := resolveVersion(devVersion)
	if got == "" {
		t.Fatal("resolveVersion returned empty; it must always name something")
	}
	if got != devVersion && got[0] != 'v' {
		t.Errorf("resolveVersion(dev) = %q; a build-info fallback must look "+
			"like a module version", got)
	}
}

// TestResolveVersionTreatsEmptyAsUnstamped guards the case where someone
// passes -X main.version= with nothing after it.
func TestResolveVersionTreatsEmptyAsUnstamped(t *testing.T) {
	if got := resolveVersion(""); got == "" {
		t.Error("resolveVersion(\"\") returned empty, want a fallback")
	}
}
