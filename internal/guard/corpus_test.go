package guard_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// payloadFor is the PreToolUse payload Claude Code sends for c, run in
// cwd.
func payloadFor(t *testing.T, c contract.GuardCase, cwd string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      "test",
		"transcript_path": filepath.Join(cwd, "transcript.jsonl"),
		"cwd":             cwd,
		"hook_event_name": "PreToolUse",
		"tool_name":       c.Payload.ToolName,
		"tool_input":      c.Payload.ToolInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// corpusRepo makes a fixture repository in the case's unlock state, an
// unlock marker being valid or expired at now: testNow for an in-process
// run, the wall clock for a subprocess.
func corpusRepo(t *testing.T, c contract.GuardCase, now time.Time) repo {
	t.Helper()
	r := newRepo(t, enforcePolicy)
	switch c.Unlock {
	case contract.UnlockValid:
		writeFile(t, filepath.Join(r.CommonDir, "vibe-unlock"), marker(now.Add(10*time.Minute)))
	case contract.UnlockExpired:
		writeFile(t, filepath.Join(r.CommonDir, "vibe-unlock"), marker(now.Add(-time.Minute)))
	}
	return r
}

// explainArgs are the explain arguments for the case's call.
func explainArgs(c contract.GuardCase) []string {
	tgt := c.Target()
	return []string{"--tool", tgt.Tool(), tgt.Text()}
}

// TestCorpus runs the whole contract guard corpus through "vibe-guard
// hook", in-process and under -short too: every deny case is refused
// (exit 2) by a line that names the case's rule ID, every allow case is
// allowed silently, and "vibe-guard explain" gives the same decision and
// rule (#105's done-when).
func TestCorpus(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	for _, c := range contract.GuardCorpus() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := corpusRepo(t, c, testNow)
			got := r.hook(t, payloadFor(t, c, r.Root))
			ex := r.explain(t, explainArgs(c)...)
			switch c.Want {
			case contract.WantDeny:
				want := "vibe-guard: refused by rule " + c.Rule + ": "
				if got.Code != 2 || !strings.HasPrefix(got.Stderr, want) {
					t.Errorf("hook: exit %d, stderr %q; want exit 2 and a line starting %q", got.Code, got.Stderr, want)
				}
				if strings.Count(got.Stderr, "\n") != 1 || !strings.Contains(got.Stderr, ". Explain: vibe-guard explain ") {
					t.Errorf("hook: the refusal is not one line naming explain: %q", got.Stderr)
				}
				if ex.Code != 2 || !hasLine(ex.Stdout, "rule: "+c.Rule) || !hasLine(ex.Stdout, "decision: deny") {
					t.Errorf("explain: exit %d, output\n%s\nwant exit 2, decision deny and rule %s", ex.Code, ex.Stdout, c.Rule)
				}
			case contract.WantAllow:
				if got.Code != 0 || got.Stderr != "" || got.Stdout != "" {
					t.Errorf("hook: exit %d, stdout %q, stderr %q; want a silent exit 0", got.Code, got.Stdout, got.Stderr)
				}
				if ex.Code != 0 || !hasLine(ex.Stdout, "decision: allow") {
					t.Errorf("explain: exit %d, output\n%s\nwant exit 0 and decision allow", ex.Code, ex.Stdout)
				}
			}
		})
	}
}

// TestCorpusFixedAndRefuses checks the corpus's fixed and refuses against
// vibe-guard's rules (spec 0072, "Guard corpus"): a deny case is fixed
// exactly when a fixed rule refuses it, and the refuses of a case of more
// than one simple command names, in source order, exactly the simple
// commands its rule refuses.
func TestCorpusFixedAndRefuses(t *testing.T) {
	for _, c := range contract.GuardCorpus() {
		if c.Want != contract.WantDeny {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := corpusRepo(t, c, testNow)
			g := guard.New(gr.FindRepo(r.Root, r.Root), guard.Options{Now: testNow, Dir: r.Root})
			refused, fixed := corpusRefusals(t, g, c)
			if fixed != c.Fixed {
				t.Errorf("the corpus says fixed %v; a fixed rule of vibe-guard refuses the case: %v", c.Fixed, fixed)
			}
			if !slices.Equal(refused, c.Refuses) {
				t.Errorf("the corpus says refuses %q; rule %s refuses %q", c.Refuses, c.Rule, refused)
			}
		})
	}
}

// corpusRefusals is what vibe-guard's rule c.Rule refuses of the deny
// case c: the simple commands of a compound command case, in source
// order (none for an edit or a single command), and whether a fixed rule
// refuses the case.
func corpusRefusals(t *testing.T, g *guard.Guard, c contract.GuardCase) (refused []string, fixed bool) {
	t.Helper()
	tgt := c.Target()
	if tgt.Kind() == gr.PathTarget {
		ok, fixed := g.RuleRefusesEdit(c.Rule, tgt.Text())
		if !ok {
			t.Fatalf("rule %s does not refuse the edit of %s", c.Rule, tgt.Text())
		}
		return nil, fixed
	}
	cmds, err := gr.Commands(gr.ShellFor(tgt.Tool()), tgt.Text())
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if ok, f := g.RuleRefuses(c.Rule, cmd); ok {
			refused = append(refused, cmd.String())
			fixed = fixed || f
		}
	}
	if len(refused) == 0 {
		t.Fatalf("rule %s refuses no simple command of %q", c.Rule, tgt.Text())
	}
	if len(cmds) == 1 {
		return nil, fixed
	}
	return refused, fixed
}

func hasLine(out, line string) bool {
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// TestRefusalExplainCommandQuotes: the refusal is one line in the exact
// format of spec 0053, and the explain command it names quotes the
// command for a shell, a multi-line one as $'…'.
func TestRefusalExplainCommandQuotes(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	got := r.hook(t, `{"tool_name":"Bash","tool_input":{"command":"echo 'a b'\nrm -rf build"}}`)
	want := `vibe-guard: refused by rule rm-rf: rm with -r and -f deletes a directory tree without asking; ` +
		`ask the user to run it themselves (in Claude Code: ! <command>). ` +
		`Explain: vibe-guard explain $'echo \'a b\'\nrm -rf build'` + "\n"
	if got.Code != 2 || got.Stderr != want {
		t.Errorf("got exit %d, stderr\n%q\nwant\n%q", got.Code, got.Stderr, want)
	}
}
