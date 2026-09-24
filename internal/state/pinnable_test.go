package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestPinnable reads the table internal/module/conformance's
// TestAuditFallbackNeverPassesSilently also runs through task audit's shell,
// so vibe sync's warning and task audit's fallback agree on every case.
func TestPinnable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pinnable_versions.json"))
	if err != nil {
		t.Fatalf("reading shared version table: %v", err)
	}
	var cases []struct {
		Version  string `json:"version"`
		Pinnable bool   `json:"pinnable"`
		Why      string `json:"why"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parsing shared version table: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("shared version table is empty")
	}

	for _, c := range cases {
		if got := Pinnable(c.Version); got != c.Pinnable {
			t.Errorf("Pinnable(%q) = %v, want %v (%s)", c.Version, got, c.Pinnable, c.Why)
		}
	}
}
