package reconcile

import "testing"

func TestDecideRemoval(t *testing.T) {
	same, other := "aaa", "bbb"
	for _, tc := range []struct {
		current *string
		want    Removal
	}{
		{nil, Forget},
		{&same, Remove},
		{&other, RemoveConflict},
	} {
		if got := DecideRemoval("aaa", tc.current); got != tc.want {
			t.Errorf("DecideRemoval(aaa, %v) = %v, want %v", tc.current, got, tc.want)
		}
	}
}
