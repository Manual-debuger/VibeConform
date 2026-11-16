// Package lib is a module inside the fixture, so that go mod tidy needs
// no download.
package lib

// Answer returns the answer.
func Answer() int { return 42 }
