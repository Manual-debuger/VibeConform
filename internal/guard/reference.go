package guard

// The reference hook commands (spec 0053, "The reference hook
// commands"), as .claude/settings.json runs them.

// ReferencePreToolUseMatcher is the reference PreToolUse matcher.
const ReferencePreToolUseMatcher = "Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit"

// ReferencePreToolUse is the reference PreToolUse command. When
// vibe-guard is not on PATH it says how to install version and refuses
// (exit 2), so a missing guard fails closed. command -v is a shell
// builtin, so the check works with only the operating system's PATH.
func ReferencePreToolUse(version string) string {
	return `command -v vibe-guard >/dev/null 2>&1 || { echo "vibe-guard is not installed; install it: ` +
		`go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@` + version +
		`" >&2; exit 2; }; vibe-guard hook`
}

// ReferenceStop is the reference Stop command: the relock runs first,
// whatever verify-fast does.
const ReferenceStop = "vibe-guard relock 2>/dev/null; task verify:fast 1>&2 </dev/null || task -x hook:done"
