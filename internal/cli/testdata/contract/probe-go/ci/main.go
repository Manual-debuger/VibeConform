// Command ci is the probe fixture's tooling: its entrypoints and hooks
// run go run ./ci <step>, so that the fixture needs only Go (spec 0053,
// "Testing Decisions").
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	case "test":
		os.Exit(run("go", "test", "./..."))
	case "ok":
		fmt.Println("ok")
	case "fail":
		fmt.Println("failing on purpose")
		os.Exit(1)
	case "sleep":
		time.Sleep(10 * time.Minute)
	case "call-vibe":
		os.Exit(run("vibe", "version"))
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
