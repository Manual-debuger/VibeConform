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
// function with a doc comment, of one integer parameter, holding 41
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
	goVet(t, env.repo.rootDir(), d)
	fn := onlyFunc(t, d)
	if !fn.Name.IsExported() || fn.Doc == nil || !strings.HasPrefix(fn.Doc.Text(), fn.Name.Name+" ") {
		t.Errorf("func %s: want an exported function with a doc comment that starts with its name", fn.Name.Name)
	}
	if params := fn.Type.Params.List; len(params) != 1 || len(params[0].Names) != 1 || identName(params[0].Type) != "int" {
		t.Errorf("func %s: want one int parameter", fn.Name.Name)
	}
	ifs, returns, other := countStatements(t, fn.Body)
	if ifs != 41 || returns != 1 || other != 1 {
		t.Errorf("%d if statements, %d returns and %d other statements; want 41, 1 and 1 (the running total)", ifs, returns, other)
	}
}

// goVet writes the planted file into the module at root and runs go vet,
// which builds the package too.
func goVet(t *testing.T, root string, d plantedDefect) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(d.Path)), d.Content, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}

// onlyFunc parses the planted Go file, which must be in package foo and
// declare one function, and returns that function.
func onlyFunc(t *testing.T, d plantedDefect) *ast.FuncDecl {
	t.Helper()
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
	return funcs[0]
}

// countStatements counts a body's top-level if and return statements and
// the rest. Each if must hold one assignment, with no else and no init.
func countStatements(t *testing.T, body *ast.BlockStmt) (ifs, returns, other int) {
	t.Helper()
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			ifs++
			if s.Else != nil || s.Init != nil || len(s.Body.List) != 1 {
				t.Errorf("if at offset %d: want no else, no init and one statement", s.Pos())
			} else if _, ok := s.Body.List[0].(*ast.AssignStmt); !ok {
				t.Errorf("if at offset %d: want an assignment in its body, nothing nested", s.Pos())
			}
		case *ast.ReturnStmt:
			returns++
		default:
			other++
		}
	}
	return ifs, returns, other
}

func identName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// vibe audit --explain V-06 prints the complexity sample of every
// language and the threshold it implies. Complexity linters report a
// value above their limit, so the sample's cognitive complexity of 41 is
// what makes a limit of 40 catch it.
func TestV06ExplainListsEveryLanguage(t *testing.T) {
	t.Parallel()
	text := RuleDetails("V-06")
	for _, want := range []string{
		"Go, complexity", "Python, complexity", "TypeScript, complexity",
		"cyclomatic complexity is 42 and its cognitive complexity 41", "a limit of 40 or less",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("--explain V-06 lacks %q:\n%s", want, text)
		}
	}
}

// V-06's rule text states the sample's complexities and the largest
// limit that catches it, in the check and in its probe part.
func TestV06RuleTextStatesTheThreshold(t *testing.T) {
	t.Parallel()
	r, ok := LookupRule("V-06")
	if !ok {
		t.Fatal("no rule V-06")
	}
	texts := []string{r.Check()}
	for _, p := range r.Parts {
		texts = append(texts, p.Check)
	}
	for _, text := range texts {
		for _, want := range []string{"cyclomatic complexity 42 and cognitive complexity 41", "A complexity limit of 40 or less"} {
			if !strings.Contains(text, want) {
				t.Errorf("V-06 text lacks %q: %s", want, text)
			}
		}
	}
}

// The catcher a V-06 finding names states the same limit.
func TestV06CatcherStatesTheThreshold(t *testing.T) {
	t.Parallel()
	if got := defectCatchers[defectComplexity]; !strings.Contains(got, "a complexity limit of 40 or less") {
		t.Errorf("complexity catcher %q lacks the limit 40", got)
	}
}
