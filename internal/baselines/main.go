// Command baselines keeps the count of complexity baselines from going
// up (spec 0073). A baseline is a nolint directive whose reason is
// "baseline: pre-existing complexity", which a function that was over
// the gocyclo or gocognit limit when the limits were enabled carries.
// task lint runs it from the repository root after golangci-lint: it
// counts the baselines in the repository's Go files and fails when the
// count is higher than the ceiling below.
//
// It reads the Go files that go's ./... pattern reads, plus nested
// modules: a directory named testdata, or whose name starts with a dot
// or an underscore, is skipped, and so are .git and the agents'
// worktrees under .claude. It counts comments only, so that a string
// that holds the reason is not a baseline.
package main

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// ceiling is the number of complexity baselines this repository may
// hold. It only goes down: a change that removes a baseline lowers it in
// the same commit, and a new function over the limits is simplified,
// not given a baseline.
const ceiling = 58

// reason is the explanation a baseline's nolint directive gives.
const reason = "// baseline: pre-existing complexity"

func main() {
	if err := check(".", ceiling, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// check counts the baselines under root and fails when there are more
// than ceiling. When there are fewer, it passes and writes the ceiling
// to record to out.
func check(root string, ceiling int, out io.Writer) error {
	n, err := count(root)
	if err != nil {
		return err
	}
	switch {
	case n > ceiling:
		return fmt.Errorf("baselines: %d complexity baselines, and the ceiling is %d "+
			"(internal/baselines/main.go): simplify the function until gocyclo and "+
			"gocognit accept it; do not add a baseline", n, ceiling)
	case n < ceiling:
		_, err := fmt.Fprintf(out, "baselines: %d complexity baselines, below the ceiling of %d: "+
			"lower the ceiling to %d in internal/baselines/main.go\n", n, ceiling, n)
		return err
	}
	return nil
}

// count returns the number of baselines in the Go files under root.
func count(root string) (int, error) {
	n := 0
	fsys := os.DirFS(root)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name != "." && skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if path.Ext(name) != ".go" {
			return nil
		}
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		n += countIn(src)
		return nil
	})
	return n, err
}

// skipDir reports whether go's ./... pattern skips a directory of this
// name.
func skipDir(name string) bool {
	return name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// countIn returns the number of baselines in one Go file's source.
func countIn(src []byte) int {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)
	n := 0
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return n
		}
		if tok == token.COMMENT && strings.HasPrefix(lit, "//nolint:") && strings.HasSuffix(lit, reason) {
			n++
		}
	}
}
