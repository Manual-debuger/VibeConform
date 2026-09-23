package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotImplementedPointsAtExistingDoc keeps the "not implemented yet"
// error from pointing at a stale or removed document, as it once pointed at
// an old milestone plan long after that milestone closed.
func TestNotImplementedPointsAtExistingDoc(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(notImplementedDoc))); err != nil {
		t.Fatalf("errNotImplemented points at %s: %v", notImplementedDoc, err)
	}
	for _, cmd := range []string{"check", "doctor"} {
		if got := errNotImplemented(cmd).Error(); !strings.Contains(got, notImplementedDoc) {
			t.Errorf("errNotImplemented(%q) = %q, want it to mention %s", cmd, got, notImplementedDoc)
		}
	}
}
