package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClaudeIgnoreSection pins spec 0039 acceptance criterion 3: claude
// adds its .gitignore section below the project's own lines, and
// deselecting claude removes exactly that section.
func TestClaudeIgnoreSection(t *testing.T) {
	dir := t.TempDir()
	user := "bin/\n.claude/settings.local.json\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\n")
	if out := mustSync(t, dir); !strings.Contains(out, ".gitignore (section claude): added") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, ".gitignore"); got != user+"\n"+claudeSection {
		t.Errorf(".gitignore = %q", got)
	}
	mustConform(t, dir)

	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\nintegrations:\n  agents: [codex]\n")
	if out := mustSync(t, dir); !strings.Contains(out, ".gitignore (section claude): removed (claude deselected)") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, ".gitignore"); got != user {
		t.Errorf(".gitignore = %q, want the user's lines alone", got)
	}
	mustConform(t, dir)
}
