package githubpy

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
)

// TestCIWorkflowNeedsNoGo pins spec 0054: a repository without Go code
// installs no Go toolchain in CI. Task comes from go-task/setup-task,
// pinned by commit SHA, at the table's version and with its checksum
// checked, in every job that runs a task.
func TestCIWorkflowNeedsNoGo(t *testing.T) {
	wf := string(ciWorkflow)
	for _, gone := range []string{"setup-go", "go install", "GO_VERSION"} {
		if strings.Contains(wf, gone) {
			t.Errorf("ci.yml still names %q (spec 0054)", gone)
		}
	}

	pin := regexp.MustCompile(`uses: go-task/setup-task@[0-9a-f]{40} # v`)
	for _, job := range regexp.MustCompile(`(?m)^  [a-z_]+:$`).Split(wf, -1)[1:] {
		if !strings.Contains(job, "run: task ") {
			continue
		}
		if !pin.MatchString(job) {
			t.Errorf("a job runs task without a SHA-pinned go-task/setup-task:\n%s", job)
		}
		if !strings.Contains(job, `version: "`+pins.Current.TaskSetup()+`"`) {
			t.Errorf("a job's setup-task is not at Task %s:\n%s", pins.Current.TaskSetup(), job)
		}
		if !strings.Contains(job, `checksum: "`+pins.Current.TaskSHA256+`"`) {
			t.Errorf("a job's setup-task does not check Task's checksum:\n%s", job)
		}
	}
}
