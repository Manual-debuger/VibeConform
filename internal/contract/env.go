package contract

import (
	"context"
	"fmt"
	"strings"
)

// Env holds what the checks of one audit read: the configuration, the
// tracked files, the models built from them (each computed once and
// cached), the notes, and the probe session that the execution and
// probe parts run in. A check reads Env and returns what it decided in
// its PartResult (spec 0064). Note is the only record a check adds
// outside its result. A cache filled on first use is not a record: the
// model is the same whichever check asks first.
//
// The audit runs its checks one after another, on one goroutine, and Env
// relies on that: its caches are plain fields with no locking. A check
// must not hand Env to another goroutine.
type Env struct {
	ctx    context.Context
	config *Config
	repo   *Repo

	files          map[string]fileEntry
	entrypoints    map[string]*entrypointState
	workflows      *workflowSet
	ci             *ciState
	hookMgrs       *[]*hookManager
	claudeSettings *parsedClaudeSettings
	// guardLoad is vibeGuardLoadError's answer once worked out.
	guardLoad *string
	notes     []string
	// probe is the probe copy while the execution and probe parts run,
	// and nil otherwise.
	probe *probeSession
}

type fileEntry struct {
	data []byte
	err  error
}

func newEnv(ctx context.Context, cfg *Config, repo *Repo) *Env {
	return &Env{ctx: ctx, config: cfg, repo: repo, files: map[string]fileEntry{}}
}

// readFile returns a tracked file's working-tree content. The error wraps
// fs.ErrNotExist when the file is not tracked.
func (e *Env) readFile(name string) ([]byte, error) {
	if f, ok := e.files[name]; ok {
		return f.data, f.err
	}
	data, err := e.repo.ReadFile(name)
	e.files[name] = fileEntry{data, err}
	return data, err
}

// auditedContract is the contract version the audit applies, Contract1 or
// Contract2: the one the declared version is audited as. A check whose
// reading differs between contracts reads it here (spec 0072, "Contract
// versions").
func (e *Env) auditedContract() string {
	return e.config.Contract.Audited
}

// note records a fact the audit assumed, such as the default branch, for
// the report.
func (e *Env) note(format string, args ...any) {
	note := fmt.Sprintf(format, args...)
	for _, n := range e.notes {
		if n == note {
			return
		}
	}
	e.notes = append(e.notes, note)
}

// entrypointState is one entrypoint as V-04 sees it.
type entrypointState struct {
	commands []shellCommand
	// problems are V-04's findings; any problem makes the entrypoint
	// unusable for the rules that need it.
	problems []Finding
}

// entrypoint parses a declared entrypoint once.
func (e *Env) entrypoint(name string) *entrypointState {
	if e.entrypoints == nil {
		e.entrypoints = map[string]*entrypointState{}
	}
	if st, ok := e.entrypoints[name]; ok {
		return st
	}
	st := &entrypointState{}
	e.entrypoints[name] = st

	at := "entrypoints." + name
	file := e.config.file
	ep, declared := e.config.Entrypoints[name]
	switch {
	case !declared:
		st.problems = append(st.problems, Finding{File: file,
			Message: at + " is not declared",
			Fix:     "declare " + at + " as the command that runs it, such as " + exampleCommand(name)})
		return st
	case len(ep.Commands) == 0:
		st.problems = append(st.problems, Finding{File: file,
			Message: at + " is empty",
			Fix:     "set " + at + " to the command that runs it, such as " + exampleCommand(name)})
		return st
	}
	for i, src := range ep.Commands {
		where := at
		if len(ep.Commands) > 1 {
			where = fmt.Sprintf("%s[%d]", at, i)
		}
		cmd, err := ParseSimpleCommand(src)
		if err != nil {
			st.problems = append(st.problems, Finding{File: file,
				Message: fmt.Sprintf("%s %q %v", where, src, err),
				Fix:     "write each command as one simple command; split a sequence into a list of commands"})
			continue
		}
		st.commands = append(st.commands, cmd)
	}
	return st
}

// entrypointCommands returns a required or declared entrypoint's commands, parsed.
// When V-04 finds the entrypoint missing, empty or malformed, it returns
// nil and a finding that points at V-04: a rule that needs the
// entrypoint fails with it (spec 0052, "Missing entrypoint").
func (e *Env) entrypointCommands(name string) ([]shellCommand, *Finding) {
	st := e.entrypoint(name)
	if len(st.problems) == 0 {
		return st.commands, nil
	}
	return nil, &Finding{
		File:    e.config.file,
		Message: fmt.Sprintf("this rule needs the %s entrypoint, and V-04 finds it unusable: %s", name, st.problems[0].Message),
		Fix:     "fix V-04 first: " + st.problems[0].Fix,
	}
}

// entrypointSource returns an entrypoint's commands as written in the
// configuration, for checks that compare text (R-02).
func (e *Env) entrypointSource(name string) []string {
	return e.config.Entrypoints[name].Commands
}

func exampleCommand(name string) string {
	switch name {
	case entrypointVerifyFast:
		return "task verify:fast"
	case entrypointFmtCheck:
		return "task fmt:check"
	default:
		return "task " + strings.ReplaceAll(name, "-", ":")
	}
}
