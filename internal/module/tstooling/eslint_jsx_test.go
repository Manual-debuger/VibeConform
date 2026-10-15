package tstooling

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestESLintJSX runs real ESLint with the template under test (spec 0048).
// It is the regression test for issue #91: a .tsx or .jsx file, which the
// standard formats as source, was linted by nothing. It uses the fixtures
// and the skips of TestESLintGitignore.
func TestESLintJSX(t *testing.T) {
	example, eslint := eslintExample(t)
	dir := fixtureDir(t, example)
	writeFile(t, filepath.Join(dir, "tsconfig.json"), jsxTSConfig)
	control := writeFile(t, filepath.Join(dir, "control.ts"), floatingPromise)
	tsx := writeFile(t, filepath.Join(dir, "app.tsx"), floatingPromiseTSX)
	jsx := writeFile(t, filepath.Join(dir, "view.jsx"), unusedJSX)
	config := writeConfig(t, dir, string(eslintSeed))

	rules := runESLintRules(t, eslint, dir, config)
	for _, c := range []struct{ file, rule string }{
		// The control shows the fixture's typed setup works, so a missing
		// report on app.tsx is about the file set, not the fixture.
		{control, "@typescript-eslint/no-floating-promises"},
		{tsx, "@typescript-eslint/no-floating-promises"},
		{jsx, "no-unused-vars"},
	} {
		got := rules[key(c.file)]
		if !slices.Contains(got, c.rule) {
			t.Errorf("%s: want a %s report, got rule ids %q", filepath.Base(c.file), c.rule, got)
		}
		if slices.Contains(got, "") {
			t.Errorf("%s: a problem without a rule id, such as a parse error (rule ids %q)", filepath.Base(c.file), got)
		}
	}
}

// jsxTSConfig puts the fixture's .ts and .tsx files in a project, as an
// adopter's tsconfig.json does, with JSX set there rather than in the base.
const jsxTSConfig = `{
  "compilerOptions": { "strict": true, "jsx": "preserve", "noEmit": true },
  "include": ["*.ts", "*.tsx"]
}
`

const floatingPromise = `async function load(): Promise<void> {}
export function run(): void {
  load()
}
`

const floatingPromiseTSX = `async function load(): Promise<void> {}
export function App() {
  load()
  return <div />
}
`

const unusedJSX = "const unused = <div />\n"
