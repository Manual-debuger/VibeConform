package contract

import (
	"context"
	"fmt"
	"strings"
)

// Env is what a check reads: the configuration, the tracked files, and
// the models built from them, each built once per audit. A check never
// writes.
type Env struct {
	ctx    context.Context
	Config *Config
	Repo   *Repo

	files       map[string]fileEntry
	entrypoints map[string]*entrypointState
	workflows   *Workflows
	notes       []string
}

type fileEntry struct {
	data []byte
	err  error
}

func newEnv(ctx context.Context, cfg *Config, repo *Repo) *Env {
	return &Env{ctx: ctx, Config: cfg, Repo: repo, files: map[string]fileEntry{}}
}

// Context is the audit's context, for checks that run git.
func (e *Env) Context() context.Context { return e.ctx }

// ReadFile returns a tracked file's working-tree content. The error wraps
// fs.ErrNotExist when the file is not tracked.
func (e *Env) ReadFile(name string) ([]byte, error) {
	if f, ok := e.files[name]; ok {
		return f.data, f.err
	}
	data, err := e.Repo.ReadFile(name)
	e.files[name] = fileEntry{data, err}
	return data, err
}

// Note records a fact the audit assumed, such as the default branch, for
// the report.
func (e *Env) Note(format string, args ...any) {
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
	commands []Command
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
	file := e.Config.File
	ep, declared := e.Config.Entrypoints[name]
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

// Entrypoint returns a required or declared entrypoint's commands, parsed.
// When V-04 finds the entrypoint missing, empty or malformed, it returns
// nil and a finding that points at V-04: a rule that needs the
// entrypoint fails with it (spec 0052, "Missing entrypoint").
func (e *Env) Entrypoint(name string) ([]Command, *Finding) {
	st := e.entrypoint(name)
	if len(st.problems) == 0 {
		return st.commands, nil
	}
	return nil, &Finding{
		File:    e.Config.File,
		Message: fmt.Sprintf("this rule needs the %s entrypoint, and V-04 finds it unusable: %s", name, st.problems[0].Message),
		Fix:     "fix V-04 first: " + st.problems[0].Fix,
	}
}

// EntrypointSource returns an entrypoint's commands as written in the
// configuration, for checks that compare text (R-02).
func (e *Env) EntrypointSource(name string) []string {
	return e.Config.Entrypoints[name].Commands
}

func exampleCommand(name string) string {
	switch name {
	case EntrypointVerifyFast:
		return "task verify:fast"
	case EntrypointFmtCheck:
		return "task fmt:check"
	default:
		return "task " + strings.ReplaceAll(name, "-", ":")
	}
}
