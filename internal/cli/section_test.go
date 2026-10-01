package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// The test-sections standard exercises managed sections before any shipped
// module emits one: a core module owning section alpha of notes.txt, with
// a check, and two options — sec-b, a second section of the same file, and
// sec-c, the only section of other.txt.
const (
	sectionsCore = "standard: test-sections\nversion: v1\n"
	sectionsB    = "standard: test-sections\nversion: v1\nintegrations:\n  agents: [sec-b]\n"
	sectionsC    = "standard: test-sections\nversion: v1\nintegrations:\n  agents: [sec-c]\n"
)

// alphaContent is a variable so a test can move the standard on.
var alphaContent = "alpha rule\n"

type sectionModule struct {
	name, path, id string
	content        func() string
	at             resource.Placement
}

func (m sectionModule) Name() string { return m.name }

func (m sectionModule) Resolve(context.Context, *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{{
		Path: m.path, Ownership: resource.ManagedSection, SectionID: m.id,
		Content: []byte(m.content()), Placement: m.at,
	}}, nil
}

// checkedSectionModule adds a SectionChecker: "BAD" after the section is a
// conflict, "WARN" before it a warning.
type checkedSectionModule struct{ sectionModule }

func (checkedSectionModule) CheckSection(_ resource.Resource, before, after []byte) (conflicts, warnings []string) {
	if strings.Contains(string(after), "BAD") {
		conflicts = append(conflicts, "a BAD line follows the section")
	}
	if strings.Contains(string(before), "WARN") {
		warnings = append(warnings, "a WARN line precedes the section")
	}
	return conflicts, warnings
}

func init() {
	agents := standard.Group{Key: manifest.CategoryAgents}
	standard.Register(standard.Standard{
		Name: "test-sections", Version: "v1",
		Modules: []module.Module{checkedSectionModule{sectionModule{
			name: "alpha", path: "notes.txt", id: "alpha", content: func() string { return alphaContent },
		}}},
		Options: []standard.Option{
			{Group: agents, Name: "sec-b", Module: sectionModule{
				name: "beta", path: "notes.txt", id: "beta", content: func() string { return "beta rule\n" }, at: resource.Bottom,
			}},
			{Group: agents, Name: "sec-c", Module: sectionModule{
				name: "gamma", path: "other.txt", id: "gamma", content: func() string { return "gamma rule\n" },
			}},
		},
	})
}

const (
	alphaBlock = "# vibeconform:begin alpha\nalpha rule\n# vibeconform:end alpha\n"
	betaBlock  = "# vibeconform:begin beta\nbeta rule\n# vibeconform:end beta\n"
)

func sectionRepo(t *testing.T, manifest, notes string) string {
	t.Helper()
	dir := t.TempDir()
	writeVibeYAML(t, dir, manifest)
	if notes != "" {
		writeNotes(t, dir, notes)
	}
	return dir
}

func writeNotes(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantExit(t *testing.T, err error, code int, out string) {
	t.Helper()
	if got := ExitCode(err); got != code {
		t.Fatalf("exit %d (%v), want %d\n%s", got, err, code, out)
	}
}

func TestSectionCreatesFile(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "")
	if out := runDiffIn(t, dir); !strings.Contains(out, "notes.txt (section alpha): would add") {
		t.Errorf("diff:\n%s", out)
	}
	out := mustSync(t, dir)
	if !strings.Contains(out, "notes.txt (section alpha): created") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "notes.txt"); got != alphaBlock {
		t.Errorf("notes.txt = %q", got)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := state.SectionState{SHA256: hashHex([]byte("alpha rule\n")), Created: true}
	if got := s.Sections[state.SectionKey{Path: "notes.txt", ID: "alpha"}]; got != want {
		t.Errorf("state %+v, want %+v", got, want)
	}
	mustConform(t, dir)
	if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section alpha): unchanged") {
		t.Errorf("second sync:\n%s", out)
	}
}

func TestSectionInsertsAtTopOfUserFile(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "user line\r\n")
	if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section alpha): added") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "notes.txt"); got != alphaBlock+"\nuser line\r\n" {
		t.Errorf("notes.txt = %q", got)
	}
	s, _ := state.Load(dir)
	if s.Sections[state.SectionKey{Path: "notes.txt", ID: "alpha"}].Created {
		t.Error("recorded as created, but the user's file existed")
	}
	mustConform(t, dir)
}

