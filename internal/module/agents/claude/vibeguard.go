package claude

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// VibeGuardPolicy is .claude/hooks/policy.json with policy.agent_guard
// vibe-guard: version 2 of the policy (spec 0053), enforcing, with no
// rules and no exceptions of the repository's own.
const VibeGuardPolicy = "{\n" +
	"  \"version\": 2,\n" +
	"  \"mode\": \"enforce\",\n" +
	"  \"rules\": [],\n" +
	"  \"exceptions\": []\n" +
	"}\n"

// The lines of templates/settings.json that policy.agent_guard vibe-guard
// replaces: the PreToolUse matcher and command, the Stop command, and the
// other hooks' Task calls.
const (
	runtimeMatcher = `"matcher": "Bash|PowerShell|Write|Edit|MultiEdit",`
	runtimeGuard   = `"command": "` + GuardCommand + `"`
	runtimeStop    = `"command": "` + StopCommand + `",`
)

// rootedCommands are the template's other hook commands, which call Task.
// With vibe-guard each runs from the project root (spec 0058, #133).
var rootedCommands = []string{ContextCommand, FormatCommand, CheckCommand}

// vibeGuardSettings returns templates/settings.json with the reference
// hook commands of spec 0053 (internal/guard's ReferencePreToolUse,
// ReferencePreToolUseMatcher and ReferenceStop) in place of the runtime
// guard's, and with guard.ProjectRootPrefix before every other hook
// command (spec 0058). Everything else is byte-for-byte the template.
func vibeGuardSettings(vibeVersion string) ([]byte, error) {
	out := settings
	replacements := []struct{ old, new string }{
		{runtimeMatcher, `"matcher": ` + jsonString(guard.ReferencePreToolUseMatcher) + `,`},
		{runtimeGuard, `"command": ` + jsonString(guard.ReferencePreToolUse(guard.InstallVersion(vibeVersion)))},
		{runtimeStop, `"command": ` + jsonString(guard.ReferenceStop) + `,`},
	}
	for _, c := range rootedCommands {
		replacements = append(replacements, struct{ old, new string }{
			`"command": "` + c + `",`, `"command": ` + jsonString(guard.ProjectRootPrefix+c) + `,`})
	}
	for _, r := range replacements {
		if n := bytes.Count(out, []byte(r.old)); n != 1 {
			return nil, fmt.Errorf("render vibe-guard settings: %s found %d times in the template, want 1", r.old, n)
		}
		out = bytes.Replace(out, []byte(r.old), []byte(r.new), 1)
	}
	return out, nil
}

// jsonString encodes s as a JSON string without HTML escaping, so that
// the shell's > and & read in the file as they are.
func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return string(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
}
