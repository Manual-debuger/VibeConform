package module

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
)

// hookBlockStart is the first line of the agent-hook block every
// repo-tooling Taskfile template carries: the comment introducing
// hook:guard.
var hookBlockStart = regexp.MustCompile(`^  # Called by Claude Code\b`)

// taskKey matches a task's key line at the tasks: map's indentation.
var taskKey = regexp.MustCompile(`^  ([^\s#]\S*):\s*$`)

// indentTwo matches a line that belongs to the tasks: map itself rather
// than to a task's body: a key or a comment at two spaces.
var indentTwo = regexp.MustCompile(`^  \S`)

// StripAgentHooks removes the agent-hook block — the hook:* tasks and the
// comments above them — from a repo-tooling Taskfile, for a repository
// that selects no agent running them (docs/specs/0026-optional-integrations.md).
//
// The block runs from the "Called by Claude Code" comment to the first
// tasks-level line after the last hook:* task. It is an error for the block
// to be missing, to appear twice, or to contain a task that is not
// hook:*, so a template edit that moves it fails the module's tests
// instead of silently keeping, or dropping, the wrong tasks.
func StripAgentHooks(taskfile []byte) ([]byte, error) {
	lines := bytes.SplitAfter(taskfile, []byte("\n"))

	start := -1
	for i, l := range lines {
		if hookBlockStart.Match(l) {
			if start >= 0 {
				return nil, errors.New("strip agent hooks: two agent-hook blocks")
			}
			start = i
		}
	}
	if start < 0 {
		return nil, errors.New("strip agent hooks: no agent-hook block")
	}

	lastHook := -1
	for i := start; i < len(lines); i++ {
		if m := taskKey.FindSubmatch(trimEOL(lines[i])); m != nil && bytes.HasPrefix(m[1], []byte("hook:")) {
			lastHook = i
		}
	}
	if lastHook < 0 {
		return nil, errors.New("strip agent hooks: agent-hook block defines no hook:* task")
	}
	// Only hook tasks may sit between the block's first line and its last
	// hook task: anything else would be dropped with them.
	for i := start; i < lastHook; i++ {
		if m := taskKey.FindSubmatch(trimEOL(lines[i])); m != nil && !bytes.HasPrefix(m[1], []byte("hook:")) {
			return nil, fmt.Errorf("strip agent hooks: task %s inside the agent-hook block", m[1])
		}
	}
	// The last hook task's body ends at the next tasks-level line.
	end := len(lines)
	for i := lastHook + 1; i < len(lines); i++ {
		if indentTwo.Match(trimEOL(lines[i])) {
			end = i
			break
		}
	}

	out := bytes.Join(lines[:start], nil)
	out = append(out, bytes.Join(lines[end:], nil)...)
	return out, nil
}

func trimEOL(l []byte) []byte {
	return bytes.TrimRight(l, "\r\n")
}
