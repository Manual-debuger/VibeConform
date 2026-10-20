package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdHelp(t *testing.T) {
	t.Parallel()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}

	for _, want := range []string{"vibe", "init", "audit", "diff", "sync", "check", "doctor"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help output missing %q\n%s", want, out.String())
		}
	}
}

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

func TestSubcommandsNotYetImplemented(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"check"} {
		root := NewRootCmd("test")
		root.SetArgs([]string{name})
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err == nil {
			t.Errorf("%s: expected not-implemented error, got nil", name)
		}
	}
}
