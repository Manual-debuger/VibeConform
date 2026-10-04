package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const monoCI = "standard: prod-mono\nversion: v1\ncomponents:\n" +
	"  - id: api\n    path: services/api\n    profile: go\n" +
	"  - id: web\n    path: apps/web\n    profile: ts\n"

var githubFiles = []string{".github/workflows/ci.yml", ".github/dependabot.yml", ".github/pull_request_template.md", ".github/workflows/conformance.yml"}
var gitlabFiles = []string{".gitlab-ci.yml", ".gitlab/merge_request_templates/Default.md", ".gitlab-ci.vibe.yml"}

func withProvider(provider string) string {
	if provider == "" {
		return monoCI
	}
	return monoCI + "ci:\n  provider: " + provider + "\n"
}

func wantFiles(t *testing.T, dir string, files []string, present bool) {
	t.Helper()
	for _, f := range files {
		if exists(t, dir, f) != present {
			t.Errorf("%s exists = %v, want %v", f, !present, present)
		}
	}
}

// TestCIProviderSwitch pins spec 0038 acceptance criterion 8: switching
// provider removes the old provider's unmodified files, forgets the ones
// already deleted, keeps a modified one as a conflict, creates the new
// ones, and round-trips; a second sync changes nothing.
func TestCIProviderSwitch(t *testing.T) {
	for _, to := range []string{"gitlab", "none"} {
		t.Run(to, func(t *testing.T) {
			dir := t.TempDir()
			writeVibeYAML(t, dir, withProvider(""))
			mustSync(t, dir)
			wantFiles(t, dir, githubFiles, true)

			// The adopter already deleted one file.
			if err := os.Remove(filepath.Join(dir, ".github", "dependabot.yml")); err != nil {
				t.Fatal(err)
			}
			writeVibeYAML(t, dir, withProvider(to))
			out := mustSync(t, dir)
			for _, want := range []string{
				".github/workflows/ci.yml: removed (ci.provider deselected",
				".github/dependabot.yml: forgotten (ci.provider deselected",
				".github/workflows/conformance.yml: removed (ci.provider deselected",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("sync lacks %q:\n%s", want, out)
				}
			}
			wantFiles(t, dir, githubFiles, false)
			if exists(t, dir, ".github") {
				t.Error(".github/ survived although every file in it was removed")
			}
			wantFiles(t, dir, gitlabFiles, to == "gitlab")
			if tf := readFile(t, dir, "Taskfile.yml"); strings.Contains(tf, "workflows:lint") {
				t.Error("Taskfile.yml still has workflows:lint")
			}
			mustConform(t, dir)
			if out := mustSync(t, dir); !strings.Contains(out, "0 created, 0 updated") || strings.Contains(out, "removed") {
				t.Errorf("second sync changed something:\n%s", out)
			}

			// And back.
			writeVibeYAML(t, dir, withProvider("github"))
			mustSync(t, dir)
			wantFiles(t, dir, githubFiles, true)
			wantFiles(t, dir, gitlabFiles, false)
			mustConform(t, dir)
		})
	}

	t.Run("modified file is a conflict", func(t *testing.T) {
		dir := t.TempDir()
		writeVibeYAML(t, dir, withProvider(""))
		mustSync(t, dir)
		ci := filepath.Join(dir, ".github", "workflows", "ci.yml")
		if err := os.WriteFile(ci, []byte("# mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeVibeYAML(t, dir, withProvider("gitlab"))
		out, err := runSyncIn(t, dir)
		if err == nil || !strings.Contains(out, ".github/workflows/ci.yml") || !strings.Contains(out, "conflict") {
			t.Errorf("sync: %v\n%s", err, out)
		}
		if got := readFile(t, dir, ".github/workflows/ci.yml"); got != "# mine\n" {
			t.Errorf("a modified file was changed: %q", got)
		}
	})
}

// TestCIProviderHandWrittenPipeline pins spec 0038 acceptance criterion 9:
// an unrecorded .gitlab-ci.yml is a conflict and is left byte for byte.
func TestCIProviderHandWrittenPipeline(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, withProvider(""))
	mustSync(t, dir)
	mine := "stages: [build]\nbuild:\n  script: [make]\n"
	writeFile(t, dir, ".gitlab-ci.yml", mine)
	writeVibeYAML(t, dir, withProvider("gitlab"))
	out, err := runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, ".gitlab-ci.yml") || !strings.Contains(out, "conflict") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, ".gitlab-ci.yml"); got != mine {
		t.Errorf(".gitlab-ci.yml = %q", got)
	}
}

// TestCIProviderErrors pins spec 0038 acceptance criteria 2 and 7 through
// the CLI: each refusal exits 1 before anything is written.
func TestCIProviderErrors(t *testing.T) {
	for name, c := range map[string]struct{ doc, want string }{
		"prod-go":       {"standard: prod-go\nversion: v1\nci:\n  provider: gitlab\n", "prod-go/v1 offers no provider setting"},
		"unknown value": {withProvider("bitbucket"), "unknown value (valid: github, gitlab, none)"},
		"unknown key":   {monoCI + "ci:\n  providr: gitlab\n", "providr"},
		"reserved id":   {"standard: prod-mono\nversion: v1\ncomponents:\n  - id: pages\n    path: site\n    profile: ts\nci:\n  provider: gitlab\n", "GitLab CI keyword"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeVibeYAML(t, dir, c.doc)
			out, err := runSyncIn(t, dir)
			wantExit(t, err, 1, out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %v, want it to contain %q", err, c.want)
			}
			if exists(t, dir, ".vibe/state.yaml") {
				t.Error("sync wrote state")
			}
		})
	}
}
