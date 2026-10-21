package guard

import (
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// The reference hook commands (spec 0053, "The reference hook
// commands"), as .claude/settings.json runs them.

// ReferencePreToolUseMatcher is the reference PreToolUse matcher.
const ReferencePreToolUseMatcher = "Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit"

// releases is where vibe-guard's release archives are published.
const releases = "https://github.com/Manual-debuger/VibeConform/releases"

// ReferencePreToolUse is the reference PreToolUse command. When
// vibe-guard is not on PATH it says how to install version, with go
// install or from the release page, and refuses (exit 2), so a missing
// guard fails closed. command -v is a shell builtin, so the check works
// with only the operating system's PATH. version is what InstallVersion
// returns; for latest the link is to the releases page.
func ReferencePreToolUse(version string) string {
	page := releases + "/tag/" + version
	if version == "latest" {
		page = releases
	}
	return `command -v vibe-guard >/dev/null 2>&1 || { echo "vibe-guard is not installed; install it: ` +
		`go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@` + version +
		`, or download it from ` + page + `" >&2; exit 2; }; vibe-guard hook`
}

// ReferenceStop is the reference Stop command: the relock runs first,
// whatever verify-fast does, then the generated Stop gate (claude-config's
// StopCommand, spec 0057), which skips verify-fast when the tree matches
// the green stamp.
const ReferenceStop = "vibe-guard relock 2>/dev/null; " +
	"task -x hook:stamp:check >/dev/null 2>&1 </dev/null || " +
	"{ task verify:fast 1>&2 </dev/null && task -x hook:stamp:save >/dev/null 2>&1 </dev/null; } || " +
	"task -x hook:done"

// InstallVersion is the version the PreToolUse command's install hint
// names for a vibe of version vibeVersion (spec 0053, "The reference hook
// commands"; spec 0054, "The install hint"): the running vibe's own
// version when it is a release, so that the guard installed matches the
// vibe that wrote the settings. Any other version names latest: "dev"
// from an unstamped build, one with build metadata ("+dirty"), and a
// pseudo-version, which go build stamps from the commit and so would
// change the settings with every commit of a repository that builds vibe
// from source.
func InstallVersion(vibeVersion string) string {
	if semver.IsValid(vibeVersion) && semver.Build(vibeVersion) == "" && !module.IsPseudoVersion(vibeVersion) {
		return vibeVersion
	}
	return "latest"
}
