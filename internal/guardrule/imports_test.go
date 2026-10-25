package guardrule_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guardrule is shared by the contract audit and vibe-guard, and imports
// no internal package at all (spec 0053, acceptance criterion 10).
func TestImportsNoInternalPackage(t *testing.T) {
	const internal = "github.com/Manual-debuger/VibeConform/internal/"
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(p, internal) {
				t.Errorf("%s imports %s", path, p)
			}
		}
	}
	if _, err := os.Stat("ids.go"); err != nil {
		t.Fatal("the test does not run in the package directory")
	}
}
