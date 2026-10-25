package guard_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Shell writes to the guardrail files (spec 0058, #123; Testing
// Decisions, Seam 1): a Bash or PowerShell command that writes,
// deletes, moves or renames .claude/settings.json, a file under
// .claude/hooks/ or policy.json gets the decision an Edit of the same
// path gets.

func commandPayload(tool, command, cwd string) string {
	p := map[string]any{"tool_name": tool, "tool_input": map[string]string{"command": command}}
	if cwd != "" {
		p["cwd"] = cwd
	}
	data, _ := json.Marshal(p)
	return string(data)
}

func psPayload(command string) string { return commandPayload("PowerShell", command, "") }

// bashWrites and psWrites write the file <F>; x.json is another file.
var bashWrites = []string{
	"sed -i 's/a/b/' <F>",
	"sed -i.bak -e s/a/b/ <F>",
	"sed --in-place s/a/b/ <F>",
	"echo x > <F>",
	"echo x >> <F>",
	"echo x >| <F>",
	"echo x &> <F>",
	"echo x &>> <F>",
	"echo x 2> <F>",
	"echo x 2>> <F>",
	"cat <<'EOF' > <F>\n{}\nEOF",
	"echo x | tee <F>",
	"tee -a <F> < /dev/null",
	"truncate -s 0 <F>",
	"cp x.json <F>",
	"mv x.json <F>",
	"mv <F> x.json",
	"install -m 644 x.json <F>",
	"ln -sf x.json <F>",
	"rm <F>",
	"unlink <F>",
	"dd if=x.json of=<F>",
	"bash -c 'echo x > <F>'",
	"sudo tee <F> < x.json",
	"sudo sed -i s/a/b/ <F>",
	"bash -c \"sudo cp x.json <F>\"",
	"echo x | xargs -I{} cp {} <F>",
}

var psWrites = []string{
	"Set-Content <F> x",
	"Set-Content -Value x -Path <F>",
	"Add-Content -LiteralPath <F> -Value x",
	"'x' | Out-File <F>",
	"'x' | Out-File -FilePath <F> -Append",
	"Clear-Content <F>",
	"Remove-Item <F>",
	"Rename-Item <F> x.json",
	"New-Item -ItemType File <F> -Force",
	"'x' | Tee-Object -FilePath <F>",
	"Copy-Item x.json -Destination <F>",
	"Move-Item <F> x.json",
	"Move-Item -Path x.json -Destination <F>",
	"'x' > <F>",
	"Get-Date *> <F>",
	"Get-Date 2>> <F>",
	"bash -c 'echo x > <F>'",
	"pwsh -c 'Set-Content <F> x'",
}

type shellWrite struct{ tool, command string }

func shellWrites(file string) []shellWrite {
	var out []shellWrite
	for _, c := range bashWrites {
		out = append(out, shellWrite{"Bash", strings.ReplaceAll(c, "<F>", file)})
	}
	for _, c := range psWrites {
		out = append(out, shellWrite{"PowerShell", strings.ReplaceAll(c, "<F>", file)})
	}
	return out
}

// TestShellWritesToGuardrailFiles: every write form, in Bash and in
// PowerShell, with no marker, a valid marker and an expired one. An
// unlock opens the settings and hooks, never policy.json.
func TestShellWritesToGuardrailFiles(t *testing.T) {
	files := []struct {
		path string
		rule string
		// unlockable is true when a valid unlock lifts the rule.
		unlockable bool
	}{
		{".claude/settings.json", "guardrail-settings", true},
		{".claude/hooks/pre.sh", "guardrail-settings", true},
		{".claude/hooks/policy.json", "guard-policy-file", false},
	}
	markers := []struct {
		name   string
		marker string
		valid  bool
	}{
		{"no marker", "", false},
		{"valid marker", marker(time.Now().Add(time.Hour)), true},
		{"expired marker", marker(time.Now().Add(-time.Minute)), false},
	}
	for _, m := range markers {
		for _, f := range files {
			t.Run(m.name+"/"+f.path, func(t *testing.T) {
				t.Parallel()
				r := newRepo(t, enforcePolicy)
				if m.marker != "" {
					writeFile(t, filepath.Join(r.CommonDir, "vibe-unlock"), m.marker)
				}
				for _, w := range shellWrites(f.path) {
					got := r.hook(t, commandPayload(w.tool, w.command, ""))
					if m.valid && f.unlockable {
						if got.Code != 0 || got.Stderr != "" {
							t.Errorf("%s %q: exit %d, stderr %q; want allowed", w.tool, w.command, got.Code, got.Stderr)
						}
						continue
					}
					want := "vibe-guard: refused by rule " + f.rule + ": "
					if got.Code != 2 || !strings.HasPrefix(got.Stderr, want) {
						t.Errorf("%s %q: exit %d, stderr %q; want a refusal by %s", w.tool, w.command, got.Code, got.Stderr, f.rule)
					}
				}
			})
		}
	}
}

// TestShellWritesThatAreAllowed: reads of the guardrail files, writes to
// other files (.env included: secret-file is an edit rule), and a written
// path with an expansion, the documented gap.
func TestShellWritesThatAreAllowed(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	for _, w := range []shellWrite{
		{"Bash", "cat .claude/settings.json"},
		{"Bash", "jq . < .claude/settings.json"},
		{"Bash", "sed -n p .claude/hooks/policy.json"},
		{"Bash", "cp .claude/settings.json backup.json"},
		{"Bash", "grep -r x .claude/hooks"},
		{"Bash", "echo x > notes.txt"},
		{"Bash", "cp .env.example .env"},
		{"Bash", "echo X=1 >> .env"},
		{"Bash", "echo x > sub/.claude/settings.json"},
		{"Bash", `echo x > "$f"`},
		{"Bash", "f=.claude/settings.json; echo x > $f"},
		{"PowerShell", "Get-Content .claude/settings.json"},
		{"PowerShell", "Get-Content .claude/hooks/policy.json | Set-Content backup.json"},
		{"PowerShell", "Set-Content .env X=1"},
		{"PowerShell", "Copy-Item .claude/settings.json -Destination backup.json"},
	} {
		assertAllowed(t, r.hook(t, commandPayload(w.tool, w.command, "")))
	}
}

// TestShellWriteCommandRuleFirst: a command rule that matches first
// still decides.
func TestShellWriteCommandRuleFirst(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	assertRefused(t, r.hook(t, bashPayload("rm -rf .claude/settings.json")), "rm-rf")
	assertRefused(t, r.hook(t, bashPayload("echo x > .claude/settings.json; git reset --hard")), "guardrail-settings")
}

// TestShellWritePathsResolveAgainstCwd: a relative written path is
// resolved against the payload's cwd, and the root when there is none.
func TestShellWritePathsResolveAgainstCwd(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := filepath.Join(r.Root, "sub", "dir")
	dotClaude := filepath.Join(r.Root, ".claude")
	assertRefused(t, r.hook(t, commandPayload("Bash", "echo x > ../../.claude/settings.json", sub)), "guardrail-settings")
	assertAllowed(t, r.hook(t, commandPayload("Bash", "echo x > .claude/settings.json", sub)))
	assertRefused(t, r.hook(t, commandPayload("Bash", "sed -i s/a/b/ settings.json", dotClaude)), "guardrail-settings")
	assertRefused(t, r.hook(t, commandPayload("PowerShell", `Set-Content hooks\policy.json x`, dotClaude)), "guard-policy-file")
	assertRefused(t, r.hook(t, commandPayload("Bash", "tee "+filepath.ToSlash(filepath.Join(r.Root, ".claude", "settings.json")), sub)),
		"guardrail-settings")
	// A directory destination writes the source's name inside it.
	assertRefused(t, r.hook(t, commandPayload("Bash", "cp backup/settings.json .claude/", "")), "guardrail-settings")
	assertRefused(t, r.hook(t, commandPayload("Bash", "cp -t .claude/hooks x.json", "")), "guardrail-settings")
	assertRefused(t, r.hook(t, commandPayload("Bash", "cp policy.json ../hooks", filepath.Join(dotClaude, "x"))), "guard-policy-file")
	// The same names outside the repository are not its guardrails.
	assertAllowed(t, r.hook(t, commandPayload("Bash", "echo x > ../.claude/settings.json", "")))
	if runtime.GOOS == "windows" {
		// Git Bash spells D:\repo as /d/repo.
		msys := "/" + strings.ToLower(r.Root[:1]) + filepath.ToSlash(r.Root[2:])
		assertRefused(t, r.hook(t, bashPayload("echo x > "+msys+"/.claude/settings.json")), "guardrail-settings")
	}
}

// TestShellWriteException: an exception for either rule lifts a shell
// write as it lifts an edit.
func TestShellWriteException(t *testing.T) {
	r := newRepo(t, `{"version": 2, "exceptions": [
		{"rule": "guardrail-settings", "match": ".claude/settings.json", "reason": "the agent maintains its settings"},
		{"rule": "guard-policy-file", "match": ".claude/hooks/policy.json", "reason": "a test"},
		{"rule": "guardrail-settings", "match": ".claude/hooks/policy.json", "reason": "policy.json is under .claude/hooks/ too"}]}`)
	assertAllowed(t, r.hook(t, bashPayload("sed -i s/a/b/ .claude/settings.json")))
	assertAllowed(t, r.hook(t, psPayload("Set-Content .claude/hooks/policy.json x")))
	assertRefused(t, r.hook(t, bashPayload("echo x > .claude/hooks/pre.sh")), "guardrail-settings")
}

// TestExplainShellWrite names the rule, the matched path and the simple
// command that writes it.
func TestExplainShellWrite(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	got := r.explain(t, "sudo sed -i s/a/b/ .claude/settings.json")
	for _, want := range []string{"decision: deny", "rule: guardrail-settings", "  matched path: .claude/settings.json",
		"  matched simple command (Bash): sed -i s/a/b/ .claude/settings.json", "  found through: sudo"} {
		if !hasLine(got.Stdout, want) {
			t.Errorf("explain output\n%s\nhas no line %q", got.Stdout, want)
		}
	}
	if got.Code != 2 {
		t.Errorf("exit %d; want 2", got.Code)
	}
	got = r.explain(t, "--tool", "PowerShell", "'{}' | Out-File .claude/hooks/policy.json")
	if got.Code != 2 || !hasLine(got.Stdout, "rule: guard-policy-file") || !hasLine(got.Stdout, "  matched path: .claude/hooks/policy.json") {
		t.Errorf("exit %d, output\n%s", got.Code, got.Stdout)
	}
}
