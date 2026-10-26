package cli

import (
	"errors"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// Process exit codes. These are part of the CLI's contract with CI: a
// pipeline needs to tell "this repository is not conformant" from "the
// tool could not answer" (spec 0052, "Exit codes").
const (
	exitOK            = 0
	exitError         = 1
	exitNonConformant = 2
)

// ExitCode maps an error returned by the vibe command tree to a process
// exit code:
//
//	0 — no error.
//	2 — the contract audit reached the verdict not conformant.
//	1 — anything else: the command could not reach a verdict, or failed.
//
// It unwraps, so a verdict stays recognizable after a command has wrapped
// it with its own name.
func ExitCode(err error) int {
	if err == nil {
		return exitOK
	}
	var cv *contractVerdictError
	if errors.As(err, &cv) && cv.verdict == contract.VerdictNotConformant {
		return exitNonConformant
	}
	return exitError
}
