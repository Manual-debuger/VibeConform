package doctor_test

import (
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// TestEntrypointsAbsolutePOSIXPath: a program written as an absolute
// POSIX path is looked for at that path on every OS, not under the
// repository (regression: on Windows filepath.IsAbs("/usr/bin/x") is
// false, so doctor looked for repo/usr/bin/x).
func TestEntrypointsAbsolutePOSIXPath(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		exists func(dir string) map[string]bool
		want   doctor.Result
	}{
		{"present", func(string) map[string]bool { return map[string]bool{"/usr/bin/make": true} },
			doctor.Result{Status: doctor.Pass, Name: "/usr/bin/make", Detail: "found; runs verify"}},
		{"missing, even if the repository has it",
			func(dir string) map[string]bool {
				return map[string]bool{filepath.Join(dir, "usr", "bin", "make"): true}
			},
			doctor.Result{Status: doctor.Fail, Name: "/usr/bin/make", Detail: "not found; verify cannot run"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, loaded := load(t, contracttest.WithFile("vibe.yaml",
				"contract: 1.2.0\nplatforms: [linux]\nagents: []\nentrypoints:\n  verify: /usr/bin/make verify\n"))
			if loaded.Err != nil {
				t.Fatal(loaded.Err)
			}
			got := run(dir, machine{exists: c.exists(dir)}, loaded)
			if len(got) < 3 || got[2] != c.want {
				t.Errorf("results = %+v, want the entrypoint line %+v", got, c.want)
			}
		})
	}
}
