package contract_test

import (
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// TestEntrypointOrder: the required entrypoints in V-04's report order,
// then setup. Each call returns its own copy, so a caller that changes it
// changes no other caller's order.
func TestEntrypointOrder(t *testing.T) {
	t.Parallel()
	want := []string{"verify", "verify-fast", "fmt", "fmt-check", "setup"}
	got := contract.EntrypointOrder()
	if !slices.Equal(got, want) {
		t.Fatalf("EntrypointOrder() = %v, want %v", got, want)
	}
	got[0] = "changed"
	if again := contract.EntrypointOrder(); !slices.Equal(again, want) {
		t.Errorf("EntrypointOrder() after a caller changed its copy = %v, want %v", again, want)
	}
}
