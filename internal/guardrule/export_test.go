package guardrule

// MatchParses is how many times a command match has been parsed, for
// the test that a match is parsed once per shell.
func MatchParses() int64 { return matchParses.Load() }

// GlobCompiles is how many times a path glob has been compiled.
func GlobCompiles() int64 { return globCompiles.Load() }
