package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdHelp(t *testing.T) {
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
	root := NewRootCmd("v0.0.0-test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--version returned error: %v", err)
	}
	if !strings.Contains(out.String(), "v0.0.0-test") {
		t.Errorf("version output missing version string: %s", out.String())
	}
}

func TestSubcommandsNotYetImplemented(t *testing.T) {
	for _, name := range []string{"init", "audit", "diff", "sync", "check", "doctor"} {
		root := NewRootCmd("test")
		root.SetArgs([]string{name})
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})

		if err := root.Execute(); err == nil {
			t.Errorf("%s: expected not-implemented error, got nil", name)
		}
	}
}
