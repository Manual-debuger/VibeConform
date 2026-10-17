package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// Exit codes. Claude Code reads exit 2 from a PreToolUse hook as a
// refusal and shows its stderr to the agent.
const (
	exitAllow = 0
	exitDeny  = 2
	exitUsage = 64
)

// MarkerFile is the unlock marker's name in the common Git directory
// (spec 0051, "The unlock marker").
const MarkerFile = "vibe-unlock"

// LogFile is observe mode's log, in the common Git directory.
const LogFile = "vibe-guard.log"

// Env is what a command reads from its process.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	// Dir is the working directory.
	Dir string
	Now func() time.Time
}

const usage = `usage:
  vibe-guard hook                                  decide the PreToolUse payload on stdin
  vibe-guard explain [--tool <tool>] '<command or path>'
  vibe-guard relock                                delete the unlock marker
  vibe-guard version`

// Main runs vibe-guard with args (without the program name) and returns
// the exit code.
func Main(args []string, env Env, version string) int {
	if len(args) == 0 {
		say(env.Stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "hook":
		if len(args) != 1 {
			break
		}
		return hook(env)
	case "explain":
		return explain(args[1:], env)
	case "relock":
		if len(args) != 1 {
			break
		}
		return relock(env)
	case "version", "--version":
		say(env.Stdout, "vibe-guard "+version)
		return 0
	case "help", "-h", "--help":
		say(env.Stdout, usage)
		return 0
	}
	say(env.Stderr, usage)
	return exitUsage
}

// startDir is where the repository root is looked for: CLAUDE_PROJECT_DIR,
// then the payload's cwd, then the process's working directory.
func startDir(env Env, cwd string) string {
	if d := env.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		return d
	}
	if cwd != "" {
		return cwd
	}
	return env.Dir
}

type payload struct {
	ToolName  string
	ToolInput json.RawMessage
	Cwd       string
}

// readPayload reads a PreToolUse payload. The error is a payload error.
func readPayload(r io.Reader) (payload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return payload{}, fmt.Errorf("stdin cannot be read: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return payload{}, errors.New("stdin is empty; a PreToolUse payload was expected")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return payload{}, errors.New("stdin is not a JSON object")
	}
	var p payload
	if raw, ok := m["cwd"]; ok {
		_ = json.Unmarshal(raw, &p.Cwd)
	}
	raw, ok := m["tool_name"]
	if !ok || json.Unmarshal(raw, &p.ToolName) != nil || p.ToolName == "" {
		return p, errors.New("the payload has no string tool_name")
	}
	p.ToolInput = m["tool_input"]
	return p, nil
}

func hook(env Env) int {
	p, perr := readPayload(env.Stdin)
	g := New(FindRepo(startDir(env, p.Cwd)), env.Now())
	var d Decision
	var t gr.Target
	if perr == nil {
		t, perr = gr.ParseTarget(p.ToolName, p.ToolInput)
	}
	if perr != nil {
		d = g.DecideError(gr.IDPayloadError, perr.Error())
	} else {
		d = g.Decide(t)
	}
	switch {
	case d.Outcome == Ask:
		out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "ask",
			"permissionDecisionReason": "vibe-guard: rule " + d.Finding.Rule + ": " + d.Finding.Why,
		}})
		say(env.Stdout, string(out))
		return exitAllow
	case d.Outcome == Deny && d.Observe:
		line := "vibe-guard (observe): would refuse by rule " + d.Finding.Rule + ": " + reasonTail(d, t)
		say(env.Stderr, line)
		if g.Repo.CommonDir != "" {
			if err := appendLine(filepath.Join(g.Repo.CommonDir, LogFile), line); err != nil {
				say(env.Stderr, "vibe-guard (observe): the log cannot be written: "+err.Error())
			}
		}
		return exitAllow
	case d.Outcome == Deny:
		say(env.Stderr, "vibe-guard: refused by rule "+d.Finding.Rule+": "+reasonTail(d, t))
		return exitDeny
	}
	return exitAllow
}

// reasonTail is "<why>. Explain: vibe-guard explain '<command>'".
func reasonTail(d Decision, t gr.Target) string {
	s := strings.TrimSuffix(d.Finding.Why, ".") + "."
	if t.Tool == "" {
		return s
	}
	arg := t.Command
	if gr.IsEditTool(t.Tool) {
		arg = t.Path
	}
	explain := "vibe-guard explain "
	if t.Tool != gr.Bash {
		explain += "--tool " + t.Tool + " "
	}
	return s + " Explain: " + explain + shellQuote(arg)
}

