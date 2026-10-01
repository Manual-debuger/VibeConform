// Package lineendings provides the opt-in line-ending policy: one managed
// section at the top of .gitattributes that pins every text file to LF.
// The rest of the file stays the project's, and a narrower rule a user
// writes below the section overrides it for the paths it names. See
// docs/specs/0029-text-policy.md.
package lineendings

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

const (
	// Path is the file the section lives in.
	Path = ".gitattributes"
	// SectionID names the section within it.
	SectionID = "line-endings"
)

// content is the section, between its markers. It is a constant rather
// than an embedded template, so a CRLF working copy of a template file
// cannot change the bytes a build ships.
const content = "# Managed by VibeConform: policy.line_endings in vibe.yaml.\n" +
	"* text=auto eol=lf\n"

type lineEndings struct{}

// New returns the line-endings-policy module.
func New() module.Module {
	return lineEndings{}
}

func (lineEndings) Name() string {
	return "line-endings-policy"
}

// Resolve returns the policy's section of .gitattributes, placed at the
// top: git applies the last matching line, so every user rule after it is
// an exception layered over the policy.
func (lineEndings) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{{
		Path:      Path,
		Ownership: resource.ManagedSection,
		SectionID: SectionID,
		Markers:   resource.HashComment,
		Placement: resource.Top,
		Content:   []byte(content),
	}}, nil
}

// CheckSection reports the user rules that defeat the policy. Below the
// section, a rule on every path that changes text, eol, or crlf disables
// the policy everywhere: a conflict. Above it, such a rule is overridden
// by the policy for the paths it names: a warning.
func (lineEndings) CheckSection(r resource.Resource, before, after []byte) (conflicts, warnings []string) {
	line := 0
	for _, l := range lines(before) {
		line++
		if pattern, attr, ok := contradiction(l); ok {
			warnings = append(warnings, fmt.Sprintf("line %d: %q comes before the policy's section, which overrides its %s for %s; move it below the section",
				line, strings.TrimSpace(l), attr, pattern))
		}
	}
	line += bytes.Count(r.Content, []byte("\n")) + 2
	for _, l := range lines(after) {
		line++
		pattern, attr, ok := contradiction(l)
		if ok && global(pattern) {
			conflicts = append(conflicts, fmt.Sprintf("line %d: %q overrides policy.line_endings (%s) for every path; narrow it to the paths that need it, or deselect the policy",
				line, strings.TrimSpace(l), attr))
		}
	}
	return conflicts, warnings
}

func lines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// contradiction parses one gitattributes line and reports the first
// attribute it sets that differs from the policy's text=auto eol=lf.
// Blank lines, comments, and macro definitions never contradict.
func contradiction(line string) (pattern, attr string, ok bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[attr]") {
		return "", "", false
	}
	var fields []string
	if strings.HasPrefix(line, `"`) {
		// A quoted pattern, gitattributes(5): C-style escapes.
		end := closingQuote(line)
		if end < 0 {
			return "", "", false
		}
		p, err := strconv.Unquote(line[:end+1])
		if err != nil {
			return "", "", false
		}
		pattern, fields = p, strings.Fields(line[end+1:])
	} else {
		all := strings.Fields(line)
		pattern, fields = all[0], all[1:]
	}
	for _, a := range fields {
		if contradicts(a) {
			return pattern, a, true
		}
	}
	return pattern, "", false
}

// closingQuote returns the index of the quote closing line's leading one.
func closingQuote(line string) int {
	for i := 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// contradicts reports whether one attribute assignment changes what the
// policy sets: text other than auto, eol other than lf, crlf in any form,
// or the binary macro.
func contradicts(a string) bool {
	name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-!"), "=")
	unset := strings.HasPrefix(a, "-") || strings.HasPrefix(a, "!")
	switch name {
	case "text":
		return unset || !hasValue || value != "auto"
	case "eol":
		return unset || !hasValue || value != "lf"
	case "crlf":
		return true
	case "binary":
		return !unset
	}
	return false
}

// global reports whether pattern matches every path.
func global(pattern string) bool {
	switch strings.TrimPrefix(pattern, "/") {
	case "*", "**":
		return true
	}
	return false
}
