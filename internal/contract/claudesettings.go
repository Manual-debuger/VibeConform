package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
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
	// Commands holds one command hook's command string each.
	Commands []string
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
	for _, c := range h.Commands {
		if cmds, err := ParseCommands(c); err == nil {
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
	data, err := e.ReadFile(claudeSettingsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, Fail(Finding{
			File:    claudeSettingsFile,
			Message: "is not tracked, so Claude Code runs no hooks of this repository",
			Fix:     "create " + claudeSettingsFile + " and " + missingFix,
		}), false
	case err != nil:
		return nil, Undecided(Finding{File: claudeSettingsFile, Message: fmt.Sprintf("cannot be read: %v", err), Fix: "make the file readable"}), false
	}
	s, err = parseClaudeSettings(data)
	if err != nil {
		return nil, Fail(ParseFailure(claudeSettingsFile, err)), false
	}
	return s, Pass(), true
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
			h.Commands = append(h.Commands, c)
		}
	}
	return h, nil
}

// commandEntries returns an event's entries that have a command hook.
func (s *claudeSettings) commandEntries(event string) []claudeHookEntry {
	var out []claudeHookEntry
	for _, h := range s.Events[event] {
		if len(h.Commands) > 0 {
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
