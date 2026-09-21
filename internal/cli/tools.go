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
	module string
	name   string
	why    string
}

// warnMissingTools reports on w every external binary the standard's modules
// require that is not on PATH.
//
// Warnings only, and never on stdout: a missing tool is not
// non-conformance. The repository's files are exactly what the standard
// says they should be, vibe audit will report it conformant, and sync's
// exit code must not disagree with audit about the same repository.
func warnMissingTools(w io.Writer, s *standard.Standard) {
	for _, t := range missingTools(s) {
		// Dropped write errors: a warning that could not be printed must
		// not fail the command it was only advising.
		_, _ = fmt.Fprintf(w, "warning: %s not found on PATH (required by %s: %s)\n", t.name, t.module, t.why)
	}
}

// missingTools walks the standard in module then declaration order,
// reporting each required binary that is not on PATH. A binary two modules
// require is reported once, under the first to ask for it: one missing
// install is one problem, and output has to be deterministic across runs
// for the same reason resource ordering does.
func missingTools(s *standard.Standard) []missingTool {
	var missing []missingTool
	checked := make(map[string]bool)

	for _, mod := range s.Modules {
		requirer, ok := mod.(module.ToolRequirer)
		if !ok {
			continue
		}
		for _, tool := range requirer.RequiredTools() {
			if checked[tool.Name] {
				continue
			}
			checked[tool.Name] = true

			if _, err := lookPath(tool.Name); err != nil {
				missing = append(missing, missingTool{module: mod.Name(), name: tool.Name, why: tool.Why})
			}
		}
	}

	return missing
}
