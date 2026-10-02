package module

import (
	"bytes"
	"fmt"
)

// GraphUpdateTask is the task the Graphify integration adds to a
// repo-tooling Taskfile, appended after its last task
// (docs/specs/0035-graphify.md §2). The logic lives here, in Task's
// portable shell, rather than in lefthook.yml: lefthook on Windows does
// not keep shell quoting in a run: line intact. It always exits 0, because
// the Git hooks that run it cannot undo a commit or a checkout, and a
// graph must never block work; a skipped or failed update says so loudly.
const GraphUpdateTask = `
  # Rebuilds the Graphify knowledge graph in graphify-out/ from the code
  # (no LLM, no network). The post-commit and post-checkout Git hooks run
  # it; nothing in verify or CI does. See docs/specs/0035-graphify.md.
  graph:update:
    desc: Rebuild the Graphify graph in graphify-out/ (run by the post-commit hook)
    silent: true
    cmds:
      - |
        if ! command -v graphify >/dev/null 2>&1; then
          echo "graphify: not on PATH; graph not updated, graphify-out/ may be stale (install: uv tool install graphifyy)" >&2
          exit 0
        fi
        graphify update . || echo "graphify: update failed; graphify-out/ may be stale" >&2
`

// GraphContextLine is the hook:context line that says whether a graph
// exists. It reads nothing but the file's presence: freshness is the
// skill's and vibe doctor's to judge, and parsing graph.json in a session
// hook would be neither cheap nor portable.
const GraphContextLine = `        if test -f graphify-out/graph.json; then echo "Graphify: graph present (check freshness: built_at_commit vs HEAD)"; else echo "Graphify: no graph yet (graphify-out/graph.json absent)"; fi
`

// GraphHookJobs are the lefthook hooks that keep the graph current. They
// are separate hooks, never pre-commit or pre-push, so a missing or
// failing graphify cannot hold up a commit or a push.
const GraphHookJobs = `
post-commit:
  commands:
    graphify-update:
      run: task graph:update

post-checkout:
  commands:
    graphify-update:
      run: task graph:update
`

// contextAnchor is the line of every hook:context script the
// Graphify line follows.
var contextAnchor = []byte("        echo \"Task: {{.TASK_VERSION}}\"\n")

// AddGraphify returns a repo-tooling Taskfile and
// lefthook.yml with the Graphify integration's additions: the graph:update
// task, the post-commit and post-checkout jobs, and, when the agent-hook
// surface is generated (hooks), the hook:context line. Without graphify
// selected the module does not call it, so its files stay byte-identical.
func AddGraphify(taskfile, lefthook []byte, hooks bool) (newTaskfile, newLefthook []byte, err error) {
	if !bytes.HasSuffix(taskfile, []byte("\n")) || !bytes.HasSuffix(lefthook, []byte("\n")) {
		return nil, nil, fmt.Errorf("add graphify: Taskfile and lefthook.yml must end in a newline")
	}
	out := taskfile
	if hooks {
		if n := bytes.Count(out, contextAnchor); n != 1 {
			return nil, nil, fmt.Errorf("add graphify: hook:context anchor %q found %d times, want 1", bytes.TrimSpace(contextAnchor), n)
		}
		out = bytes.Replace(out, contextAnchor, append(append([]byte{}, contextAnchor...), GraphContextLine...), 1)
	}
	out = append(append([]byte{}, out...), GraphUpdateTask...)
	return out, append(append([]byte{}, lefthook...), GraphHookJobs...), nil
}
