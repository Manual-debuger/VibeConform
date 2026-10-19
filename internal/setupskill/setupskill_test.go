package setupskill_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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

// TestImportsOnlyGuard: among this repository's packages, setupskill
// imports only internal/guard, so that #112 can remove the old packages
// without touching it.
func TestImportsOnlyGuard(t *testing.T) {
	const internal = "github.com/Manual-debuger/VibeConform/internal/"
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, internal) && p != internal+"guard" {
				t.Errorf("%s imports %s", path, p)
			}
		}
	}
}
