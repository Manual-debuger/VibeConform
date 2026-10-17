// Package guardrule holds what the contract audit and vibe-guard must
// agree on (spec 0053, "Guard rule IDs and exceptions, shared"): the ten
// guard rule IDs of contract 1.0.0, the guard-internal IDs, policy.json
// version 2, exception matching, and the decomposition of a Bash or
// PowerShell command into simple commands that both use to match
// exceptions. It imports no package of the old template system.
package guardrule

import "slices"

// The guard rule IDs of contract 1.0.0 (spec 0051, "The guard corpus").
const (
	RmRf                   = "rm-rf"
	RemoveItemRecurseForce = "remove-item-recurse-force"
	GitResetHard           = "git-reset-hard"
	GitCleanFD             = "git-clean-fd"
	GitPushForce           = "git-push-force"
	GitBranchForceDelete   = "git-branch-force-delete"
	GitDiscardWorktree     = "git-discard-worktree"
	SecretFile             = "secret-file"
	GuardPolicyFile        = "guard-policy-file"
	GuardrailSettings      = "guardrail-settings"
)

// IDs lists the ten guard rule IDs in the order spec 0051 tables them. A
// corpus deny case and a guard exception name one of them.
var IDs = []string{
	RmRf, RemoveItemRecurseForce, GitResetHard, GitCleanFD, GitPushForce,
	GitBranchForceDelete, GitDiscardWorktree, SecretFile, GuardPolicyFile, GuardrailSettings,
}

// The guard-internal IDs (spec 0053, "Parsing"). A refusal for one of
// these is not a contract rule, and no exception can name it.
const (
	IDParseError     = "parse-error"
	IDPayloadError   = "payload-error"
	IDPolicyError    = "policy-error"
	IDDynamicCommand = "dynamic-command"
)

// InternalIDs lists the guard-internal IDs.
var InternalIDs = []string{IDParseError, IDPayloadError, IDPolicyError, IDDynamicCommand}

// IsContractID reports whether id is one of the ten guard rule IDs.
func IsContractID(id string) bool { return slices.Contains(IDs, id) }

// IsInternalID reports whether id is a guard-internal ID.
func IsInternalID(id string) bool { return slices.Contains(InternalIDs, id) }

// IsPathRule reports whether id is a rule on file edits, whose exception
// match is a path glob. The other contract rules are command rules, whose
// match is a word prefix.
func IsPathRule(id string) bool {
	return id == SecretFile || id == GuardPolicyFile || id == GuardrailSettings
}

// The Claude Code tools the guard reads (spec 0053, "Tools").
const (
	Bash         = "Bash"
	PowerShell   = "PowerShell"
	Write        = "Write"
	Edit         = "Edit"
	MultiEdit    = "MultiEdit"
	NotebookEdit = "NotebookEdit"
)

// A01Tools are the tools A-01 requires a PreToolUse hook for (spec 0051).
// Every corpus case uses one of them.
var A01Tools = []string{Bash, PowerShell, Write, Edit, MultiEdit}

// CommandTools run a command; EditTools write a file.
var (
	CommandTools = []string{Bash, PowerShell}
	EditTools    = []string{Write, Edit, MultiEdit, NotebookEdit}
)

// IsCommandTool reports whether tool runs a command.
func IsCommandTool(tool string) bool { return slices.Contains(CommandTools, tool) }

// IsEditTool reports whether tool writes a file.
func IsEditTool(tool string) bool { return slices.Contains(EditTools, tool) }
