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
	exitOutOfDate     = 3
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

// staleBinaryError reports that the running binary is older than the one
// that last wrote .vibe/state.yaml. It is deliberately not a
// nonConformantError: the repository may be in perfect shape, and it is
// this binary's templates that are behind. Saying "not conformant" would
// blame the repository for the operator's install (spec 0019).
type staleBinaryError struct {
	recorded string
	running  string
}

func (e *staleBinaryError) Error() string {
	return fmt.Sprintf("this vibe (%s) is older than the one that last synced "+
		"this repository (%s); upgrade vibe and run again", e.running, e.recorded)
}

// ExitCode maps an error returned by the vibe command tree to a process
// exit code:
//
//	0 — no error.
//	3 — the repository is conformant except that it is out of date: every
//	    managed file is exactly as VibeConform last wrote it, and the
//	    standard has moved on. Distinct from 2 so a conformance job can
//	    choose whether being behind is a failure; non-zero by default,
//	    because the repository is in fact behind.
//	2 — the repository was audited and is not conformant: a managed file
//	    was edited, is missing, or conflicts.
//	1 — anything else: the command could not reach a verdict. Includes a
//	    stale binary, where no verdict is possible.
//
// It unwraps, so a non-conformance error stays recognizable after a command
// has wrapped it with its own name.
func ExitCode(err error) int {
	if err == nil {
		return exitOK
	}

	var nc *nonConformantError
	if errors.As(err, &nc) {
		if nc.drifted == 0 && nc.conflicts == 0 && nc.outOfDate > 0 {
			return exitOutOfDate
		}
		return exitNonConformant
	}
	return exitError
}
