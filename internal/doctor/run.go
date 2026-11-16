package doctor

import (
	"context"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// Loaded is the outcome of loading the repository's configuration, as
// contract.Load returns it.
type Loaded struct {
	// Config and Repo are the configuration and its repository. They are
	// read only when Err is nil.
	Config *contract.Config
	Repo   *contract.Repo
	// Err is the load error, with the text the caller chose for it. The
	// vibe.yaml line prints it as it is.
	Err error
}

// Run runs every check in a fixed order and returns the report: git, the
// configuration, the program of each entrypoint, then what the hooks
// need. A configuration that did not load fails its own line and stops
// the checks that need it.
func Run(ctx context.Context, env Env, root string, loaded Loaded) Report {
	var report Report
	report.Add(checkGit(ctx, env, root))
	if loaded.Err != nil {
		report.Add(Result{Status: Fail, Name: contract.ConfigFile, Detail: loaded.Err.Error()})
		return report
	}
	report.Add(Result{Status: Pass, Name: contract.ConfigFile,
		Detail: "version 2, contract " + loaded.Config.Contract.Declared})
	report.Add(checkEntrypoints(env, root, loaded.Config.Entrypoints)...)
	report.Add(checkHooks(env, loaded.Repo)...)
	return report
}

// checkHooks checks what the hooks of the tracked .claude/settings.json
// need: bash, and vibe-guard when a PreToolUse hook runs it. A file that
// is not tracked has no hooks; one that does not parse is A-01's to
// report.
func checkHooks(env Env, repo *contract.Repo) []Result {
	var hooks contract.ClaudeHooks
	if repo.HasFile(contract.ClaudeSettingsFile) {
		data, err := repo.ReadFile(contract.ClaudeSettingsFile)
		if err == nil {
			hooks, err = contract.ReadClaudeHooks(data)
		}
		if err != nil {
			return []Result{{Status: Unverified, Name: "hooks",
				Detail: contract.ClaudeSettingsFile + ": " + err.Error() + "; vibe audit reports it"}}
		}
	}
	return append([]Result{checkBash(env, hooks)}, checkVibeGuard(env, hooks)...)
}