// shellQuote quotes s for a POSIX shell, on one line: in single quotes,
// or as $'…' when it holds a control character.
func shellQuote(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	var b strings.Builder
	b.WriteString("$'")
	for _, r := range s {
		switch {
		case r == '\\' || r == '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			sayf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func appendLine(file, line string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- vibe-guard.log in the repository's own common Git directory
	if err != nil {
		return err
	}
	_, err = f.WriteString(line + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func relock(env Env) int {
	repo := FindRepo(startDir(env, ""))
	if repo.CommonDir == "" {
		say(env.Stderr, "vibe-guard: relock: no Git directory found from "+repo.Root)
		return 1
	}
	err := os.Remove(filepath.Join(repo.CommonDir, MarkerFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		say(env.Stderr, "vibe-guard: relock: "+err.Error())
		return 1
	}
	return 0
}

func explain(args []string, env Env) int {
	tool := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--tool" && i+1 < len(args):
			tool = args[i+1]
			i++
		case strings.HasPrefix(a, "--tool="):
			tool = strings.TrimPrefix(a, "--tool=")
		case a == "--":
			rest = append(rest, args[i+1:]...)
			i = len(args)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		say(env.Stderr, usage)
		return exitUsage
	}
	arg := rest[0]
	if tool == "" {
		tool = gr.Bash
		if !strings.ContainsAny(arg, " \t\n") {
			if _, err := os.Stat(arg); err == nil {
				tool = gr.Write
			}
		}
	}
	if !gr.IsCommandTool(tool) && !gr.IsEditTool(tool) {
		sayf(env.Stderr, "vibe-guard: explain: --tool %q is not one of %s\n", tool,
			strings.Join(append(append([]string{}, gr.CommandTools...), gr.EditTools...), ", "))
		return exitUsage
	}
	t := gr.Target{Tool: tool, Command: arg}
	if gr.IsEditTool(tool) {
		t = gr.Target{Tool: tool, Path: arg}
	}
	g := New(FindRepo(startDir(env, "")), env.Now())
	d := g.Decide(t)
	w := env.Stdout
	sayf(w, "tool: %s\n", tool)
	if gr.IsEditTool(tool) {
		sayf(w, "path: %s\n", arg)
	} else {
		sayf(w, "command: %s\n", arg)
	}
	observe := ""
	if d.Outcome == Deny && d.Observe {
		observe = " (observe mode: the hook allows the call and logs it)"
	}
	sayf(w, "decision: %s%s\n", d.Outcome, observe)
	if f := d.Finding; f != nil {
		sayf(w, "rule: %s\n", f.Rule)
		sayf(w, "why: %s\n", f.Why)
		writeMatch(w, *f)
	}
	for _, f := range d.Lifted {
		sayf(w, "lifted: rule %s by the exception {rule: %s, match: %q, reason: %q}\n",
			f.Rule, f.Exception.Rule, f.Exception.Match, f.Exception.Reason)
		writeMatch(w, f)
	}
	if d.Finding == nil && len(d.Lifted) == 0 {
		say(w, "rule: none")
	}
	if d.Outcome == Deny && !d.Observe {
		return exitDeny
	}
	return exitAllow
}

func writeMatch(w io.Writer, f Finding) {
	if f.Path != "" {
		sayf(w, "  matched path: %s\n", f.Path)
	}
	if c := f.Command; c != nil {
		sayf(w, "  matched simple command (%s): %s\n", c.Shell, c.String())
		words := make([]string, len(c.Words))
		for i, word := range c.Words {
			words[i] = describeWord(word)
		}
		sayf(w, "  words: %s\n", strings.Join(words, " "))
		if len(c.Via) > 0 {
			sayf(w, "  found through: %s\n", strings.Join(c.Via, ", "))
		}
	}
}

func describeWord(w gr.Word) string {
	s := "[" + strings.ReplaceAll(w.Text, gr.ExpansionMark, "$…") + "]"
	switch {
	case w.Unknown:
		s += "(unknown: an unquoted expansion)"
	case !w.Literal:
		s += "(operand with an expansion)"
	}
	return s
}

// say writes s and a newline. vibe-guard has nowhere to report a failed
// write to its own stdout or stderr.
func say(w io.Writer, s string) { _, _ = io.WriteString(w, s+"\n") }

func sayf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
