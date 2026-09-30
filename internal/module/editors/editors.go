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
