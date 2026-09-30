package tsrepotooling

import (
	"bytes"
	"fmt"
	"strings"
)

// Keeping Prettier off a selected editor's owned files rewrites exact
// template text. A template where that text is missing or repeated is
// refused rather than half edited, as module.StripAgentHooks does for the
// hook block. With no paths the templates come back unchanged, so a
// repository without an editor is byte-identical to the embedded
// templates. See docs/specs/0026-optional-integrations.md §10.
const (
	prettierGlob = `"**/*.{js,jsx,ts,tsx,json,css,md}"`
	// hookFormatDiff and hookFormatOthers end the two git commands that
	// list hook:format's changed files.
	hookFormatDiff   = `'*.md' 2>/dev/null`
	hookFormatOthers = `--others --exclude-standard -- '*.js' '*.jsx' '*.ts' '*.tsx' '*.json' '*.css' '*.md'` + "\n"
	lefthookGlob     = `      glob: "*.{js,jsx,ts,tsx,json,css,md}"` + "\n"
)

// excludeFromPrettierTaskfile adds a negated pattern per path to fmt and
// fmt:check and, when the template still has its hook block, an exclude
// pathspec per path to both of hook:format's git commands.
func excludeFromPrettierTaskfile(taskfile []byte, paths []string, hooks bool) ([]byte, error) {
	if len(paths) == 0 {
		return taskfile, nil
	}
	var negated, pathspecs strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&negated, ` "!%s"`, p)
		fmt.Fprintf(&pathspecs, ` ':(exclude)%s'`, p)
	}

	edits := [][2]string{
		{"prettier --write --cache " + prettierGlob, "prettier --write --cache " + prettierGlob + negated.String()},
		{"prettier --check --cache " + prettierGlob, "prettier --check --cache " + prettierGlob + negated.String()},
	}
	if hooks {
		edits = append(edits,
			[2]string{hookFormatDiff, `'*.md'` + pathspecs.String() + ` 2>/dev/null`},
			[2]string{hookFormatOthers, strings.TrimSuffix(hookFormatOthers, "\n") + pathspecs.String() + "\n"},
		)
	}
	for _, e := range edits {
		var err error
		if taskfile, err = replaceOnce(taskfile, "Taskfile.yml", e[0], e[1]); err != nil {
			return nil, err
		}
	}
	return taskfile, nil
}

// excludeFromPrettierLefthook adds an exclude list to the prettier
// pre-commit command.
func excludeFromPrettierLefthook(lefthook []byte, paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return lefthook, nil
	}
	var list strings.Builder
	list.WriteString(lefthookGlob + "      exclude:\n")
	for _, p := range paths {
		fmt.Fprintf(&list, "        - %q\n", p)
	}
	return replaceOnce(lefthook, "lefthook.yml", lefthookGlob, list.String())
}

// replaceOnce replaces old with replacement, which must occur exactly
// once in data.
func replaceOnce(data []byte, file, old, replacement string) ([]byte, error) {
	if n := bytes.Count(data, []byte(old)); n != 1 {
		return nil, fmt.Errorf("exclude owned editor files from prettier: %s has %d copies of %q, want 1", file, n, old)
	}
	return bytes.Replace(data, []byte(old), []byte(replacement), 1), nil
}
