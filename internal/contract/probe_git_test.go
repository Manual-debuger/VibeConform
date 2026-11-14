package contract

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// A probe session finds git on PATH once, however many git commands it
// runs. On Windows every PATH search stats each PATH entry with each
// PATHEXT extension, and go test records each stat in its test log; a
// search per command made the internal/cli test log in the probe copy
// pass its cap, and task audit failed V-01 and R-01 (spec 0061,
// implementation note 10, bc2e574).
func TestProbeSessionLooksUpGitOnce(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	var lookups []string
	s := &probeSession{
		ctx:  context.Background(),
		root: t.TempDir(),
		lookPath: func(name string) (string, error) {
			lookups = append(lookups, name)
			return exec.LookPath(name)
		},
	}
	for _, args := range [][]string{{"init", "-q"}, {"status", "--short"}, {"rev-parse", "--git-dir"}} {
		if _, err := s.git(args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
	if len(lookups) != 1 || lookups[0] != "git" {
		t.Errorf("PATH lookups across three git commands: %q, want one lookup of git", lookups)
	}
}
