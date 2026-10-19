// Package pins holds every toolchain version a generated CI file names, so a
// bump is one edit here rather than one per template, and the template
// helpers the CI and conformance modules render with.
// See docs/specs/0044-ci-pin-table.md.
package pins

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Table is every toolchain version a generated CI file names.
//
// Bump practice: when Go is bumped, bump the tool pins (GolangciLint,
// Goimports, Govulncheck, Actionlint) in the same change, to versions that
// build with that Go. Stale-pin detection is issue #75.
type Table struct {
	Go   string
	Task string
	// TaskSHA256 is the checksum of Task's task_linux_amd64.tar.gz release
	// archive at version Task. Change both together.
	TaskSHA256   string
	GolangciLint string
	// Goimports is a golang.org/x/tools version, the module goimports is
	// installed from.
	Goimports   string
	Govulncheck string
	Actionlint  string
	Node        string
	Python      string
	// SetupAction is the commit of this repository whose action.yml the
	// GitHub conformance workflow uses (spec 0054), pinned by SHA (C-02).
	// Bump it when action.yml changes in a way the workflow needs.
	SetupAction string
}

// Current is the table the generated CI uses. Modules read it when they
// resolve, never at init, so a test can swap it.
var Current = Table{
	Go:           "1.27.0",
	Task:         "v3.53.1",
	TaskSHA256:   "a54a408f6861ff921f6e87774180db31bacd8c1e7c944ca696db9fea49a82fc7",
	GolangciLint: "v2.13.2",
	Goimports:    "v0.51.0",
	Govulncheck:  "v1.8.0",
	Actionlint:   "v1.7.12",
	Node:         "22",
	Python:       "3.12",
	SetupAction:  "32514425a6693842f13177086897a2ca6e9beffc",
}

// TaskSetup is Task as go-task/setup-task takes it: the same release,
// without the leading v, which the action adds itself. Templates name it
// as [[.Pins.TaskSetup]].
func (t Table) TaskSetup() string {
	return strings.TrimPrefix(t.Task, "v")
}

// Parse parses a CI template. Its [[ ]] delimiters leave GitHub's ${{ }}
// expressions and GitLab's ${VAR} references untouched.
func Parse(name, src string) *template.Template {
	return template.Must(template.New(name).Delims("[[", "]]").Option("missingkey=error").Parse(src))
}

// RenderCurrent executes a template whose only data is the table, as .Pins.
func RenderCurrent(t *template.Template) ([]byte, error) {
	return Render(t, struct{ Pins Table }{Current})
}

// Render executes t with data.
func Render(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", t.Name(), err)
	}
	return buf.Bytes(), nil
}
