package contracttest

import (
	"path/filepath"
	"testing"
)

func TestFixturePathMapsDotClaude(t *testing.T) {
	t.Parallel()
	in := filepath.Join("dot-claude", "hooks", "policy.json")
	if got, want := fixturePath(in), filepath.Join(".claude", "hooks", "policy.json"); got != want {
		t.Errorf("fixturePath(%q) = %q, want %q", in, got, want)
	}
	if got := fixturePath("AGENTS.md"); got != "AGENTS.md" {
		t.Errorf("fixturePath changed a plain path: %q", got)
	}
}
