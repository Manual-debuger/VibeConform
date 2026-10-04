package module

import (
	"bytes"
	"fmt"
)

// ReplaceOnce replaces old with replacement, which must occur exactly once
// in data. It is how a module edits an embedded template by exact anchor:
// a template where the anchor is missing or repeated is refused rather
// than half edited. purpose names the edit in the error.
func ReplaceOnce(data []byte, file, purpose, old, replacement string) ([]byte, error) {
	if n := bytes.Count(data, []byte(old)); n != 1 {
		return nil, fmt.Errorf("%s: %s has %d copies of %q, want 1", purpose, file, n, old)
	}
	return bytes.Replace(data, []byte(old), []byte(replacement), 1), nil
}
