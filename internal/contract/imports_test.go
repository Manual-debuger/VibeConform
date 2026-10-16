package contract

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The contract audit stands beside the old system and imports none of it,
// so that #112 can remove it (spec 0052, "Package and command").
func TestImportsNoOldPackage(t *testing.T) {
	old := []string{"reconcile", "state", "resource", "standard", "module", "manifest"}
	const internal = "github.com/Manual-debuger/VibeConform/internal/"

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			for _, o := range old {
				if p == internal+o || strings.HasPrefix(p, internal+o+"/") {
					t.Errorf("%s imports %s", path, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
