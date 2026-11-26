package contract

import (
	"context"
	"testing"
)

// A check reads the contract version the audit applies from Env, which
// is the declared version's MAJOR as this vibe implements it (spec 0072,
// "Contract versions").
func TestEnvAuditedContract(t *testing.T) {
	t.Parallel()
	for declared, want := range map[string]string{"1.0.0": "1.2.0", "1.2.5": "1.2.0", "2.0.0": "2.0.0", "2.0.3": "2.0.0"} {
		t.Run(declared, func(t *testing.T) {
			t.Parallel()
			repo := NewRepo(t.TempDir(), []string{"go.mod"})
			cfg, err := Parse("vibe.yaml", []byte("contract: "+declared+"\nplatforms: [linux]\nagents: []\n"), repo)
			if err != nil {
				t.Fatal(err)
			}
			if got := newEnv(context.Background(), cfg, repo).auditedContract(); got != want {
				t.Errorf("auditedContract() = %q, want %q", got, want)
			}
		})
	}
}
