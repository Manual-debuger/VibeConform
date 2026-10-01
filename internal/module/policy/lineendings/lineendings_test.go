package lineendings

import (
	"context"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func section(t *testing.T) resource.Resource {
	t.Helper()
	rs, err := New().Resolve(context.Background(), nil)
	if err != nil || len(rs) != 1 {
		t.Fatalf("Resolve = %v, %v", rs, err)
	}
	return rs[0]
}

func TestResolve(t *testing.T) {
	r := section(t)
	if r.Path != ".gitattributes" || r.Ownership != resource.ManagedSection || r.SectionID != "line-endings" ||
		r.Markers != resource.HashComment || r.Placement != resource.Top {
		t.Errorf("resource %+v", r)
	}
	if want := "# Managed by VibeConform: policy.line_endings in vibe.yaml.\n* text=auto eol=lf\n"; string(r.Content) != want {
		t.Errorf("content %q", r.Content)
	}
	if _, ok := New().(module.SectionChecker); !ok {
		t.Error("the module does not check its section")
	}
}

// TestCheckSection is spec 0029 §4's table: what follows the section may
// narrow it, never undo it everywhere; what precedes it is overridden.
func TestCheckSection(t *testing.T) {
	r := section(t)
	check := New().(module.SectionChecker)
	for name, tc := range map[string]struct {
		before, after       string
		conflicts, warnings []string
	}{
		"nothing else":     {"", "", nil, nil},
		"narrow below":     {"", "\n*.bat eol=crlf\n*.png binary\ndocs/** -text\n", nil, nil},
		"same rule below":  {"", "\n* text=auto eol=lf\n", nil, nil},
		"comments, macros": {"", "\n# * eol=crlf\n[attr]mine -text\n\n", nil, nil},
		"eol crlf below": {"", "\n*.png binary\n* eol=crlf\n", []string{
			`line 7: "* eol=crlf" overrides policy.line_endings (eol=crlf) for every path`,
		}, nil},
		"-text below":    {"", "\n* -text\n", []string{`line 6: "* -text" overrides policy.line_endings (-text)`}, nil},
		"binary below":   {"", "\n/** binary\n", []string{`line 6: "/** binary" overrides policy.line_endings (binary)`}, nil},
		"plain text":     {"", "\n* text\n", []string{`"* text"`}, nil},
		"unset eol":      {"", "\n* !eol\n", []string{`"* !eol"`}, nil},
		"legacy crlf":    {"", "\n* -crlf\n", []string{`"* -crlf"`}, nil},
		"quoted pattern": {"", "\n\"*\" eol=crlf\n", []string{`"\"*\" eol=crlf"`}, nil},
		"CRLF lines":     {"", "\r\n* eol=crlf\r\n", []string{`line 6: "* eol=crlf"`}, nil},
		"rule above": {"*.bat eol=crlf\n", "", nil, []string{
			`line 1: "*.bat eol=crlf" comes before the policy's section, which overrides its eol=crlf for *.bat; move it below the section`,
		}},
		"same rule above": {"* text=auto eol=lf\n", "", nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			conflicts, warnings := check.CheckSection(r, []byte(tc.before), []byte(tc.after))
			matches(t, "conflicts", conflicts, tc.conflicts)
			matches(t, "warnings", warnings, tc.warnings)
		})
	}
}

// matches checks that got has one entry per want, each containing it.
func matches(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %q, want %d matching %q", what, got, len(want), want)
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("%s[%d] = %q, want it to contain %q", what, i, got[i], want[i])
		}
	}
}
