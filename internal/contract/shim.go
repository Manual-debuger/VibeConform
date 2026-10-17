package contract

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The R-01 shim (spec 0053, "R-01, execution part"). "vibe is not on
// PATH" is done with a directory first on every run's PATH that holds a
// copy of the running vibe named vibe. Started from there, the copy
// records its arguments in a log beside the directory and exits 127.
// Removing the directory that holds vibe instead would also remove every
// tool installed beside it.

const (
	shimDirName = "r01-shim"
	// shimMarker sits beside the shim. The copy is the shim only when it
	// runs from a directory that holds the marker, so an installed vibe
	// that happens to live in a directory named r01-shim is not one.
	shimMarker  = "vibe-r01-shim"
	shimLogName = "r01-shim.log"
	// ShimExitCode is the shim's exit code: a shell's "command not found".
	ShimExitCode = 127
	shimMessage  = "vibe is not available during the R-01 probe"
)

// RunAsShim makes the running executable act as the R-01 shim when it is
// one: it records its arguments, prints why it refuses, and returns
// ShimExitCode and true. Otherwise it returns false and does nothing.
// cmd/vibe's main calls it first, and so does every test binary that
// runs the full audit in-process, since the shim is a copy of the
// running executable.
func RunAsShim() (int, bool) {
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != shimDirName {
		return 0, false
	}
	if _, err := os.Stat(filepath.Join(dir, shimMarker)); err != nil {
		return 0, false
	}
	cwd, _ := os.Getwd()
	line, err := json.Marshal(shimCall{Args: os.Args[1:], Dir: cwd})
	if err == nil {
		// #nosec G304 -- the log beside the shim directory the audit made
		if f, err := os.OpenFile(filepath.Join(filepath.Dir(dir), shimLogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.Write(append(line, '\n'))
			_ = f.Close()
		}
	}
	fmt.Fprintln(os.Stderr, shimMessage)
	return ShimExitCode, true
}

// shimCall is one call of vibe the shim recorded.
type shimCall struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
	// Run is the label of the run during which the call happened.
	Run string `json:"-"`
	// File declares that run.
	File string `json:"-"`
}

// String writes the call as a command: "vibe audit --static".
func (c shimCall) String() string {
	return strings.TrimSpace("vibe " + strings.Join(c.Args, " "))
}

// shim is the installed R-01 shim of one audit.
type shim struct {
	dir string
	log string
	// read is how much of the log the audit has read.
	read int64
}

// installShim puts a copy of the running executable, named vibe, into
// tmp/r01-shim, beside the marker. A hard link is tried first; a copy
// when the file system refuses one.
func installShim(tmp string) (*shim, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find the running vibe to copy as the R-01 shim: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Join(tmp, shimDirName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	name := "vibe"
	if runtime.GOOS == "windows" {
		name = "vibe.exe"
	}
	dst := filepath.Join(dir, name)
	if err := os.Link(exe, dst); err != nil {
		if err := copyFile(exe, dst, 0o755); err != nil {
			return nil, fmt.Errorf("cannot copy the running vibe as the R-01 shim: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, shimMarker), []byte("vibe audit R-01 shim\n"), 0o600); err != nil {
		return nil, err
	}
	return &shim{dir: dir, log: filepath.Join(tmp, shimLogName)}, nil
}

// calls returns the calls recorded since the last call of calls.
func (s *shim) calls() []shimCall {
	f, err := os.Open(s.log)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(s.read, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	// Only whole lines; a partial one is read next time.
	end := bytes.LastIndexByte(data, '\n') + 1
	s.read += int64(end)
	var out []shimCall
	sc := bufio.NewScanner(bytes.NewReader(data[:end]))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var c shimCall
		if json.Unmarshal(sc.Bytes(), &c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// copyFile copies src to dst with mode perm.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) // #nosec G304 -- a tracked file of the audited repository, or the running executable
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm) // #nosec G304 -- a path inside the audit's temporary directory
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
