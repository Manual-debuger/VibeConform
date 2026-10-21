package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

// warnUnpinnable reports on w when sync is about to record a vibe_version
// that task audit's fallback cannot fetch: a dev or +dirty build (spec
// 0022, 3a). CI's conformance job relies on that fallback, so a repository
// synced this way fails CI unless vibe is on PATH there — better said at
// sync time than a push later.
//
// A warning, never a failure, for the same reason as warnMissingTools: the
// files sync writes are correct. Skipped when the repository provides
// cmd/vibe, which in practice is VibeConform itself (ADR 0008). There the
// conformance job's setup action resolves its source to workspace, builds
// vibe from the checkout and takes the recorded version as information
// only, so dev and +dirty pass CI; task audit builds ./cmd/vibe too (spec
// 0057). The setup action and task audit also check go.mod's module path;
// this check is looser, which costs at most a missing warning.
func warnUnpinnable(w io.Writer, repoRoot, version string) {
	if state.Pinnable(version) {
		return
	}
	if info, err := os.Stat(filepath.Join(repoRoot, "cmd", "vibe")); err == nil && info.IsDir() {
		return
	}
	// Dropped write error: a warning that could not be printed must not
	// fail the command it was only advising.
	_, _ = fmt.Fprintf(w, "warning: vibe %s is not a released or pseudo-version, so task audit\n"+
		"cannot pin it; CI's conformance check will fail unless vibe is on PATH.\n"+
		"Sync with a released vibe (go install github.com/Manual-debuger/VibeConform/cmd/vibe@<tag>) before pushing.\n",
		version)
}
