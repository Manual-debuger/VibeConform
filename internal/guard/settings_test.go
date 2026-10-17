package guard_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// repoRoot is this repository, which runs vibe-guard through
// policy.agent_guard (spec 0053, "This repository").
var repoRoot = filepath.Join("..", "..")

// repositoryHook returns the PreToolUse matcher and command of this
// repository's .claude/settings.json, which must be exactly one hook.
func repositoryHook(t *testing.T) (matcher, command string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing .claude/settings.json: %v", err)
	}
	pre := cfg.Hooks["PreToolUse"]
	if len(pre) != 1 || len(pre[0].Hooks) != 1 || pre[0].Hooks[0].Type != "command" {
		t.Fatalf("PreToolUse = %+v, want one command hook", pre)
	}
	return pre[0].Matcher, pre[0].Hooks[0].Command
}

// TestRepositorySettingsRunTheReferenceHook: this repository's settings
// run the reference PreToolUse command, with the reference matcher. Only
// the version its install hint names may differ.
func TestRepositorySettingsRunTheReferenceHook(t *testing.T) {
	matcher, command := repositoryHook(t)
	if matcher != guard.ReferencePreToolUseMatcher {
		t.Errorf("matcher %q, want %q", matcher, guard.ReferencePreToolUseMatcher)
	}
	const marker = "\x00version\x00"
	before, after, _ := strings.Cut(guard.ReferencePreToolUse(marker), marker)
	version, ok := strings.CutPrefix(command, before)
	if ok {
		version, ok = strings.CutSuffix(version, after)
	}
	if !ok || version == "" || strings.ContainsAny(version, " \"'") || command != guard.ReferencePreToolUse(version) {
		t.Errorf("PreToolUse command %q is not the reference command", command)
	}
}

// TestRepositorySettingsHookCorpus runs the whole contract corpus through
// the exact PreToolUse command of this repository's settings, through
// bash, with only vibe-guard on PATH and this repository's policy.json.
// CI's hook-guard workflow sets VIBE_GUARD_SETTINGS_E2E=1 on Linux and
// Windows; the reference command's own corpus runs already cover the
// same decisions in task verify, so this is skipped otherwise.
func TestRepositorySettingsHookCorpus(t *testing.T) {
	if os.Getenv("VIBE_GUARD_SETTINGS_E2E") == "" {
		t.Skip("VIBE_GUARD_SETTINGS_E2E not set")
	}
	bash := findBash(t)
	_, command := repositoryHook(t)
	policy, err := os.ReadFile(filepath.Join(repoRoot, ".claude", "hooks", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	data, err := os.ReadFile(guardBin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vibe-guard"+exeSuffix()), data, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range contract.GuardCorpus() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := corpusRepo(t, c)
			writeFile(t, filepath.Join(r.Root, ".claude", "hooks", "policy.json"), string(policy))
			cmd := exec.Command(bash, "-c", command)
			cmd.Dir = r.Root
			var env []string
			for _, kv := range os.Environ() {
				if k, _, _ := strings.Cut(kv, "="); !strings.EqualFold(k, "PATH") {
					env = append(env, kv)
				}
			}
			cmd.Env = append(env, "PATH="+dir, "CLAUDE_PROJECT_DIR="+r.Root)
			cmd.Stdin = strings.NewReader(payloadFor(t, c, r.Root))
			var errOut bytes.Buffer
			cmd.Stderr = &errOut
			code := 0
			var exit *exec.ExitError
			switch err := cmd.Run(); {
			case errors.As(err, &exit):
				code = exit.ExitCode()
			case err != nil:
				t.Fatalf("running bash: %v", err)
			}
			want := 0
			if c.Want == contract.WantDeny {
				want = 2
			}
			if code != want || want == 2 && !strings.Contains(errOut.String(), "refused by rule "+c.Rule+":") {
				t.Errorf("exit %d, stderr %q; want exit %d", code, errOut.String(), want)
			}
		})
	}
}
