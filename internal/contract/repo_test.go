package contract

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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
