package contract

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Repo built from a known file list holds those files, sorted and once
// each, reads them from the root, and knows no other file. It runs no git:
// the root here is not a Git work tree.
func TestNewRepoHoldsTheListedFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"b.txt", "a/x.txt", "untracked.txt"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := NewRepo(root, []string{"b.txt", "a/x.txt", "b.txt"})
	if got, want := strings.Join(r.Files(), " "), "a/x.txt b.txt"; got != want {
		t.Errorf("Files() = %q, want %q", got, want)
	}
	if r.Root() != root {
		t.Errorf("Root() = %q, want %q", r.Root(), root)
	}
	if !r.HasDir("a") || r.HasDir("c") {
		t.Errorf("HasDir(a) = %v, HasDir(c) = %v, want true, false", r.HasDir("a"), r.HasDir("c"))
	}
	if data, err := r.ReadFile("a/x.txt"); err != nil || string(data) != "a/x.txt" {
		t.Errorf("ReadFile(a/x.txt) = %q, %v", data, err)
	}
	if _, err := r.ReadFile("untracked.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile of an unlisted file: %v, want fs.ErrNotExist", err)
	}
}

// A git run the audit's context ended says so, and names the git command
// and directory: a bare "exit status" names neither.
func TestOpenRepoCanceledNamesGitAndContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()

	_, err := OpenRepo(ctx, dir)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenRepo error = %v, want it to wrap context.Canceled", err)
	}
	for _, want := range []string{"git rev-parse --is-inside-work-tree", dir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("OpenRepo error %q does not name %q", err, want)
		}
	}
	if errors.Is(err, ErrNotWorkTree) {
		t.Errorf("a canceled run reads as not a work tree: %v", err)
	}
}
