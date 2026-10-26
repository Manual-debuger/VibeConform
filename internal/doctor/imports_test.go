package doctor

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// doctor reads the repository with the contract audit's readers and
// imports no other package of this module (spec 0059, "vibe doctor reuses
// the audit's readers").
func TestImportsOnlyContract(t *testing.T) {
	const module = "github.com/Manual-debuger/VibeConform/"
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(p, module) && p != module+"internal/contract" {
				t.Errorf("%s imports %s", f, p)
			}
		}
	}
}
