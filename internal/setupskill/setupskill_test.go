package setupskill_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guard"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// TestNoCarriageReturn: a CRLF working copy would change the bytes go:embed
// ships (spec 0054, "Source"), so no embedded file may hold a carriage
// return. .gitattributes forces LF for the directory.
func TestNoCarriageReturn(t *testing.T) {
	n := 0
	err := fs.WalkDir(setupskill.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		data, err := fs.ReadFile(setupskill.FS(), p)
		if err != nil {
			return err
		}
		if bytes.ContainsRune(data, '\r') {
			t.Errorf("%s holds a carriage return", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("the embedded skill has no files")
	}
}

// TestSkillLength: SKILL.md, the process an agent reads in full, has at
// most 1500 words (spec 0054, "Layout").
func TestSkillLength(t *testing.T) {
	data, err := fs.ReadFile(setupskill.FS(), "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(string(data))); n > 1500 {
		t.Errorf("SKILL.md has %d words, want at most 1500", n)
	}
	if !bytes.HasPrefix(data, []byte("---\nname: "+setupskill.Name+"\n")) {
		t.Errorf("SKILL.md's front matter does not name %s", setupskill.Name)
	}
}

// TestSkillListsEveryOptionalReference: SKILL.md is the skill's index, and
// an agent reads an optional reference only when the person asks for what
// it covers, so SKILL.md names each file under reference/optional/.
func TestSkillListsEveryOptionalReference(t *testing.T) {
	skill, err := fs.ReadFile(setupskill.FS(), "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	optional, err := fs.ReadDir(setupskill.FS(), "reference/optional")
	if err != nil {
		t.Fatal(err)
	}
	if len(optional) == 0 {
		t.Fatal("reference/optional/ has no files")
	}
	for _, e := range optional {
		if !bytes.Contains(skill, []byte("`reference/optional/"+e.Name()+"`")) {
			t.Errorf("SKILL.md does not name reference/optional/%s", e.Name())
		}
	}
	if _, err := fs.Stat(setupskill.FS(), "reference/optional/dependency-direction.md"); err != nil {
		t.Errorf("the optional dependency-direction reference is missing: %v", err)
	}
}

// TestReferenceHooksAreTheGuards: the rendered reference settings run
// internal/guard's reference PreToolUse and Stop commands, for the
// version the version rule picks.
func TestReferenceHooksAreTheGuards(t *testing.T) {
	for _, vibeVersion := range []string{"v0.12.0-alpha.1", "dev"} {
		files, err := setupskill.Render(vibeVersion)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(files["reference/agents/settings.json"], &cfg); err != nil {
			t.Fatalf("reference/agents/settings.json: %v", err)
		}
		pre, stop := cfg.Hooks["PreToolUse"], cfg.Hooks["Stop"]
		if len(pre) != 1 || len(pre[0].Hooks) != 1 || pre[0].Matcher != guard.ReferencePreToolUseMatcher ||
			pre[0].Hooks[0].Command != guard.ReferencePreToolUse(guard.InstallVersion(vibeVersion)) {
			t.Errorf("vibe %s: PreToolUse = %+v, want the reference command and matcher", vibeVersion, pre)
		}
		if len(stop) != 1 || len(stop[0].Hooks) != 1 || stop[0].Hooks[0].Command != guard.ReferenceStop {
			t.Errorf("vibe %s: Stop = %+v, want the reference command", vibeVersion, stop)
		}
	}
}

// TestReferenceStopIsRelockThenStopGate: the reference settings' Stop
// command relocks first, whatever the gate does, then runs spec 0057's
// Stop gate unchanged from the project root (spec 0058), so the green
// stamp works the same as without the guard. The gate is spelled out
// here, independently of internal/guard. Replaces the deleted
// internal/module/agents/claude TestReferenceStopIsRelockThenStopCommand.
func TestReferenceStopIsRelockThenStopGate(t *testing.T) {
	const gate = "task -x hook:stamp:check >/dev/null 2>&1 </dev/null || " +
		"{ task verify:fast 1>&2 </dev/null && task -x hook:stamp:save >/dev/null 2>&1 </dev/null; } || " +
		"task -x hook:done"
	want := "vibe-guard relock 2>/dev/null; " + `cd "${CLAUDE_PROJECT_DIR:-.}" && ` + "{ " + gate + "; }"
	if got := refStopCommand(t); got != want {
		t.Errorf("the reference Stop command =\n%s\nwant\n%s", got, want)
	}
}
