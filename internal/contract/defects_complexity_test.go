package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Go complexity sample of V-06 (spec 0061, "V-06, complexity") is a
// non-test file that builds and passes go vet beside the component's
// code, so that only a complexity limit can catch it. It is one exported
// function with a doc comment, of one integer parameter, holding 40
// consecutive unnested if statements and one return statement.
func TestGoComplexitySampleBuildsAndPassesVet(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	env, comp := goTestRepo(t)
	d, err := defectFor(env, comp, defectComplexity)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(d.Path, "_test.go") || !strings.HasPrefix(d.Path, "foo/") {
		t.Fatalf("planted at %s, want a non-test file in foo/", d.Path)
	}
	p := filepath.Join(env.Repo.Root(), filepath.FromSlash(d.Path))
	if err := os.WriteFile(p, d.Content, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = env.Repo.Root()
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}

	f, err := parser.ParseFile(token.NewFileSet(), d.Path, d.Content, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name.Name != "foo" {
		t.Errorf("package %s, want foo", f.Name.Name)
	}
	var funcs []*ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			funcs = append(funcs, fd)
		}
	}
	if len(funcs) != 1 {
		t.Fatalf("%d functions, want 1", len(funcs))
	}
	fn := funcs[0]
	if !fn.Name.IsExported() || fn.Doc == nil || !strings.HasPrefix(fn.Doc.Text(), fn.Name.Name+" ") {
		t.Errorf("func %s: want an exported function with a doc comment that starts with its name", fn.Name.Name)
	}
	if params := fn.Type.Params.List; len(params) != 1 || len(params[0].Names) != 1 || types(params[0].Type) != "int" {
		t.Errorf("func %s: want one int parameter", fn.Name.Name)
	}
	ifs, returns, other := 0, 0, 0
	for _, stmt := range fn.Body.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			ifs++
			if s.Else != nil || s.Init != nil || len(s.Body.List) != 1 {
				t.Errorf("if at offset %d: want no else, no init and one statement", s.Pos())
			}
			if _, nested := s.Body.List[0].(*ast.AssignStmt); !nested {
				t.Errorf("if at offset %d: want an assignment in its body, nothing nested", s.Pos())
			}
		case *ast.ReturnStmt:
			returns++
		default:
			other++
		}
	}
	if ifs != 40 || returns != 1 {
		t.Errorf("%d if statements and %d returns, want 40 and 1", ifs, returns)
	}
	if other != 1 {
		t.Errorf("%d other statements, want 1 (the running total)", other)
	}
}

func types(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// vibe audit --explain V-06 prints the complexity sample of every
// language and the threshold it implies.
func TestV06ExplainListsEveryLanguage(t *testing.T) {
	t.Parallel()
	text := RuleDetails("V-06")
	for _, want := range []string{"Go, complexity", "Python, complexity", "TypeScript, complexity", "40"} {
		if !strings.Contains(text, want) {
			t.Errorf("--explain V-06 lacks %q:\n%s", want, text)
		}
	}
}
