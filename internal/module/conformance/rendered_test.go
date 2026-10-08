package conformance

import "github.com/Manual-debuger/VibeConform/internal/module/ci/pins"

// conformanceWorkflow and gitlabConformance are the two CI files as Resolve
// renders them with the current pins, which is what the template tests
// compare and parse (spec 0044).
var (
	conformanceWorkflow = must(pins.RenderCurrent(conformanceTemplate))
	gitlabConformance   = must(pins.RenderCurrent(gitlabConformanceTemplate))
)

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
