package guard_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// One path reading (spec 0069): every path the guard checks, from an
// edit tool or a shell write, is made absolute against the payload's cwd
// (else the project root), with Git Bash's /d/… read as D:/…, and
// cleaned before any rule sees it.

// dotDot spells the path rel inside root with a ".." that leaves root
// and comes back: relative ("../root/rel") and absolute
// ("<root>/../root/rel"). filepath.Join would clean the "..", so the
// paths are put together by hand.
func dotDot(root, rel string) (relative, absolute string) {
	base := filepath.Base(root)
	relative = "../" + base + "/" + rel
	absolute = filepath.ToSlash(root) + "/../" + base + "/" + rel
	return relative, absolute
}

// assertExplainRefused runs explain for an edit tool and checks the rule.
func assertExplainRefused(t *testing.T, r repo, tool, p, rule string) {
	t.Helper()
	got := r.explain(t, "--tool", tool, p)
	if got.Code != 2 || !hasLine(got.Stdout, "rule: "+rule) {
		t.Errorf("explain --tool %s %s: exit %d, output\n%s\nwant exit 2 and rule %s", tool, p, got.Code, got.Stdout, rule)
	}
}

// TestGuardrailSettingsThroughDotDot is a regression test (spec 0069,
// bug 1): an edit path that leaves the project with ".." and comes back
// was allowed, so the agent could change its own hooks without an
// unlock. It is refused by hook and explain, and a valid marker allows
// it.
func TestGuardrailSettingsThroughDotDot(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	var paths []string
	for _, rel := range []string{".claude/settings.json", ".claude/hooks/x.sh"} {
		relative, absolute := dotDot(r.Root, rel)
		paths = append(paths, relative, absolute)
	}
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		for _, p := range paths {
			t.Run(tool+" "+p, func(t *testing.T) {
				assertRefused(t, r.hook(t, editPayload(tool, p, "")), "guardrail-settings")
				assertRefused(t, r.hook(t, editPayload(tool, p, r.Root)), "guardrail-settings")
				assertExplainRefused(t, r, tool, p, "guardrail-settings")
			})
		}
	}

	writeFile(t, filepath.Join(r.CommonDir, "vibe-unlock"), marker(time.Now().Add(10*time.Minute)))
	for _, p := range paths {
		assertAllowed(t, r.hook(t, editPayload("Write", p, r.Root)))
	}
}

// TestBuiltInPathRulesThroughDotDot: the rules that match a suffix
// already refused a ".." spelling, and still do.
func TestBuiltInPathRulesThroughDotDot(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	policy, policyAbs := dotDot(r.Root, ".claude/hooks/policy.json")
	env, envAbs := dotDot(r.Root, ".env")
	for _, p := range []string{policy, policyAbs} {
		assertRefused(t, r.hook(t, writePayload(p)), "guard-policy-file")
	}
	for _, p := range []string{env, envAbs} {
		assertRefused(t, r.hook(t, writePayload(p)), "secret-file")
	}
}

// TestDotDotToASiblingProject: a path that really leaves the project is
// not this project's, as before.
func TestDotDotToASiblingProject(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	for _, p := range []string{"../other/.claude/settings.json", filepath.ToSlash(r.Root) + "/../other/.claude/settings.json"} {
		assertAllowed(t, r.hook(t, writePayload(p)))
	}
}

// TestProjectBelowGitTopThroughDotDot: from top/proj, a path that leaves
// the Git top and comes back to its settings is refused (spec 0068's
// protection holds for every spelling).
func TestProjectBelowGitTopThroughDotDot(t *testing.T) {
	top, proj := nestedProject(t, "", enforcePolicy)
	p := "../../" + filepath.Base(top.Root) + "/.claude/settings.json"
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		assertRefused(t, proj.hook(t, editPayload(tool, p, proj.Root)), "guardrail-settings")
	}
	assertExplainRefused(t, proj, "Write", p, "guardrail-settings")
	assertRefused(t, proj.hook(t, editPayload("Write", "../.claude/settings.json", proj.Root)), "guardrail-settings")
}

// TestRepositoryPathRulesThroughDotDot: a repository path rule and an
// exception glob match the ".." spelling of a path they match.
func TestRepositoryPathRulesThroughDotDot(t *testing.T) {
	r := newRepo(t, `{"version": 2,
		"rules": [{"id": "no-secrets", "level": "deny", "path": "secrets/**", "reason": "the secrets directory"}],
		"exceptions": [{"rule": "secret-file", "match": ".env", "reason": "a fixture env file"}]}`)
	secrets, secretsAbs := dotDot(r.Root, "secrets/a")
	for _, p := range []string{"secrets/a", secrets, secretsAbs} {
		assertRefused(t, r.hook(t, writePayload(p)), "no-secrets")
	}
	assertAllowed(t, r.hook(t, writePayload("../other/secrets/a")))
	env, envAbs := dotDot(r.Root, ".env")
	for _, p := range []string{".env", env, envAbs} {
		assertAllowed(t, r.hook(t, writePayload(p)))
	}
	assertRefused(t, r.hook(t, writePayload("../other/.env")), "secret-file")
}

// TestEditPathsResolveAgainstCwd is a regression test (spec 0069, bug
// 2): a relative edit path is read against the payload's cwd, as a
// shell write is. It was read against the project root.
func TestEditPathsResolveAgainstCwd(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	sub := filepath.Join(r.Root, "sub")
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		assertRefused(t, r.hook(t, editPayload(tool, "../.claude/settings.json", sub)), "guardrail-settings")
		assertAllowed(t, r.hook(t, editPayload(tool, ".claude/settings.json", sub)))
	}
	assertRefused(t, r.hook(t, editPayload("Write", "../.claude/hooks/policy.json", sub)), "guard-policy-file")
}

// TestEditPathInGitBashForm is a regression test (spec 0069, bug 2): on
// Windows, an edit path spelled /d/… is read as D:/…, as a shell write's
// is.
func TestEditPathInGitBashForm(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the Git Bash drive form is read on Windows only")
	}
	r := newRepo(t, enforcePolicy)
	msys := "/" + strings.ToLower(r.Root[:1]) + filepath.ToSlash(r.Root[2:])
	assertRefused(t, r.hook(t, writePayload(msys+"/.claude/settings.json")), "guardrail-settings")
	assertRefused(t, r.hook(t, writePayload(msys+"/.claude/hooks/policy.json")), "guard-policy-file")
	assertExplainRefused(t, r, "Write", msys+"/.claude/settings.json", "guardrail-settings")
}

// TestPayloadCwdNotAString is a regression test (spec 0069, bug 5): a
// cwd that is not a JSON string was dropped without a sign. It is a
// payload error; a payload without cwd is decided as before.
func TestPayloadCwdNotAString(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	for _, cwd := range []string{`5`, `null`, `["/x"]`, `{"a": 1}`, `true`} {
		payload := `{"tool_name": "Write", "tool_input": {"file_path": "a.txt", "content": "x"}, "cwd": ` + cwd + `}`
		assertRefused(t, r.hook(t, payload), "payload-error")
	}
	assertAllowed(t, r.hook(t, `{"tool_name": "Write", "tool_input": {"file_path": "a.txt", "content": "x"}}`))
	assertAllowed(t, r.hook(t, `{"tool_name": "Write", "tool_input": {"file_path": "a.txt", "content": "x"}, "cwd": ""}`))
}
