package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxComplexity is the fixture's cyclomatic complexity limit, the setup
// skill's reference value for gocyclo.
const maxComplexity = 15

// complexity lists the functions whose cyclomatic complexity is over
// maxComplexity, and fails when it lists one. It stands in for gocyclo,
// so that the fixture needs only Go (spec 0061, V-06). It skips ci/, the
// fixture's tooling, whose main is over the limit.
func complexity() int {
	code := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == "ci" || path != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if c := cyclomatic(fn.Body); c > maxComplexity {
				fmt.Printf("%s: func %s has cyclomatic complexity %d (> %d)\n", filepath.ToSlash(path), fn.Name.Name, c, maxComplexity)
				code = 1
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// cyclomatic is 1 plus the number of decision points in body.
func cyclomatic(body *ast.BlockStmt) int {
	c := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if n.List != nil {
				c++
			}
		case *ast.CommClause:
			if n.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}
