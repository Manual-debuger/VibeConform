package guardrule

// MatchParses is how many times a command match has been parsed, for
// the test that a match is parsed once per shell.
func MatchParses() int64 { return matchParses.Load() }

// GlobCompiles is how many times a path glob has been compiled.
func GlobCompiles() int64 { return globCompiles.Load() }

// OtherTarget is the target ParseTarget gives a call of tool, a tool
// that is neither a command tool nor a file-edit tool.
func OtherTarget(tool string) Target { return Target{tool: tool} }
