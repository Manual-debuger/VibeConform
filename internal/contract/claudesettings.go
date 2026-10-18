package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Claude Code settings as the A rules see them (spec 0052, "Recognition
// rules", Claude Code settings): the tracked .claude/settings.json,
// hooks.<Event>[] with a matcher and hooks[] of type command.
// .claude/settings.local.json is personal and never read (spec 0051).

const claudeSettingsFile = ".claude/settings.json"

// The Claude Code hook events the A rules read.
const (
	eventPreToolUse   = "PreToolUse"
	eventPostToolUse  = "PostToolUse"
	eventStop         = "Stop"
	eventSessionStart = "SessionStart"
)

// claudeSettings is the hooks of .claude/settings.json by event.
type claudeSettings struct {
	Events map[string][]claudeHookEntry
}

// claudeHookEntry is one hooks.<Event>[] entry.
type claudeHookEntry struct {
	// Matcher is the entry's matcher as written; "" when it has none.
	Matcher string
	// Hooks holds the entry's command hooks, in order.
	Hooks []commandHook
}

// commandHook is one command hook of an entry.
type commandHook struct {
	// Command is the hook's command string.
	Command string
	// Timeout is the hook's own timeout; zero when it sets none.
	Timeout time.Duration
}

// settingsHook is one command hook as a probe runs it: its command and
// timeout, and the entry whose matcher decides which tools reach it.
type settingsHook struct {
	entry   claudeHookEntry
	command string
	limit   time.Duration
}

// commandHooks returns every command hook of an event's entries, in
// order, each with its entry.
func (s *claudeSettings) commandHooks(event string) []settingsHook {
	var out []settingsHook
	for _, entry := range s.commandEntries(event) {
		for _, h := range entry.Hooks {
			out = append(out, settingsHook{entry: entry, command: h.Command, limit: h.Timeout})
		}
	}
	return out
}

// matches reports whether the entry's matcher matches a tool. A matcher
// that is missing, empty or * matches every tool. Otherwise the whole
// tool name must match it as a regular expression, so Write|Edit does
// not cover MultiEdit. A matcher that is not a valid regular expression
// matches only the tool it names exactly.
func (h claudeHookEntry) matches(tool string) bool {
	if h.Matcher == "" || h.Matcher == "*" {
		return true
	}
	re, err := regexp.Compile(`^(?:` + h.Matcher + `)$`)
	if err != nil {
		return h.Matcher == tool
	}
	return re.MatchString(tool)
}

// calls returns every simple command of the entry's command hooks. A
// command that does not parse as shell calls nothing.
func (h claudeHookEntry) calls() []Command {
	var out []Command
	for _, c := range h.Hooks {
		if cmds, err := ParseCommands(c.Command); err == nil {
			out = append(out, cmds...)
		}
	}
	return out
}

// readClaudeSettings reads .claude/settings.json. When it cannot, ok is
// false and result is what the rule decides: fail when the file is not
// tracked (with missingFix) or does not parse, cannot decide when it
// cannot be read.
func (e *Env) readClaudeSettings(missingFix string) (s *claudeSettings, result PartResult, ok bool) {
	data, res, ok := readDocument(e, claudeSettingsFile, Finding{
		File:    claudeSettingsFile,
		Message: "is not tracked, so Claude Code runs no hooks of this repository",
		Fix:     "create " + claudeSettingsFile + " and " + missingFix,
	})
	if !ok {
		return nil, res, false
	}
	if e.claudeSettings == nil {
		s, err := parseClaudeSettings(data)
		e.claudeSettings = &parsedClaudeSettings{s, err}
	}
	if err := e.claudeSettings.err; err != nil {
		return nil, Fail(ParseFailure(claudeSettingsFile, err)), false
	}
	return e.claudeSettings.settings, Pass(), true
}

// parsedClaudeSettings is .claude/settings.json parsed, once per audit.
type parsedClaudeSettings struct {
	settings *claudeSettings
	err      error
}

func parseClaudeSettings(data []byte) (*claudeSettings, error) {
	top, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	s := &claudeSettings{Events: map[string][]claudeHookEntry{}}
	raw, ok := top["hooks"]
	if !ok {
		return s, nil
	}
	var events map[string]json.RawMessage
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, errors.New("hooks must be an object")
	}
	for _, event := range slices.Sorted(maps.Keys(events)) {
		var entries []json.RawMessage
		if err := json.Unmarshal(events[event], &entries); err != nil {
			return nil, fmt.Errorf("hooks.%s must be a list", event)
		}
		for i, rawEntry := range entries {
			at := fmt.Sprintf("hooks.%s[%d]", event, i)
			entry, err := parseHookEntry(rawEntry, at)
			if err != nil {
				return nil, err
			}
			s.Events[event] = append(s.Events[event], entry)
		}
	}
	return s, nil
}

func parseHookEntry(raw json.RawMessage, at string) (claudeHookEntry, error) {
	var entry struct {
		Matcher *json.RawMessage  `json:"matcher"`
		Hooks   []json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil || string(raw) == "null" {
		return claudeHookEntry{}, fmt.Errorf("%s must be an object with a hooks list", at)
	}
	var h claudeHookEntry
	if entry.Matcher != nil && string(*entry.Matcher) != "null" {
		if err := json.Unmarshal(*entry.Matcher, &h.Matcher); err != nil {
			return claudeHookEntry{}, fmt.Errorf("%s.matcher must be a string", at)
		}
	}
	for j, rawHook := range entry.Hooks {
		var hook map[string]any
		if err := json.Unmarshal(rawHook, &hook); err != nil || hook == nil {
			return claudeHookEntry{}, fmt.Errorf("%s.hooks[%d] must be an object", at, j)
		}
		if hook["type"] != "command" {
			continue
		}
		if c, ok := hook["command"].(string); ok && strings.TrimSpace(c) != "" {
			var limit time.Duration
			if secs, ok := hook["timeout"].(float64); ok && secs > 0 {
				limit = time.Duration(secs * float64(time.Second))
			}
			h.Hooks = append(h.Hooks, commandHook{Command: c, Timeout: limit})
		}
	}
	return h, nil
}

// commandEntries returns an event's entries that have a command hook.
func (s *claudeSettings) commandEntries(event string) []claudeHookEntry {
	var out []claudeHookEntry
	for _, h := range s.Events[event] {
		if len(h.Hooks) > 0 {
			out = append(out, h)
		}
	}
	return out
}

// matching returns an event's entries with a command hook that match
// every one of tools.
func (s *claudeSettings) matching(event string, tools ...string) []claudeHookEntry {
	var out []claudeHookEntry
	for _, h := range s.commandEntries(event) {
		all := true
		for _, t := range tools {
			all = all && h.matches(t)
		}
		if all {
			out = append(out, h)
		}
	}
	return out
}
