package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guard"
	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
)

// testVibeVersion is the vibe version the tests resolve with.
const testVibeVersion = "v1.2.3"

func agentGuard(v string) *module.Context {
	return &module.Context{Integrations: []string{"claude"}, Policies: map[string]string{manifest.PolicyAgentGuard: v},
		VibeVersion: testVibeVersion}
}

// TestVibeGuardInstallVersion: the settings' install hint names the
// version internal/guard's version rule picks for the running vibe
// (spec 0054, "The install hint"; the rule's cases are in
// internal/guard).
func TestVibeGuardInstallVersion(t *testing.T) {
	for vibeVersion, want := range map[string]string{
		testVibeVersion: "go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@" + testVibeVersion + ", ",
		"dev":           "go install github.com/Manual-debuger/VibeConform/cmd/vibe-guard@latest, ",
	} {
		mctx := agentGuard(module.AgentGuardVibeGuard)
		mctx.VibeVersion = vibeVersion
		got := resolveContents(t, mctx)[SettingsPath]
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("vibe %s: settings.json does not name %s:\n%s", vibeVersion, want, got)
		}
	}
}

func resolveContents(t *testing.T, mctx *module.Context) map[string][]byte {
	t.Helper()
	rs, err := New().Resolve(context.Background(), mctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	out := map[string][]byte{}
	for _, r := range rs {
		out[r.Path] = r.Content
	}
	return out
}

// TestAgentGuardRuntimeUnchanged: an absent policy.agent_guard and an
// explicit runtime render the runtime guard's settings and policy, byte
// for byte (spec 0053, "This repository").
func TestAgentGuardRuntimeUnchanged(t *testing.T) {
	policyJSON, err := renderPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	for name, mctx := range map[string]*module.Context{
		"nil":     nil,
		"absent":  {Integrations: []string{"claude"}, Policies: map[string]string{}},
		"runtime": agentGuard(module.AgentGuardRuntime),
	} {
		got := resolveContents(t, mctx)
		if !bytes.Equal(got[SettingsPath], settings) {
			t.Errorf("%s: settings.json is not the template", name)
		}
		if !bytes.Equal(got[".claude/hooks/policy.json"], policyJSON) {
			t.Errorf("%s: policy.json is not the runtime guard's policy", name)
		}
	}
	if len(New().(claudeModule).GuardedFiles(nil)) != 1 {
		t.Error("the runtime guard no longer checks policy.local.json")
	}
}

// TestAgentGuardVibeGuard: with policy.agent_guard vibe-guard the
// settings run internal/guard's reference commands, every other hook is
// the template's, and policy.json is an empty version 2 policy that
// vibe-guard accepts.
func TestAgentGuardVibeGuard(t *testing.T) {
	got := resolveContents(t, agentGuard(module.AgentGuardVibeGuard))

	type hook struct {
		Type        string `json:"type"`
		Command     string `json:"command"`
		Timeout     int    `json:"timeout,omitempty"`
		AsyncRewake bool   `json:"asyncRewake,omitempty"`
	}
	type entry struct {
		Matcher string `json:"matcher,omitempty"`
		Hooks   []hook `json:"hooks"`
	}
	parse := func(b []byte) map[string][]entry {
		var cfg struct {
			Hooks map[string][]entry `json:"hooks"`
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			t.Fatalf("parsing settings: %v", err)
		}
		return cfg.Hooks
	}
	vg, rt := parse(got[SettingsPath]), parse(settings)

	pre := vg["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != guard.ReferencePreToolUseMatcher || len(pre[0].Hooks) != 1 ||
		pre[0].Hooks[0].Command != guard.ReferencePreToolUse(testVibeVersion) {
		t.Errorf("PreToolUse = %+v, want the reference command and matcher", pre)
	}
	stop := vg["Stop"]
	if len(stop) != 1 || len(stop[0].Hooks) != 1 || stop[0].Hooks[0].Command != guard.ReferenceStop ||
		stop[0].Hooks[0].Timeout != rt["Stop"][0].Hooks[0].Timeout {
		t.Errorf("Stop = %+v, want the reference command with the template's timeout", stop)
	}
	for event, entries := range rt {
		if event == "PreToolUse" || event == "Stop" {
			continue
		}
		if !reflect.DeepEqual(vg[event], entries) {
			t.Errorf("%s hooks differ from the template", event)
		}
	}
	if len(vg) != len(rt) {
		t.Errorf("events %d, template %d", len(vg), len(rt))
	}
	if bytes.Contains(got[SettingsPath], []byte("hook:guard")) {
		t.Error("settings.json still runs hook:guard")
	}
	if bytes.Contains(got[SettingsPath], []byte(`\`+`u00`)) {
		t.Error("settings.json HTML-escapes the shell's > or &")
	}

	pol, err := guardrule.ParsePolicy(got[".claude/hooks/policy.json"])
	if err != nil {
		t.Fatalf("vibe-guard rejects the generated policy.json: %v", err)
	}
	if pol.Version != 2 || pol.Mode != guardrule.Enforce || len(pol.Rules) != 0 || len(pol.Exceptions) != 0 {
		t.Errorf("policy = %+v, want version 2, enforce, no rules, no exceptions", pol)
	}
	if !strings.HasSuffix(string(got[".claude/hooks/policy.json"]), "}\n") {
		t.Error("policy.json does not end in a newline")
	}
	if New().(claudeModule).GuardedFiles(agentGuard(module.AgentGuardVibeGuard)) != nil {
		t.Error("vibe-guard does not read policy.local.json, yet it is still checked")
	}
}
