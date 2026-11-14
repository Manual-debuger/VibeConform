// Command ci is the probe fixture's tooling: its entrypoints and hooks
// run go run ./ci <step>, so that the fixture needs only Go (spec 0053,
// "Testing Decisions").
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./ci <step>")
		os.Exit(2)
	}
	switch step := os.Args[1]; step {
	case "fmt":
		os.Exit(formatFiles(true))
	case "fmt-check":
		os.Exit(formatFiles(false))
	case "typecheck":
		os.Exit(run("go", "vet", "./..."))
	case "vet-nolint":
		// A linter that honors //nolint, as golangci-lint does: go vet's
		// findings on a line holding //nolint do not count (V-08).
		os.Exit(vetNolint())
	case "nolintlint":
		// Refuses a //nolint that names no linter, as golangci-lint's
		// nolintlint does with require-specific (V-08).
		os.Exit(nolintlint())
	case "test":
		// go test runs part of go vet by itself, printf included; the
		// fixture turns that off so that typecheck alone catches the
		// static-analysis defect and a verify without it misses it.
		os.Exit(run("go", "test", "-vet=off", "./..."))
	case "pytypecheck":
		os.Exit(pyTypeCheck())
	case "ok":
		fmt.Println("ok")
	case "fail":
		fmt.Println("failing on purpose")
		os.Exit(1)
	case "sleep":
		time.Sleep(10 * time.Minute)
	case "call-vibe":
		os.Exit(run("vibe", "version"))
	case "stop":
		os.Exit(stop(true))
	case "stop-always":
		os.Exit(stop(false))
	case "session-start":
		fmt.Println("facts: this is the probe fixture")
	case "session-start-write":
		if err := os.WriteFile("session.log", []byte("started\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown step %q\n", step)
		os.Exit(2)
	}
}

// formatFiles lists the Go files gofmt would change, or rewrites them
// when write is true. Listing a file fails the step.
func formatFiles(write bool) int {
	code := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := format.Source(src)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if bytes.Equal(src, out) {
			return nil
		}
		if write {
			return os.WriteFile(path, out, 0o644)
		}
		fmt.Println(filepath.ToSlash(path))
		code = 1
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// pyTypeCheck stands in for a Python type checker (V-05): it lists the
// .py files where a function annotated -> int returns a string literal.
// Listing a file fails the step.
func pyTypeCheck() int {
	code := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
		for i := 0; i+1 < len(lines); i++ {
			if strings.HasSuffix(lines[i], "-> int:") && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "return \"") {
				fmt.Printf("%s:%d: a string returned from a function annotated -> int\n", filepath.ToSlash(path), i+2)
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

func run(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

// stop is a Stop hook: it runs the verify-fast steps and exits 2 when one
// fails. With honorActive, a payload with stop_hook_active true exits 0
// at once.
func stop(honorActive bool) int {
	var payload struct {
		StopHookActive bool `json:"stop_hook_active"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		fmt.Fprintln(os.Stderr, "no Stop payload:", err)
		return 1
	}
	if honorActive && payload.StopHookActive {
		return 0
	}
	if formatFiles(false) != 0 || run("go", "vet", "./...") != 0 || run("go", "test", "-vet=off", "./...") != 0 {
		fmt.Fprintln(os.Stderr, "verify-fast failed")
		return 2
	}
	return 0
}

// vetFinding is a go vet finding: file:line:col: message.
var vetFinding = regexp.MustCompile(`^(.*\.go):(\d+):\d+: `)

// vetNolint runs go vet and fails on its findings, except those on a line
// that holds //nolint. Output go vet prints that is not a finding fails
// the step when go vet failed.
func vetNolint() int {
	out, err := exec.Command("go", "vet", "./...").CombinedOutput()
	if err == nil {
		return 0
	}
	findings, counted := 0, 0
	for _, line := range strings.Split(string(out), "\n") {
		m := vetFinding.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		findings++
		if !onNolintLine(m[1], m[2]) {
			fmt.Println(line)
			counted++
		}
	}
	switch {
	case findings == 0:
		fmt.Print(string(out))
		return 1
	case counted > 0:
		return 1
	}
	fmt.Println("go vet's findings are all on //nolint lines")
	return 0
}

// onNolintLine reports whether line n of file holds //nolint.
func onNolintLine(file, n string) bool {
	src, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	lines := strings.Split(string(src), "\n")
	var i int
	if _, err := fmt.Sscan(n, &i); err != nil || i < 1 || i > len(lines) {
		return false
	}
	return strings.Contains(lines[i-1], "//nolint")
}

// nolintlint lists the //nolint directives that name no linter, outside
// this program, whose source names the directive.
func nolintlint() int {
	code := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(d.Name(), ".") || path == "ci") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			_, after, ok := strings.Cut(line, "//nolint")
			if ok && !strings.HasPrefix(after, ":") {
				fmt.Printf("%s:%d: //nolint names no linter\n", filepath.ToSlash(path), i+1)
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
