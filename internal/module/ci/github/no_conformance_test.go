package github

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCIWorkflowCarriesNoConformance pins spec 0022: VibeConform's own check
// lives in .github/workflows/conformance.yml, owned by the vibe-conformance
// module, so removing VibeConform deletes that file instead of editing this
// one in three places. Any mention of vibe here — a job, a gate needs entry,
// an install step — would bring that editing back.
func TestCIWorkflowCarriesNoConformance(t *testing.T) {
	if strings.Contains(string(ciWorkflow), "vibe") || strings.Contains(string(ciWorkflow), "conformance") {
		t.Error("ci.yml mentions vibe or conformance; both belong in conformance.yml (spec 0022)")
	}

	var wf struct {
		Jobs map[string]struct {
			Needs []string `yaml:"needs"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(ciWorkflow, &wf); err != nil {
		t.Fatalf("parsing ci.yml: %v", err)
	}
	gate, ok := wf.Jobs["gate"]
	if !ok {
		t.Fatal("ci.yml has no gate job")
	}
	for _, need := range gate.Needs {
		if _, ok := wf.Jobs[need]; !ok {
			t.Errorf("gate needs %q, which is not a job in ci.yml", need)
		}
	}
	for name := range wf.Jobs {
		if name == "gate" {
			continue
		}
		found := false
		for _, need := range gate.Needs {
			found = found || need == name
		}
		if !found {
			t.Errorf("gate does not need job %q, so its failure would not fail CI", name)
		}
		if !strings.Contains(string(ciWorkflow), "needs."+name+".result") {
			t.Errorf("gate never checks needs.%s.result", name)
		}
	}
}
