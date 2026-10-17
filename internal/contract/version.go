package contract

import (
	"fmt"
	"regexp"
	"strconv"
)

// Implemented is the contract version this vibe implements.
const Implemented = "1.0.0"

// Versions is the contract version a configuration declares and the one
// the audit checks it against. They differ only for a newer PATCH, which
// changes no verdict and is audited as Implemented (spec 0052, "Contract
// versions"); the report shows both.
type Versions struct {
	Declared string
	Audited  string
}

// UnsupportedVersionError is a declared contract version this vibe cannot
// audit: another MAJOR, or a newer MINOR. There is no verdict (spec 0051,
// "Versioning").
type UnsupportedVersionError struct {
	File     string
	Declared string
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("%s: contract: cannot decide; upgrade vibe: this vibe implements contract %s, "+
		"and the configuration declares %s", e.File, Implemented, e.Declared)
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// parseVersion checks an X.Y.Z version and returns its MAJOR and MINOR;
// PATCH changes no verdict. ok is false for anything else.
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

// supported reports whether a well-formed declared version can be audited
// with Implemented: the same MAJOR and no newer MINOR.
func supported(major, minor int) bool {
	return major == 1 && minor == 0
}
