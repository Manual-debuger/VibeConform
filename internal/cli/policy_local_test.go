package cli

import (
	"strings"
	"testing"
)

const localPolicy = ".claude/hooks/policy.local.json"

// TestGuardPolicyLocal pins spec 0050 D8 to D10 end to end: without the
// file nothing changes; a good file is listed, line by line, and passes; a
// problem is a conflict in audit, diff and sync; sync never writes it.
func TestGuardPolicyLocal(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	out := mustSync(t, dir)
	if exists(t, dir, localPolicy) || strings.Contains(out, localPolicy) {
		t.Fatalf("sync wrote or mentioned an absent %s:\n%s", localPolicy, out)
	}
	if out, err := runAuditIn(t, dir); err != nil || strings.Contains(out, localPolicy) {
		t.Fatalf("audit with no local policy: %v\n%s", err, out)
	}

	good := `{"version": 1, "exceptions": [{"rule": "git-clean-fd", "pattern": "git clean -fdx build/", "reason": "build output"}]}` + "\n"
	writeFile(t, dir, localPolicy, good)
	out, err := runAuditIn(t, dir)
	if err != nil {
		t.Fatalf("audit with a good local policy: %v\n%s", err, out)
	}
	mustContain(t, "audit", out,
		localPolicy+": exception git-clean-fd: git clean -fdx build/ (build output) (project-owned)",
		localPolicy+": ok (project-owned)", "conformant")

	for _, c := range []struct{ name, content, want string }{
		{"broken", `{"version": 2}`, "version is not 1, the only version the guards understand; the guard blocks every tool call until it is fixed"},
		{"unknown rule", `{"version": 1, "exceptions": [{"rule": "nope", "pattern": "x"}]}`, "exception for nope: no rule has that id"},
	} {
		t.Run(c.name, func(t *testing.T) {
			writeFile(t, dir, localPolicy, c.content)
			want := localPolicy + ": conflict: " + c.want
			out, err := runAuditIn(t, dir)
			if code := ExitCode(err); code != 2 {
				t.Fatalf("audit exit %d, want 2:\n%s", code, out)
			}
			mustContain(t, "audit", out, want)
			mustContain(t, "diff", runDiffIn(t, dir), want)
			out, err = runSyncIn(t, dir)
			if err == nil {
				t.Fatalf("sync passed:\n%s", out)
			}
			mustContain(t, "sync", out, want)
			if got := readFile(t, dir, localPolicy); got != c.content {
				t.Errorf("sync wrote the file: %q", got)
			}
		})
	}
}
