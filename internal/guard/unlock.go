package guard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The guardrail-settings rule (spec 0053, "guardrail-settings"): the
// agent edits .claude/settings.json and .claude/hooks/ only while a
// person's unlock is valid, and it can never unlock by itself.

// unlocked reports whether the unlock marker in the common Git directory
// is valid: one RFC 3339 time, still in the future (spec 0051, "The
// unlock marker"). A missing, unreadable or expired marker is not, and
// neither is one in a common directory that is not known.
func (g *Guard) unlocked() bool {
	if g.Repo.CommonDir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(g.Repo.CommonDir, MarkerFile)) // #nosec G304 -- the unlock marker in the repository's own common Git directory
	if err != nil {
		return false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	expiry, err := time.Parse(time.RFC3339, strings.TrimSpace(line))
	return err == nil && g.Now.Before(expiry)
}

var guardrailSettingsRe = regexp.MustCompile(`(?i)(^|/)\.claude/+(settings\.json$|hooks/+[^/])`)

// matchGuardrailSettings reports whether p is .claude/settings.json or a
// path under .claude/hooks/.
func matchGuardrailSettings(p string) bool {
	return guardrailSettingsRe.MatchString(strings.ReplaceAll(p, `\`, "/"))
}

// isMarker reports whether p is the unlock marker: a file named
// vibe-unlock inside a .git directory, or in the repository's Git or
// common Git directory. Names are compared without case, since Windows
// and macOS file systems do.
func (g *Guard) isMarker(p string) bool {
	parts := strings.Split(strings.ReplaceAll(p, `\`, "/"), "/")
	if !strings.EqualFold(parts[len(parts)-1], MarkerFile) {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}
	file := filepath.FromSlash(strings.Join(parts, "/"))
	if !filepath.IsAbs(file) {
		file = filepath.Join(g.Repo.Root, file)
	}
	dir := filepath.Dir(filepath.Clean(file))
	for _, d := range []string{g.Repo.GitDir, g.Repo.CommonDir} {
		if d != "" && strings.EqualFold(filepath.Clean(d), dir) {
			return true
		}
	}
	return false
}

// matchSelfUnlock reports whether c runs vibe unlock or names the unlock
// marker in a word or a redirection target (touch, cp, tee, > …,
// Set-Content, Out-File). A vibe whose subcommand is not known counts as
// vibe unlock.
func matchSelfUnlock(c gr.Command) bool {
	for _, ws := range [][]gr.Word{c.Words, c.Redirects} {
		for _, w := range ws {
			if strings.Contains(strings.ToLower(w.Text), MarkerFile) {
				return true
			}
		}
	}
	if c.Name() != "vibe" {
		return false
	}
	for _, w := range c.Words[1:] {
		switch {
		case !w.Literal:
			return true
		case strings.HasPrefix(w.Text, "-"):
		default:
			return w.Text == "unlock"
		}
	}
	return false
}
