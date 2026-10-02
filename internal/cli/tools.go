package cli

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/standard"
)

// lookPath is a test seam. Contributor laptops and CI runners differ in what
// they have installed, so a test calling exec.LookPath directly would assert
// on the machine rather than on this code.
var lookPath = exec.LookPath

// missingTool is one required binary that is not installed, together with
// the module that asked for it.
type missingTool struct {
	module   string
	name     string
	why      string
	optional bool
	install  string
}

// warnMissingTools reports on w every external binary the standard's modules
// require that is not on PATH.
//
// Warnings only, and never on stdout: a missing tool is not
// non-conformance. The repository's files are exactly what the standard
// says they should be, vibe audit will report it conformant, and sync's
// exit code must not disagree with audit about the same repository.
func warnMissingTools(w io.Writer, s *standard.Standard, mctx *module.Context) {
	for _, t := range missingTools(s, mctx) {
		// Dropped write errors: a warning that could not be printed must
		// not fail the command it was only advising.
		if t.optional {
			_, _ = fmt.Fprintf(w, "warning: %s not found on PATH (optional, for %s: %s; install: %s)\n", t.name, t.module, t.why, t.install)
			continue
		}
		_, _ = fmt.Fprintf(w, "warning: %s not found on PATH (required by %s: %s)\n", t.name, t.module, t.why)
	}
}

// requiredTool is one binary a module asked for, with that module's name.
type requiredTool struct {
	module string
	tool   module.Tool
}

// requiredTools walks the standard in module then declaration order,
// returning every binary its modules require. A binary two modules require
// is listed once, under the first to ask for it: one missing install is one
// problem, and output has to be deterministic across runs for the same
// reason resource ordering does. warnMissingTools and vibe doctor both read
// this list, so they cannot disagree about what the standard needs.
func requiredTools(s *standard.Standard, mctx *module.Context) []requiredTool {
	var tools []requiredTool
	listed := make(map[string]bool)

	selected := s.Defaults()
	if mctx != nil && mctx.Integrations != nil {
		selected = standard.Selection{Integrations: mctx.Integrations, Policies: mctx.Policies}
	}
	for _, mod := range s.ModulesFor(selected) {
		requirer, ok := mod.(module.ToolRequirer)
		if !ok {
			continue
		}
		for _, tool := range requirer.RequiredTools(mctx) {
			if listed[tool.Name] {
				continue
			}
			listed[tool.Name] = true
			tools = append(tools, requiredTool{module: mod.Name(), tool: tool})
		}
	}
	return tools
}

// missingTools reports each required binary that is not on PATH.
func missingTools(s *standard.Standard, mctx *module.Context) []missingTool {
	var missing []missingTool
	for _, rt := range requiredTools(s, mctx) {
		if _, err := lookPath(rt.tool.Name); err != nil {
			missing = append(missing, missingTool{module: rt.module, name: rt.tool.Name, why: rt.tool.Why,
				optional: rt.tool.Optional, install: rt.tool.Install})
		}
	}
	return missing
}
