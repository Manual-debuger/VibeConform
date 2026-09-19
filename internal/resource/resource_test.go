package resource

import (
	"os"
	"testing"
)

func TestModeOrDefaultUsesDefaultForZeroValue(t *testing.T) {
	r := Resource{Path: ".golangci.yml", Ownership: Generated}
	if got := r.ModeOrDefault(); got != 0o644 {
		t.Errorf("ModeOrDefault() = %v, want %v — an unset mode must fall back to the default", got, os.FileMode(0o644))
	}
}

func TestModeOrDefaultKeepsExplicitMode(t *testing.T) {
	r := Resource{Path: ".claude/hooks/block-dangerous.sh", Ownership: Generated, Mode: 0o755}
	if got := r.ModeOrDefault(); got != 0o755 {
		t.Errorf("ModeOrDefault() = %v, want %v", got, os.FileMode(0o755))
	}
}
