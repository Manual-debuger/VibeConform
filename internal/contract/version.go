package contract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The contract versions this vibe implements, side by side: the latest
// MINOR of each MAJOR it audits (spec 0072, "Contract versions").
const (
	// Contract1 is the 1.x contract: a declared 1.y.z is audited as it.
	Contract1 = "1.2.0"
	// Contract2 is the 2.x contract: a declared 2.0.z or 2.1.z is audited
	// as it (spec 0074).
	Contract2 = "2.1.0"
	// Latest is the newest contract this vibe implements. vibe --version
	// shows it, and a new configuration declares it: vibe init writes it,
	// and the setup skill names it (spec 0072).
	Latest = Contract2
)

// contract21 is contract 2.1.0, which made the guard set SHOULD, C-05
// follow platforms and the rules that test the guard not applicable
// without one (spec 0074). The rule table's revisions and level changes
// name it, and so do the checks whose reading it changed.
const contract21 = "2.1.0"

// implemented lists the contracts this vibe implements, oldest first.
var implemented = []string{Contract1, Contract2}

// Versions is the contract version a configuration declares and the one
// the audit checks it against. They differ for an older MINOR of the same
// MAJOR, such as 1.0.0 or 1.0.1, and for a newer PATCH: both are audited
// as the contract this vibe implements for that MAJOR, since a vibe
// implements the latest MINOR of each MAJOR it audits and a PATCH makes no
// conformant repository non-conformant (spec 0051, "Versioning"; spec
// 0052, "Contract versions"; spec 0072, "Contract versions"); the report
// shows both.
type Versions struct {
	Declared string
	Audited  string
}

// UnsupportedVersionError is a declared contract version this vibe cannot
// audit: a MAJOR it does not implement, or a newer MINOR. There is no
// verdict (spec 0051, "Versioning").
type UnsupportedVersionError struct {
	file     string
	declared string
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("%s: contract: cannot decide; upgrade vibe: this vibe implements contracts %s, "+
		"and the configuration declares %s", e.file, strings.Join(implemented, " and "), e.declared)
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// versionParts checks an X.Y.Z version and returns its three numbers.
// ok is false for anything else, and for a MAJOR or MINOR too large to
// read; a PATCH makes no conformant repository non-conformant, so one too
// large reads as the largest int.
func versionParts(v string) (parts [3]int, ok bool) {
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return parts, false
	}
	ok = true
	for i := range parts {
		var err error
		// Atoi returns the largest int with its range error.
		if parts[i], err = strconv.Atoi(m[i+1]); err != nil && i < 2 {
			ok = false
		}
	}
	return parts, ok
}

// compareVersions orders two X.Y.Z versions: negative when a is older
// than b, zero when they are equal, positive when a is newer. A version
// that is not X.Y.Z sorts as 0.0.0; the callers pass well-formed ones.
func compareVersions(a, b string) int {
	pa, _ := versionParts(a)
	pb, _ := versionParts(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

// auditedAs returns the implemented contract a well-formed declared
// version is audited as: the one of the same MAJOR, when the declared
// MINOR is not newer. ok is false when this vibe implements no such
// contract.
func auditedAs(major, minor int) (audited string, ok bool) {
	for _, v := range implemented {
		impl, _ := versionParts(v)
		if major == impl[0] && minor <= impl[1] {
			return v, true
		}
	}
	return "", false
}
