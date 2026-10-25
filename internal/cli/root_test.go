package cli

import (
	"bytes"
	"testing"
)

func TestRootCmdVersion(t *testing.T) {
	t.Parallel()
	root := NewRootCmd("v0.0.0-test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--version returned error: %v", err)
	}
	// vibe --version names the contract version vibe implements (spec
	// 0054, "vibe --version"); the setup skill's upgrade flow reads it.
	if got, want := out.String(), "vibe version v0.0.0-test (contract 1.1.0)\n"; got != want {
		t.Errorf("vibe --version printed %q, want %q", got, want)
	}
}
