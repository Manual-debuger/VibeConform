package claude

import (
	"bytes"
	"encoding/json"
	"fmt"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// VibeGuardVersion is the version the PreToolUse command's install hint
// names for a vibe of version vibeVersion (spec 0053, "The reference hook
// commands"): the running vibe's own version when it is a release, so
// that the guard installed matches the vibe that wrote the settings.
// Any other version names latest: "dev" from an unstamped build, one
// with build metadata ("+dirty"), and a pseudo-version, which go build
// stamps from the commit and so would change the settings with every
// commit of a repository that builds vibe from source.
func VibeGuardVersion(vibeVersion string) string {
	if semver.IsValid(vibeVersion) && semver.Build(vibeVersion) == "" && !module.IsPseudoVersion(vibeVersion) {
		return vibeVersion
	}
	return "latest"
}

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
// replaces: the PreToolUse matcher and command, and the Stop command.
const (
	runtimeMatcher = `"matcher": "Bash|PowerShell|Write|Edit|MultiEdit",`
	runtimeGuard   = `"command": "` + GuardCommand + `"`
	runtimeStop    = `"command": "` + StopCommand + `",`
)

// vibeGuardSettings returns templates/settings.json with the reference
// hook commands of spec 0053 (internal/guard's ReferencePreToolUse,
// ReferencePreToolUseMatcher and ReferenceStop) in place of the runtime
// guard's. Everything else is byte-for-byte the template.
func vibeGuardSettings(vibeVersion string) ([]byte, error) {
	out := settings
	for _, r := range []struct{ old, new string }{
		{runtimeMatcher, `"matcher": ` + jsonString(guard.ReferencePreToolUseMatcher) + `,`},
		{runtimeGuard, `"command": ` + jsonString(guard.ReferencePreToolUse(VibeGuardVersion(vibeVersion)))},
		{runtimeStop, `"command": ` + jsonString(guard.ReferenceStop) + `,`},
	} {
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
