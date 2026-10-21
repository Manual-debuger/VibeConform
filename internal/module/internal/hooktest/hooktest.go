// Package hooktest turns a test binary into stub formatters, so the
// hook:format tests can run every standard's script, through the real
// Task, without goimports, Ruff or Prettier installed.
//
// A stub opens every file argument from its own working directory, as the
// real formatters do, and exits 2 on one it cannot open. Otherwise it
// records the file and changes nothing.
package hooktest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// logEnv names the file the stubs record their files in. Only a stub's
// process tree has it set, so the test binary it is a copy of never sees it.
const logEnv = "VIBE_HOOKTEST_FORMATTER_LOG"

// formatted are the extensions the hook:format scripts select; any other
// argument is a subcommand or a flag ("run", "--write", "warn").
var formatted = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true,
	".tsx": true, ".json": true, ".css": true, ".md": true,
}

// Main runs the stub and exits when the binary was started as one. Call it
// first in TestMain.
func Main() {
	log := os.Getenv(logEnv)
	if log == "" {
		return
	}
	os.Exit(stub(log, os.Args[1:]))
}

func stub(log string, args []string) int {
	name := filepath.Base(os.Args[0])
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 2
	}
	var files []string
	for _, arg := range args {
		if !formatted[filepath.Ext(arg)] {
			continue
		}
		if _, err := os.Stat(arg); err != nil { // #nosec G703 -- a formatter opens what hook:format passes it
			fmt.Fprintf(os.Stderr, "%s: %v (working directory %s)\n", name, err, cwd)
			return 2
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			return 2
		}
		files = append(files, abs+"\n")
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 G703 -- the test that installed the stub chose the path
	if err == nil {
		_, err = io.WriteString(f, strings.Join(files, ""))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 2
	}
	return 0
}

// Install puts a stub under each name first on PATH for the rest of the
// test, and returns the log Formatted reads. It uses t.Setenv, so the test
// must not be parallel.
func Install(t *testing.T, names ...string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, name := range names {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		copyFile(t, self, filepath.Join(bin, name))
	}
	log := filepath.Join(t.TempDir(), "formatted.log")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(logEnv, log)
	return log
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	// #nosec G304 -- the running test binary
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	// #nosec G302 G304 -- an executable in the test's own temp directory
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// Formatted returns every file the stubs were given, as absolute paths
// with symlinks resolved, in the order they got them.
func Formatted(t *testing.T, log string) []string {
	t.Helper()
	// #nosec G304 -- the log Install chose
	f, err := os.Open(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var files []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		files = append(files, Resolve(t, scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return files
}

// Resolve returns path with symlinks resolved, so it compares equal to
// what Formatted returns (macOS's temp directory is behind a symlink).
func Resolve(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