// TestSectionUnrecorded: a section already there is adopted when its
// content is the target and is a conflict when it is not.
func TestSectionUnrecorded(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "x\n"+alphaBlock)
	if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section alpha): unchanged") {
		t.Errorf("identical section:\n%s", out)
	}

	dir = sectionRepo(t, sectionsCore, "# vibeconform:begin alpha\nmine\n# vibeconform:end alpha\n")
	out, err := runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "notes.txt (section alpha): conflict: manual changes detected") {
		t.Errorf("audit:\n%s", out)
	}
	out, err = runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, "notes.txt (section alpha): conflict:") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, "notes.txt"); !strings.Contains(got, "mine") {
		t.Errorf("a conflicting section was overwritten: %q", got)
	}
}

func TestSectionDriftIsRestoredAndOutsideIsKept(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "user line\n")
	mustSync(t, dir)
	writeNotes(t, dir, strings.Replace(readFile(t, dir, "notes.txt"), "alpha rule", "edited", 1)+"another user line\n")

	out, err := runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "notes.txt (section alpha): drifted") {
		t.Errorf("audit:\n%s", out)
	}
	if out := runDiffIn(t, dir); !strings.Contains(out, "would update (edited since last applied state)") {
		t.Errorf("diff:\n%s", out)
	}
	if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section alpha): updated") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, "notes.txt"); got != alphaBlock+"\nuser line\nanother user line\n" {
		t.Errorf("notes.txt = %q", got)
	}
}

func TestSectionOutOfDate(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "")
	mustSync(t, dir)
	alphaContent = "alpha rule, v2\n"
	t.Cleanup(func() { alphaContent = "alpha rule\n" })

	out, err := runAuditIn(t, dir)
	wantExit(t, err, 3, out)
	if !strings.Contains(out, "notes.txt (section alpha): out of date") {
		t.Errorf("audit:\n%s", out)
	}
	mustSync(t, dir)
	if got := readFile(t, dir, "notes.txt"); !strings.Contains(got, "alpha rule, v2\n") {
		t.Errorf("notes.txt = %q", got)
	}
}

func TestSectionMalformedMarkersAreAConflict(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "")
	mustSync(t, dir)
	broken := strings.Replace(readFile(t, dir, "notes.txt"), "# vibeconform:end alpha\n", "", 1)
	writeNotes(t, dir, broken)

	out, err := runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "conflict: begin marker for alpha at line 1 has no end marker") {
		t.Errorf("audit:\n%s", out)
	}
	if _, err := runSyncIn(t, dir); err == nil {
		t.Error("sync succeeded")
	}
	if got := readFile(t, dir, "notes.txt"); got != broken {
		t.Errorf("sync repaired a malformed section: %q", got)
	}
}

func TestTwoSectionsShareAFile(t *testing.T) {
	dir := sectionRepo(t, sectionsB, "user line\n")
	out := mustSync(t, dir)
	for _, want := range []string{"notes.txt (section alpha): added", "notes.txt (section beta): added"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync missing %q:\n%s", want, out)
		}
	}
	if got := readFile(t, dir, "notes.txt"); got != alphaBlock+"\nuser line\n\n"+betaBlock {
		t.Errorf("notes.txt = %q", got)
	}
	s, _ := state.Load(dir)
	if len(s.Sections) != 2 {
		t.Errorf("sections %v, want alpha and beta", s.Sections)
	}
	mustConform(t, dir)

	// beta breaks, and alpha's update waits with it.
	alphaContent = "alpha rule, v2\n"
	t.Cleanup(func() { alphaContent = "alpha rule\n" })
	broken := strings.Replace(readFile(t, dir, "notes.txt"), "# vibeconform:end beta\n", "", 1)
	writeNotes(t, dir, broken)
	if out := runDiffIn(t, dir); !strings.Contains(out, "notes.txt (section alpha): would update (standard moved since last applied state) (held: another section of this file conflicts)") {
		t.Errorf("diff:\n%s", out)
	}
	out, err := runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, "notes.txt (section alpha): not written: another section of this file conflicts") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, "notes.txt"); got != broken {
		t.Errorf("a conflicted file was written: %q", got)
	}
}

