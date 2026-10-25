package doctor

import (
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// TestEntrypointsAbsolutePOSIXPath: a program written as an absolute
// POSIX path is looked for at that path on every OS, not under the
// repository (regression: on Windows filepath.IsAbs("/usr/bin/x") is
// false, so doctor looked for repo/usr/bin/x).
func TestEntrypointsAbsolutePOSIXPath(t *testing.T) {
	repo := filepath.Join("some", "repo")
	eps := contract.Entrypoints{"verify": {Declared: true, Commands: []string{"/usr/bin/make verify"}}}
	for _, c := range []struct {
		name   string
		exists map[string]bool
		want   Result
	}{
		{"present", map[string]bool{"/usr/bin/make": true}, Result{Status: Pass, Name: "/usr/bin/make", Detail: "found; runs verify"}},
		{"missing, even if the repository has it", map[string]bool{filepath.Join(repo, "usr", "bin", "make"): true},
			Result{Status: Fail, Name: "/usr/bin/make", Detail: "not found; verify cannot run"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Entrypoints((&fake{exists: c.exists}).env(), repo, eps)
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("Entrypoints() = %+v, want [%+v]", got, c.want)
			}
		})
	}
}
