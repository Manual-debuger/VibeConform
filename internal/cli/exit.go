package cli

import (
	"errors"
	"fmt"
)

// Process exit codes. These are part of the CLI's contract with CI: a
// pipeline needs to tell "this repository has drifted" from "the tool could
// not answer", and only the first is fixed by running vibe sync.
const (
	exitOK            = 0
	exitError         = 1
	exitNonConformant = 2
)

// nonConformantError reports that a command successfully audited a
// repository and found it out of conformance. It is distinct from every
// other error a command can return, all of which mean the command failed to
// reach a verdict at all.
type nonConformantError struct {
	drifted   int
	outOfDate int
	conflicts int
}

func (e *nonConformantError) Error() string {
	return fmt.Sprintf("not conformant: %d drifted, %d out of date, %d conflicts",
		e.drifted, e.outOfDate, e.conflicts)
}

// ExitCode maps an error returned by the vibe command tree to a process
// exit code:
//
//	0 — no error.
//	2 — the repository was audited and is not conformant.
//	1 — anything else: the command could not reach a verdict.
//
// It unwraps, so a non-conformance error stays recognizable after a command
// has wrapped it with its own name.
func ExitCode(err error) int {
	if err == nil {
		return exitOK
	}

	var nc *nonConformantError
	if errors.As(err, &nc) {
		return exitNonConformant
	}
	return exitError
}
