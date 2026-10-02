package doctor

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

var testGraph = Graph{Name: "graphify", File: "graphify-out/graph.json", Commit: "built_at_commit", Rebuild: "task graph:update"}

// exitCode is an error carrying an exit status, as *exec.ExitError does.
type exitCode int

func (e exitCode) Error() string { return "exit status " + strconv.Itoa(int(e)) }
func (e exitCode) ExitCode() int { return int(e) }

const head = "b34c33e0000000000000000000000000000000000"

// graphFake is a repository at head whose graph file holds content (none
// when content is empty), ignored by git when ignored is set.
func graphFake(content string, ignored bool, dirty string) *fake {
	f := &fake{
		files: map[string]string{},
		commands: map[string]reply{
			"git rev-parse HEAD":                          {stdout: head + "\n"},
			"git status --porcelain --untracked-files=no": {stdout: dirty},
			"git check-ignore -q graphify-out/graph.json": {err: exitCode(1)},
		},
	}
	if content != "" {
		f.files["repo/graphify-out/graph.json"] = content
	}
	if ignored {
		f.commands["git check-ignore -q graphify-out/graph.json"] = reply{}
	}
	return f
}

// TestGraphChecks covers every status spec 0035 §3 lists.
func TestGraphChecks(t *testing.T) {
	big := `{"directed":false,"nodes":[{"id":"a","attrs":{"x":[1,2,{"y":"}"}]}}],"links":[],"built_at_commit":"`
	cases := []struct {
		name    string
		fake    *fake
		graph   Status
		detail  string
		ignored Status
	}{
		{"absent", graphFake("", true, ""), Warn, "no graph (graphify-out/graph.json absent); run task graph:update", Pass},
		{"not json", graphFake("<html>", true, ""), Warn, "graphify-out/graph.json unreadable", Pass},
		{"not an object", graphFake(`[1,2]`, true, ""), Warn, "unreadable (not a JSON object)", Pass},
		{"truncated", graphFake(`{"nodes":[{"id":`, true, ""), Warn, "unreadable", Pass},
		{"no commit", graphFake(`{"nodes":[],"links":[]}`, true, ""), Unverified, "freshness unknown: graphify-out/graph.json records no built_at_commit", Pass},
		{"stale", graphFake(big+`785b8050000"}`, true, ""), Warn, "stale: built at 785b805, HEAD b34c33e; run task graph:update", Pass},
		{"current", graphFake(big+head+`"}`, true, ""), Pass, "current: built at HEAD b34c33e", Pass},
		{"current dirty", graphFake(big+head+`"}`, true, " M main.go\n"), Pass, "current: built at HEAD b34c33e; uncommitted changes are not in the graph", Pass},
		{"not ignored", graphFake(big+head+`"}`, false, ""), Pass, "current", Warn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphChecks(context.Background(), tc.fake.env(), "repo", true, testGraph)
			if len(got) != 2 {
				t.Fatalf("got %d results, want 2: %+v", len(got), got)
			}
			if got[0].Name != "graphify graph" || got[0].Status != tc.graph || !strings.Contains(got[0].Detail, tc.detail) {
				t.Errorf("graph = %v %q, want %v containing %q", got[0].Status, got[0].Detail, tc.graph, tc.detail)
			}
			if got[1].Name != "graphify ignore" || got[1].Status != tc.ignored {
				t.Errorf("ignore = %v %q, want %v", got[1].Status, got[1].Detail, tc.ignored)
			}
			for _, r := range got {
				if r.Status == Fail {
					t.Errorf("%s failed; a graph problem is never a FAIL (spec 0035 §3)", r.Name)
				}
			}
		})
	}
}

func TestGraphChecksNeedGit(t *testing.T) {
	f := graphFake("", true, "")
	for _, r := range GraphChecks(context.Background(), f.env(), "repo", false, testGraph) {
		if r.Status != Unverified || r.Detail != "needs git" {
			t.Errorf("%s = %v %q, want UNVERIFIED needs git", r.Name, r.Status, r.Detail)
		}
	}
	if len(f.ran) != 0 {
		t.Errorf("ran %v without git", f.ran)
	}
}

func TestGraphChecksIgnoreProbeFails(t *testing.T) {
	f := graphFake("", true, "")
	f.commands["git check-ignore -q graphify-out/graph.json"] = reply{stderr: "fatal: broken", err: exitCode(128)}
	got := GraphChecks(context.Background(), f.env(), "repo", true, testGraph)
	if got[1].Status != Unverified || !strings.Contains(got[1].Detail, "fatal: broken") {
		t.Errorf("ignore = %v %q, want UNVERIFIED with the git error", got[1].Status, got[1].Detail)
	}
}

func TestGraphChecksNoHead(t *testing.T) {
	f := graphFake(`{"built_at_commit":"abc"}`, true, "")
	f.commands["git rev-parse HEAD"] = reply{stderr: "fatal: ambiguous argument 'HEAD'", err: errors.New("exit status 128")}
	got := GraphChecks(context.Background(), f.env(), "repo", true, testGraph)
	if got[0].Status != Unverified || !strings.Contains(got[0].Detail, "ambiguous argument") {
		t.Errorf("graph = %v %q, want UNVERIFIED naming the git error", got[0].Status, got[0].Detail)
	}
}

// TestOptionalToolIsAWarning: a missing optional tool degrades a
// capability, so it warns with an install hint instead of failing.
func TestOptionalToolIsAWarning(t *testing.T) {
	f := &fake{}
	got := Tools(context.Background(), f.env(), "repo", []Tool{{Name: "graphify", Module: "graphify", Why: "graph updates", Optional: true, Install: "uv tool install graphifyy"}})
	if got[0].Status != Warn || !strings.Contains(got[0].Detail, "optional") || !strings.Contains(got[0].Detail, "install: uv tool install graphifyy") {
		t.Errorf("got %v %q, want WARN naming it optional with the install hint", got[0].Status, got[0].Detail)
	}
}
