package tstooling

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestESLintGitignore runs real ESLint with the template under test, not
// the synced copy (spec 0045). It is the regression test for issue #87: a
// file under a directory that the .gitignore next to the config ignores,
// such as an agent worktree, must not be linted.
//
// Each case lints its own fixture directory under examples/typescript's
// node_modules/.cache, where the config's imports still resolve but Git,
// Prettier and the example's own ESLint never look. Fixtures in the
// example's working tree raced with the agent hooks' lint runs. It skips
// where the example has no node_modules; the examples/typescript job in
// examples.yml runs it.
func TestESLintGitignore(t *testing.T) {
	example, eslint := eslintExample(t)

	t.Run("gitignored directory is not linted", func(t *testing.T) {
		dir := fixtureDir(t, example)
		writeFile(t, filepath.Join(dir, ".gitignore"), ".claude/worktrees/\n")
		ignored := writeFile(t, filepath.Join(dir, ".claude", "worktrees", "x", "bad.mjs"), unusedVar)
		control := writeFile(t, filepath.Join(dir, "control.mjs"), unusedVar)
		config := writeFile(t, filepath.Join(dir, "eslint.config.js"), string(eslintConfig))

		reported := runESLint(t, eslint, dir, config)
		if !reported[key(control)] {
			t.Fatalf("control file %s was not reported; ESLint did not lint it, so the test proves nothing (reported: %v)", control, reported)
		}
		if reported[key(ignored)] {
			t.Errorf("%s is under a gitignored directory and was linted", ignored)
		}
	})

	t.Run("no .gitignore changes nothing", func(t *testing.T) {
		dir := fixtureDir(t, example)
		bad := writeFile(t, filepath.Join(dir, ".claude", "worktrees", "x", "bad.mjs"), unusedVar)
		config := writeFile(t, filepath.Join(dir, "eslint.config.js"), string(eslintConfig))

		if reported := runESLint(t, eslint, dir, config); !reported[key(bad)] {
			t.Errorf("without a .gitignore, %s should be linted as before (reported: %v)", bad, reported)
		}
	})
}

// unusedVar fails js.configs.recommended's no-unused-vars. Fixtures are
// .mjs files so the result does not depend on the TypeScript project
// service.
const unusedVar = "const unused = 1\n"

// eslintCmd is how to run the example's installed ESLint.
type eslintCmd struct{ node, bin string }

// eslintExample returns examples/typescript's absolute path and its ESLint,
// or skips the test when node or the installed ESLint is missing.
func eslintExample(t *testing.T) (string, eslintCmd) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; the examples/typescript job runs this test in CI")
	}
	example, err := filepath.Abs(filepath.Join("..", "..", "..", "examples", "typescript"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(example, "node_modules", "eslint", "bin", "eslint.js")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("examples/typescript has no node_modules (pnpm install); the examples/typescript job runs this test in CI")
	}
	return example, eslintCmd{node: node, bin: bin}
}

// fixtureDir returns a fresh directory under the example's
// node_modules/.cache, removed when the test ends.
func fixtureDir(t *testing.T, example string) string {
	t.Helper()
	cache := filepath.Join(example, "node_modules", ".cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(cache, "vibe-eslint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// writeFile writes content to path, creating its directory, and returns
// path.
func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runESLint lints dir with config and returns the files that have at least
// one problem.
func runESLint(t *testing.T, eslint eslintCmd, dir, config string) map[string]bool {
	t.Helper()
	reported := map[string]bool{}
	for file, rules := range runESLintRules(t, eslint, dir, config) {
		if len(rules) > 0 {
			reported[file] = true
		}
	}
	return reported
}

// runESLintRules lints dir with config and returns, per file with a
// problem, the rule ids it was reported for. A problem without a rule id,
// such as a parse error, is recorded as "". Exit 1 (problems found) is
// expected; anything else, such as a config that fails to load, fails the
// test.
func runESLintRules(t *testing.T, eslint eslintCmd, dir, config string) map[string][]string {
	t.Helper()
	cmd := exec.Command(eslint.node, eslint.bin, ".", "--no-cache", "--config", config, "--format", "json")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		t.Fatalf("eslint: %v\nstderr:\n%s", err, stderr.String())
	}

	var results []struct {
		FilePath string `json:"filePath"`
		Messages []struct {
			RuleID *string `json:"ruleId"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("eslint JSON: %v\n%s", err, stdout.String())
	}
	rules := map[string][]string{}
	for _, r := range results {
		for _, m := range r.Messages {
			id := ""
			if m.RuleID != nil {
				id = *m.RuleID
			}
			rules[key(r.FilePath)] = append(rules[key(r.FilePath)], id)
		}
	}
	return rules
}

// key makes a path from ESLint comparable with one from Go: on Windows the
// two may disagree on the drive letter's case.
func key(path string) string {
	path = filepath.Clean(path)
	if filepath.Separator == '\\' {
		return strings.ToLower(path)
	}
	return path
}
