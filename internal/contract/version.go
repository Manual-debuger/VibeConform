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
	// Contract2 is the 2.x contract: a declared 2.0.z is audited as it.
	Contract2 = "2.0.0"
	// Latest is the newest contract this vibe implements, which
	// vibe --version shows.
	Latest = Contract2
	// Initial is the contract a new configuration declares: vibe init
	// writes it, and the setup skill names it.
	Initial = Contract1
)

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

// parseVersion checks an X.Y.Z version and returns its MAJOR and MINOR;
// a PATCH makes no conformant repository non-conformant, so it is not
// returned. ok is false for anything else.
func parseVersion(v string) (major, minor int, ok bool) {
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(m[1]); err != nil {
		return 0, 0, false
	}
	if minor, err = strconv.Atoi(m[2]); err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// auditedAs returns the implemented contract a well-formed declared
// version is audited as: the one of the same MAJOR, when the declared
// MINOR is not newer. ok is false when this vibe implements no such
// contract.
func auditedAs(major, minor int) (audited string, ok bool) {
	for _, v := range implemented {
		implMajor, implMinor, _ := parseVersion(v)
		if major == implMajor && minor <= implMinor {
			return v, true
		}
	}
	return "", false
}
