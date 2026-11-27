package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baselineFixture writes a tree with three complexity baselines that
// count, and baseline text that does not: in a testdata directory, in a
// directory whose name starts with a dot, in a string literal, in a
// comment that is no nolint directive and in a file that is not Go.
func baselineFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"a.go": "package a\n\n" +
			"func f() { //nolint:gocognit // baseline: pre-existing complexity\n}\n\n" +
			"func g() { //nolint:gocognit,gocyclo // baseline: pre-existing complexity\n}\n\n" +
			"// h says baseline: pre-existing complexity in a plain comment.\n" +
			"func h() string { return \"//nolint:gocyclo // baseline: pre-existing complexity\" }\n",
		"sub/b.go": "package sub\n\n" +
			"func f() { //nolint:gocyclo // baseline: pre-existing complexity\n}\n",
		"sub/testdata/c.go": "package c\n\n" +
			"func f() { //nolint:gocyclo // baseline: pre-existing complexity\n}\n",
		".claude/worktrees/x/d.go": "package d\n\n" +
			"func f() { //nolint:gocyclo // baseline: pre-existing complexity\n}\n",
		"notes.md": "//nolint:gocyclo // baseline: pre-existing complexity\n",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCheckPassesAtTheCeiling(t *testing.T) {
	var out strings.Builder
	if err := check(baselineFixture(t), 3, &out); err != nil {
		t.Fatalf("check at the ceiling: %v", err)
	}
}

func TestCheckFailsAboveTheCeiling(t *testing.T) {
	var out strings.Builder
	err := check(baselineFixture(t), 2, &out)
	if err == nil {
		t.Fatal("check with one baseline more than the ceiling passed")
	}
	for _, want := range []string{"3 complexity baselines", "ceiling is 2", "simplify the function"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

// A count below the ceiling passes, and says the ceiling to record, so
// that the change that removed a baseline lowers it.
func TestCheckBelowTheCeilingNamesTheNewCeiling(t *testing.T) {
	var out strings.Builder
	if err := check(baselineFixture(t), 4, &out); err != nil {
		t.Fatalf("check below the ceiling: %v", err)
	}
	if !strings.Contains(out.String(), "lower the ceiling to 3") {
		t.Errorf("output %q does not name the new ceiling", out.String())
	}
}
