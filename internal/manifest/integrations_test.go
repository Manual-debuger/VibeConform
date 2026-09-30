package manifest

import (
	"slices"
	"strings"
	"testing"
)

const header = "standard: prod-go\nversion: v1\n"

// TestIntegrationsAbsentAndEmptyDiffer pins the one distinction the
// schema exists for: an absent category takes the standard's defaults,
// an empty one selects none (docs/specs/0026-optional-integrations.md).
func TestIntegrationsAbsentAndEmptyDiffer(t *testing.T) {
	m, err := Parse([]byte(header))
	if err != nil {
		t.Fatal(err)
	}
	if m.Integrations != nil {
		t.Fatalf("no integrations: key, got %+v", m.Integrations)
	}
	for _, c := range Categories {
		if m.Integrations.Get(c) != nil {
			t.Errorf("nil Integrations: %s is not absent", c)
		}
	}

	m, err = Parse([]byte(header + "integrations:\n  agents: []\n  editors: [vscode, zed]\n"))
	if err != nil {
		t.Fatal(err)
	}
	agents := m.Integrations.Get(CategoryAgents)
	if agents == nil || len(*agents) != 0 {
		t.Errorf("agents: [] parsed as %v, want a present, empty list", agents)
	}
	if got := m.Integrations.Get(CategoryEditors); got == nil || !slices.Equal(*got, []string{"vscode", "zed"}) {
		t.Errorf("editors parsed as %v", got)
	}
	if m.Integrations.Get(CategoryIntelligence) != nil {
		t.Error("intelligence: absent parsed as present")
	}
}

func TestIntegrationsRejected(t *testing.T) {
	for name, tc := range map[string]struct{ yaml, want string }{
		"unknown category": {"integrations:\n  editor: [zed]\n", "editor"},
		"duplicate":        {"integrations:\n  editors: [vscode, vscode]\n", "integrations.editors[1] (vscode): duplicate name"},
		"bad name":         {"integrations:\n  agents: [Claude]\n", "integrations.agents[0] (Claude): name must match"},
		"not a list":       {"integrations:\n  agents: claude\n", "into []string"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(header + tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
