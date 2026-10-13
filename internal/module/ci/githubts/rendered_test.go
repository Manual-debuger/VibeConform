package githubts

import "github.com/Manual-debuger/VibeConform/internal/module/ci/pins"

// ciWorkflow is ci.yml as Resolve renders it with the current pins, which is
// what the template tests compare and parse (spec 0044).
var ciWorkflow = must(pins.RenderCurrent(ciTemplate))

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
