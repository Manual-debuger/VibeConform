package contract

// The static checks that read hook managers, Claude Code settings, the
// guard policy and update bots: H-01, H-02, A-01, A-04, A-05's and
// A-07's static parts, A-06 and D-02 (spec 0052, "Recognition rules").
// Hook commands are parsed with ParseCommands, and "called" is Called /
// CallsAny against env.Entrypoint(...).

func checkH01(_ *Env) PartResult { return notImplementedYet() }

func checkH02(_ *Env) PartResult { return notImplementedYet() }

func checkA01(_ *Env) PartResult { return notImplementedYet() }

func checkA04(_ *Env) PartResult { return notImplementedYet() }

func checkA05(_ *Env) PartResult { return notImplementedYet() }

func checkA06(_ *Env) PartResult { return notImplementedYet() }

func checkA07(_ *Env) PartResult { return notImplementedYet() }

func checkD02(_ *Env) PartResult { return notImplementedYet() }

// guardListings returns what the report lists for review: the guard
// policy's exceptions (A-04) and the exceptions and extra rules of a
// tracked .claude/hooks/policy.local.json (spec 0050). It reads, and
// decides nothing.
func guardListings(_ *Env) ([]GuardException, []LocalAdjustment) { return nil, nil }
