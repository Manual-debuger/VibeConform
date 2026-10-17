package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "claude-config" {
		t.Errorf("Name() = %q, want %q", got, "claude-config")
	}
}

func resolve(t *testing.T) []resource.Resource {
	t.Helper()
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return resources
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources := resolve(t)

	want := []string{
		".claude/settings.json",
		".claude/hooks/policy.json",
		ClaudeMDPath,
		IgnorePath,
	}
	if len(resources) != len(want) {
		t.Fatalf("Resolve returned %d resources, want %d", len(resources), len(want))
	}

	for i, r := range resources {
		if r.Path != want[i] {
			t.Errorf("resource %d path = %q, want %q", i, r.Path, want[i])
		}
		if r.Path == IgnorePath {
			// The .gitignore section (spec 0039): a managed section, last.
			if r.Ownership != resource.ManagedSection || r.SectionID != IgnoreSectionID || r.Placement != resource.Bottom ||
				string(r.Content) != "# Managed by VibeConform: integrations.agents claude in vibe.yaml.\n.claude/settings.local.json\n" {
				t.Errorf(".gitignore section = %+v", r)
			}
			continue
		}
		if r.Path == ClaudeMDPath {
			// The import of AGENTS.md, with or without a workflow (spec 0042).
			if r.Ownership != resource.ManagedSection || r.SectionID != ImportSectionID || r.Placement != resource.Top {
				t.Errorf("CLAUDE.md section = %+v", r)
			}
			continue
		}
		if r.Ownership != resource.Generated {
			t.Errorf("%s ownership = %v, want Generated", r.Path, r.Ownership)
		}
		if len(r.Content) == 0 {
			t.Errorf("%s has empty content", r.Path)
		}
		if strings.Contains(r.Path, "\\") {
			t.Errorf("path %q contains a backslash; paths must be slash-separated", r.Path)
		}
	}
}

// TestNoResourceDependsOnModeBit pins what spec 0021 removed: the bash
// hooks needed 0755, and a chmod -x silently disabled them. Every guard is
// now run through an interpreter, so nothing here may need a mode bit.
func TestNoResourceDependsOnModeBit(t *testing.T) {
	for _, r := range resolve(t) {
		if r.Mode != 0 {
			t.Errorf("%s sets mode %v; agent config must work at the default mode", r.Path, r.Mode)
		}
	}
}

func TestResolveDeterministic(t *testing.T) {
	first, second := resolve(t), resolve(t)
	for i := range first {
		if first[i].Path != second[i].Path || !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("resource %d differs across calls", i)
		}
	}
}

// hookHandler is one command a hooks config runs for one event.
type hookHandler struct {
	event       string
	matcher     *string
	command     string
	timeout     int
	async       bool
	asyncRewake bool
}

