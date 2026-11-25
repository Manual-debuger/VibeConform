package guard

import gr "github.com/Manual-debuger/VibeConform/internal/guardrule"

// Repo is the repository a tool call acts in. guardrule.FindRepo finds
// it by reading files, for the guard and for vibe unlock alike (spec
// 0069).
type Repo = gr.Repo