func TestSectionChecker(t *testing.T) {
	dir := sectionRepo(t, sectionsCore, "")
	mustSync(t, dir)
	writeNotes(t, dir, "WARN\n"+readFile(t, dir, "notes.txt"))
	stubHookInstall(t)
	out, errOut, err := runSyncCapturing(t, dir)
	if err != nil || !strings.Contains(errOut, "warning: notes.txt (section alpha): a WARN line precedes the section") {
		t.Errorf("sync: %v\n%s\n%s", err, out, errOut)
	}

	writeNotes(t, dir, readFile(t, dir, "notes.txt")+"BAD\n")
	out, err = runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, "notes.txt (section alpha): conflict: a BAD line follows the section") {
		t.Errorf("audit:\n%s", out)
	}
	if _, err := runSyncIn(t, dir); err == nil {
		t.Error("sync succeeded despite the check's conflict")
	}
}

func TestSectionPrune(t *testing.T) {
	t.Run("remove", func(t *testing.T) {
		dir := sectionRepo(t, sectionsB, "user line\n")
		mustSync(t, dir)
		writeVibeYAML(t, dir, sectionsCore)
		if out := runDiffIn(t, dir); !strings.Contains(out, "notes.txt (section beta): would remove (sec-b deselected)") {
			t.Errorf("diff:\n%s", out)
		}
		out, err := runAuditIn(t, dir)
		wantExit(t, err, 3, out)
		if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section beta): removed (sec-b deselected)") {
			t.Errorf("sync:\n%s", out)
		}
		if got := readFile(t, dir, "notes.txt"); got != alphaBlock+"\nuser line\n" {
			t.Errorf("notes.txt = %q", got)
		}
		s, _ := state.Load(dir)
		if _, ok := s.Sections[state.SectionKey{Path: "notes.txt", ID: "beta"}]; ok {
			t.Error("beta is still recorded")
		}
		mustConform(t, dir)
	})

	t.Run("modified", func(t *testing.T) {
		dir := sectionRepo(t, sectionsB, "")
		mustSync(t, dir)
		writeNotes(t, dir, strings.Replace(readFile(t, dir, "notes.txt"), "beta rule", "mine", 1))
		writeVibeYAML(t, dir, sectionsCore)
		out, err := runAuditIn(t, dir)
		wantExit(t, err, 2, out)
		out, err = runSyncIn(t, dir)
		if err == nil || !strings.Contains(out, "conflict: sec-b deselected but section modified since sync; kept") {
			t.Errorf("sync: %v\n%s", err, out)
		}
		if !strings.Contains(readFile(t, dir, "notes.txt"), "mine") {
			t.Error("a modified section was removed")
		}
	})

	t.Run("forget", func(t *testing.T) {
		dir := sectionRepo(t, sectionsB, "")
		mustSync(t, dir)
		writeNotes(t, dir, alphaBlock)
		writeVibeYAML(t, dir, sectionsCore)
		if out := mustSync(t, dir); !strings.Contains(out, "notes.txt (section beta): forgotten (sec-b deselected; already removed)") {
			t.Errorf("sync:\n%s", out)
		}
		mustConform(t, dir)
	})

	t.Run("created file left empty is deleted", func(t *testing.T) {
		dir := sectionRepo(t, sectionsC, "")
		mustSync(t, dir)
		writeVibeYAML(t, dir, sectionsCore)
		if out := mustSync(t, dir); !strings.Contains(out, "other.txt (section gamma): removed, and the file (sec-c deselected; nothing else was in it)") {
			t.Errorf("sync:\n%s", out)
		}
		if exists(t, dir, "other.txt") {
			t.Error("other.txt survived")
		}
	})

	t.Run("created file with user lines is kept", func(t *testing.T) {
		dir := sectionRepo(t, sectionsC, "")
		mustSync(t, dir)
		p := filepath.Join(dir, "other.txt")
		data, _ := os.ReadFile(p)
		if err := os.WriteFile(p, append(data, []byte("user line\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		writeVibeYAML(t, dir, sectionsCore)
		mustSync(t, dir)
		if got := readFile(t, dir, "other.txt"); got != "user line\n" {
			t.Errorf("other.txt = %q", got)
		}
	})
}

func TestSectionResourceValidation(t *testing.T) {
	for name, r := range map[string]resource.Resource{
		"no id":            {Path: "a", Ownership: resource.ManagedSection},
		"id on a file":     {Path: "a", Ownership: resource.Generated, SectionID: "x"},
		"bad id":           {Path: "a", Ownership: resource.ManagedSection, SectionID: "X y"},
		"no final newline": {Path: "a", Ownership: resource.ManagedSection, SectionID: "x", Content: []byte("rule")},
		"marker inside":    {Path: "a", Ownership: resource.ManagedSection, SectionID: "x", Content: []byte("# vibeconform:end x\n")},
	} {
		if err := checkSection(r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
