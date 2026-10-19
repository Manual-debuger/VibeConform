package guard_test

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// TestInstallVersion: the install hint names the running vibe's version
// when go install can fetch it, and latest otherwise (spec 0053, "The
// reference hook commands"; spec 0054, "The install hint").
func TestInstallVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v0.12.0":         "v0.12.0",
		"v0.12.0-alpha.1": "v0.12.0-alpha.1",
		// A pseudo-version changes with every commit of a repository
		// that builds vibe from source, as this one's Conformance /
		// audit job does; naming it made that job fail on every commit.
		"v0.11.0-alpha.1.0.20261017172609-46246ad76fbf":       "latest",
		"v0.11.0-alpha.1.0.20261017172609-46246ad76fbf+dirty": "latest",
		"dev":     "latest",
		"(devel)": "latest",
		"":        "latest",
		"test":    "latest",
	} {
		if got := guard.InstallVersion(in); got != want {
			t.Errorf("InstallVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestReferencePreToolUseNamesTheReleasePage: the install hint keeps go
// install and adds the release page, on one line (spec 0054, "The
// install hint"). For latest the link is to the releases page.
func TestReferencePreToolUseNamesTheReleasePage(t *testing.T) {
	for version, want := range map[string]string{
		"v0.12.0-alpha.1": `echo "vibe-guard is not installed; install it: ` +
			`go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@v0.12.0-alpha.1, ` +
			`or download it from https://github.com/Manual-debuger/VibeConform/releases/tag/v0.12.0-alpha.1" >&2; exit 2; }`,
		"latest": `echo "vibe-guard is not installed; install it: ` +
			`go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@latest, ` +
			`or download it from https://github.com/Manual-debuger/VibeConform/releases" >&2; exit 2; }`,
	} {
		got := guard.ReferencePreToolUse(version)
		if !strings.Contains(got, want) {
			t.Errorf("ReferencePreToolUse(%q) = %q, want it to contain %q", version, got, want)
		}
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("ReferencePreToolUse(%q) is not one line", version)
		}
	}
}
