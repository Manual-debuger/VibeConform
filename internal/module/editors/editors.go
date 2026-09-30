// Package editors holds what the editor integrations share: the Task
// entry points each one offers from its task runner. Every command goes
// through Task — the canonical verification interface — and never through
// vibe. See docs/specs/0026-optional-integrations.md.
package editors

// Tasks are the Taskfile tasks an editor integration exposes, in the
// order it lists them.
var Tasks = []string{"fmt", "lint", "test", "verify"}

// Label is the label an editor task running "task <name>" is owned by.
// It names the command it runs, so a user reading the editor's task list
// knows what each entry does, and a user task that happens to share it is
// a real collision.
func Label(task string) string {
	return "task " + task
}

// OwnedPaths lists, per editor integration, the files it owns elements
// of. Core modules that run a formatter over the repository leave these
// out of its reach, since no fixed bytes are stable under every
// configuration of it (spec 0026 §10). A test checks the map against the
// paths the integrations resolve.
var OwnedPaths = map[string][]string{
	"vscode": {".vscode/tasks.json", ".vscode/extensions.json"},
	"zed":    {".zed/tasks.json"},
}

// OwnedPathsOf returns the owned paths of the selected integrations, in
// the selection's order. Integrations that own no editor file contribute
// nothing, so a selection without an editor returns none.
func OwnedPathsOf(integrations []string) []string {
	var paths []string
	for _, name := range integrations {
		paths = append(paths, OwnedPaths[name]...)
	}
	return paths
}
