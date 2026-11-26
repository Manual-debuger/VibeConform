package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The rerun rewrites policy.json in the probe copy, one set of removed
// exceptions after another, and puts the original back afterwards, also
// when a rerun fails (spec 0072, "The rerun"). The audit resets the probe
// copy's tracked files before every part, so no later part can see
// whether the rerun restored the file; this test reads it directly.
func TestRerunRestoresPolicyWhenARerunFails(t *testing.T) {
	t.Parallel()
	const original = `{"version": 2, "exceptions": [` + "\n" +
		`  {"rule": "rm-rf", "match": "rm -r -f build", "reason": "a"},` + "\n" +
		`  {"rule": "git-skip-hooks", "match": "git commit --no-verify", "reason": "b"}]}` + "\n"
	rmBuild := guardrule.Exception{Rule: "rm-rf", Match: "rm -r -f build"}
	noVerify := guardrule.Exception{Rule: "git-skip-hooks", Match: "git commit --no-verify"}
	sets := []*rerunSet{{removed: []guardrule.Exception{rmBuild}}, {removed: []guardrule.Exception{noVerify}}}

	p := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(p, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	var ran []string
	failed := map[*rerunSet]string{}
	restoreErr := rewritePolicyPerSet(p, sets, func(set *rerunSet) {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		ran = append(ran, string(data))
		// The first rerun fails and leaves the file removed, as a hook
		// that deletes it would.
		if set == sets[0] {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
		}
	}, func(set *rerunSet, why string) { failed[set] = why })
	if restoreErr != nil {
		t.Fatalf("restore: %v", restoreErr)
	}
	if len(failed) != 0 {
		t.Errorf("sets failed: %v", failed)
	}
	if len(ran) != 2 {
		t.Fatalf("ran %d sets, want 2", len(ran))
	}
	if strings.Contains(ran[0], "rm -r -f build") || !strings.Contains(ran[0], "git commit --no-verify") {
		t.Errorf("the first rerun's policy.json is\n%s\nwant it without rm-rf and with git-skip-hooks", ran[0])
	}
	if !strings.Contains(ran[1], "rm -r -f build") || strings.Contains(ran[1], "git commit --no-verify") {
		t.Errorf("the second rerun's policy.json is\n%s\nwant it with rm-rf and without git-skip-hooks", ran[1])
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("policy.json after the reruns is\n%s\nwant the original\n%s", got, original)
	}
}

// A policy.json that cannot be read fails every set, runs none, and
// writes no file.
func TestRerunPolicyThatCannotBeRead(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "policy.json")
	sets := []*rerunSet{{removed: []guardrule.Exception{{Rule: "rm-rf", Match: "rm -rf"}}}}
	failed := map[*rerunSet]string{}
	restoreErr := rewritePolicyPerSet(p, sets, func(*rerunSet) { t.Error("a set ran") },
		func(set *rerunSet, why string) { failed[set] = why })
	if restoreErr != nil {
		t.Errorf("restore: %v", restoreErr)
	}
	if why := failed[sets[0]]; !strings.Contains(why, "cannot read "+guardPolicyFile+" in the probe copy") {
		t.Errorf("the set failed with %q", why)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("policy.json exists after the reruns: %v", err)
	}
}
