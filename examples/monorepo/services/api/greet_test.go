package api

import "testing"

func TestGreet(t *testing.T) {
	if got := Greet("World"); got != "Hello, World!" {
		t.Fatalf("Greet(%q) = %q", "World", got)
	}
}
