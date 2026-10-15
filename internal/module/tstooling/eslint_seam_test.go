package tstooling

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// seedExport is the seed's last line, where an adopter adds entries.
const seedExport = "export default [...base]\n"

// TestESLintSeam runs real ESLint through the seed (spec 0049): the base's
// typed rules apply through it, and an entry the repository adds after the
// base switches one off. It uses the fixtures and the skips of
// TestESLintGitignore.
func TestESLintSeam(t *testing.T) {
	if !strings.HasSuffix(string(eslintSeed), seedExport) {
		t.Fatalf("the seed does not end with %q; the test cannot add an entry to it", seedExport)
	}
	example, eslint := eslintExample(t)
	const rule = "@typescript-eslint/no-floating-promises"

	for _, c := range []struct {
		name, seed string
		reported   bool
	}{
		{"seed", string(eslintSeed), true},
		{"rule switched off after the base", strings.Replace(string(eslintSeed), seedExport,
			"export default [...base, { files: ['**/*.ts'], rules: { '"+rule+"': 'off' } }]\n", 1), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := fixtureDir(t, example)
			writeFile(t, filepath.Join(dir, "tsconfig.json"), jsxTSConfig)
			file := writeFile(t, filepath.Join(dir, "a.ts"), floatingPromise)
			config := writeConfig(t, dir, c.seed)

			got := runESLintRules(t, eslint, dir, config)[key(file)]
			if slices.Contains(got, rule) != c.reported {
				t.Errorf("%s reported = %v, want %v (rule ids %q)", rule, !c.reported, c.reported, got)
			}
		})
	}
}