// hookHandlers returns every handler a Claude Code hooks config declares,
// decoding the documented nested shape strictly for every event.
func hookHandlers(t *testing.T, name string, data []byte) []hookHandler {
	t.Helper()
	var cfg struct {
		Hooks map[string][]struct {
			Matcher *string `json:"matcher"`
			Hooks   []struct {
				Type        string `json:"type"`
				Command     string `json:"command"`
				Timeout     int    `json:"timeout"`
				Async       bool   `json:"async"`
				AsyncRewake bool   `json:"asyncRewake"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("%s is not in the documented hooks shape: %v", name, err)
	}

	var handlers []hookHandler
	for event, entries := range cfg.Hooks {
		for _, e := range entries {
			if len(e.Hooks) == 0 {
				t.Errorf("%s has a %s entry with no hooks array", name, event)
			}
			for _, h := range e.Hooks {
				if h.Type != "command" {
					t.Errorf("%s %s hook type = %q, want command", name, event, h.Type)
				}
				handlers = append(handlers, hookHandler{event, e.Matcher, h.Command, h.Timeout, h.Async, h.AsyncRewake})
			}
		}
	}
	return handlers
}

// hookCommands returns the matchers and commands of one event's handlers.
func hookCommands(t *testing.T, name string, data []byte, event string) (matchers, commands []string) {
	t.Helper()
	for _, h := range hookHandlers(t, name, data) {
		if h.event != event {
			continue
		}
		if h.matcher == nil {
			t.Errorf("%s has a %s entry with no matcher", name, event)
		} else {
			matchers = append(matchers, *h.matcher)
		}
		commands = append(commands, h.command)
	}
	if len(commands) == 0 {
		t.Fatalf("%s has no %s entries", name, event)
	}
	return matchers, commands
}

// hookCommandsByName are the only commands the settings may run: one
// fixed command per task, the same in every standard.
var hookCommandsByName = []string{GuardCommand, ContextCommand, FormatCommand, CheckCommand, DoneCommand}

// TestAgentConfigsCallTaskWithExitCode is the -x regression test. Without
// -x, Task exits 201 when a hook exits 2, and Claude Code treats 201 as a
// non-blocking error: every guardrail would silently stop guarding, and
// every failed check would silently pass (spec 0021, and spec 0023 for the
// other events).
func TestAgentConfigsCallTaskWithExitCode(t *testing.T) {
	for _, c := range hookCommandsByName {
		if !strings.HasPrefix(c, "task -x hook:") || strings.Count(c, " ") != 2 {
			t.Errorf("hook command %q is not of the form \"task -x hook:<name>\"", c)
		}
	}
	for _, h := range hookHandlers(t, ".claude/settings.json", settings) {
		if h.event == "Stop" {
			continue // TestStopHookCallsVerifyFast
		}
		if !slices.Contains(hookCommandsByName, h.command) {
			t.Errorf(".claude/settings.json %s runs %q, want one of %q", h.event, h.command, hookCommandsByName)
		}
	}
}

// redirection matches a shell redirection word such as 1>&2 or </dev/null.
var redirection = regexp.MustCompile(`^[0-9]*(<|>|>>|>&|<&)\S*$`)

// TestStopHookCallsVerifyFast pins contract rule A-05's static part (spec
// 0051): the Stop hook calls verify-fast, here `task verify:fast`, as a
// simple command whose words, without redirections, are exactly those (spec
// 0052). Only when it fails does hook:done run, through Task's -x like
// every other hook task, to decide between blocking the stop (exit 2) and
// letting a second stop through (spec 0023).
func TestStopHookCallsVerifyFast(t *testing.T) {
	var stops int
	for _, h := range hookHandlers(t, ".claude/settings.json", settings) {
		if h.event != "Stop" {
			continue
		}
		stops++
		first, rest, ok := strings.Cut(h.command, " || ")
		if !ok || rest != DoneCommand {
			t.Errorf("Stop runs %q, want `task verify:fast ... || %s`", h.command, DoneCommand)
		}
		words := slices.DeleteFunc(strings.Fields(first), redirection.MatchString)
		if !slices.Equal(words, []string{"task", "verify:fast"}) {
			t.Errorf("Stop's first command has the words %q without redirections, want [task verify:fast]", words)
		}
	}
	if stops != 1 {
		t.Errorf(".claude/settings.json has %d Stop hooks, want 1", stops)
	}
}

// wantHandler is one row of spec 0023's section 1 table.
type wantHandler struct {
	event, matcher, command string
	timeout                 int
	async, asyncRewake      bool
}

// TestAgentHookEvents pins spec 0023's section 1 table: every event, its
// matcher, command, timeout, and async option. The background check uses
// asyncRewake, because only it wakes the model as soon as the hook exits 2;
// an "async" hook's result waits for the next turn.
func TestAgentHookEvents(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		want []wantHandler
	}{
		".claude/settings.json": {settings, []wantHandler{
			{"SessionStart", "startup|resume|clear", ContextCommand, 30, false, false},
			{"PreToolUse", "Bash|PowerShell|Write|Edit|MultiEdit", GuardCommand, 0, false, false},
			{"PostToolUse", "Write|Edit|MultiEdit|NotebookEdit", FormatCommand, 60, false, false},
			{"PostToolUse", "Write|Edit|MultiEdit|NotebookEdit", CheckCommand, 300, false, true},
			{"Stop", "", StopCommand, 600, false, false},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			var got []wantHandler
			for _, h := range hookHandlers(t, name, tc.data) {
				matcher := ""
				if h.matcher != nil {
					matcher = *h.matcher
				}
				got = append(got, wantHandler{h.event, matcher, h.command, h.timeout, h.async, h.asyncRewake})
			}
			key := func(w wantHandler) string { return w.event + "\x00" + w.command }
			slices.SortFunc(got, func(a, b wantHandler) int { return strings.Compare(key(a), key(b)) })
			slices.SortFunc(tc.want, func(a, b wantHandler) int { return strings.Compare(key(a), key(b)) })
			if !slices.Equal(got, tc.want) {
				t.Errorf("handlers =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

// TestClaudeMatcherCoversCommandsAndEdits checks that Claude Code routes
// every tool kind the policy has rules for to the guard, and every tool
// contract rule A-01 names (spec 0051), under the reading the contract
// audit uses: a matcher matches a tool when it matches the whole tool name
// as a regular expression (spec 0052). Whether Claude Code anchors its
// matchers is not verified; read unanchored, "Edit" alone would also cover
// MultiEdit, and naming it does no harm.
func TestClaudeMatcherCoversCommandsAndEdits(t *testing.T) {
	matchers, _ := hookCommands(t, ".claude/settings.json", settings, "PreToolUse")
	tools := []string{"Bash", "PowerShell", "Write", "Edit", "MultiEdit"}
	for _, ts := range policy.Tools {
		tools = append(tools, ts...)
	}
	slices.Sort(tools)
	for _, tool := range slices.Compact(tools) {
		if !slices.ContainsFunc(matchers, func(m string) bool {
			return regexp.MustCompile("^(?:" + m + ")$").MatchString(tool)
		}) {
			t.Errorf("no .claude/settings.json PreToolUse matcher matches the whole tool name %s", tool)
		}
	}
}

func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..", "..")

	// This repository selects policy.agent_guard vibe-guard (spec 0053), so
	// its live files are that rendering of the templates.
	// TestAgentGuardRuntimeUnchanged pins the default's.
	live := resolveContents(t, agentGuard(module.AgentGuardVibeGuard))

	for _, tc := range []struct {
		live     string
		embedded []byte
	}{
		{filepath.Join(".claude", "settings.json"), live[SettingsPath]},
		{filepath.Join(".claude", "hooks", "policy.json"), live[".claude/hooks/policy.json"]},
	} {
		t.Run(tc.live, func(t *testing.T) {
			live, err := os.ReadFile(filepath.Join(repoRoot, tc.live))
			if err != nil {
				t.Fatalf("reading live file: %v", err)
			}
			if !bytes.Equal(live, tc.embedded) {
				t.Errorf("embedded template has drifted from %s — "+
					"read the difference before resolving it; these files are guardrails", tc.live)
			}
		})
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	if bytes.Contains(settings, []byte("\r")) {
		t.Error("settings.json contains CR bytes; the working copy it was embedded from is CRLF")
	}
}
