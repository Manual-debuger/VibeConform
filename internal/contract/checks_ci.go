package contract

// The static checks that read GitHub Actions workflows: C-01 to C-05,
// D-03 and R-01's static part (spec 0052, "Recognition rules"). Each
// starts from env.GitHubCI() and reads the workflow model in
// workflows.go; run: steps are parsed with ParseRunStep, and "called"
// is Called / CallsAny against env.Entrypoint(...).

func checkC01(_ *Env) PartResult { return notImplementedYet() }

func checkC02(_ *Env) PartResult { return notImplementedYet() }

func checkC03(_ *Env) PartResult { return notImplementedYet() }

func checkC04(_ *Env) PartResult { return notImplementedYet() }

func checkC05(_ *Env) PartResult { return notImplementedYet() }

func checkD03(_ *Env) PartResult { return notImplementedYet() }

func checkR01(_ *Env) PartResult { return notImplementedYet() }
