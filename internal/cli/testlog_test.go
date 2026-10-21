package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
)

// testLogLimit caps the entries of this package's test log, the file in
// which the testing package records every file the binary opens or stats
// and every variable it reads. Without -count=1, go test hashes each entry
// after the run and again before a cache lookup, with up to two
// filepath.EvalSymlinks calls per entry outside the module (spec 0056,
// A-05). At 661,000 entries that took 73s in the main checkout and 10m in
// a probe copy deep under %TEMP%, past the Stop hook's 600s timeout. The
// tests write about 100,000 now.
const testLogLimit = 150_000

// checkTestLog returns code, or 1 when the run passed but its test log
// holds more than testLogLimit entries. go test passes -test.testlogfile
// only when the result can be cached, which is how the Stop hook runs the
// suite; with -count=1 there is nothing to check. testing closes the log
// before m.Run returns.
func checkTestLog(code int) int {
	f := flag.Lookup("test.testlogfile")
	if code != 0 || f == nil || f.Value.String() == "" {
		return code
	}
	data, err := os.ReadFile(f.Value.String())
	if err != nil {
		return code
	}
	if n := bytes.Count(data, []byte("\n")); n > testLogLimit {
		fmt.Fprintf(os.Stderr, "the test log holds %d entries, more than %d: go test hashes each one before it can report "+
			"this package, which takes minutes. Look for a file or PATH search repeated in every test.\n", n, testLogLimit)
		return 1
	}
	return code
}

// memoLookPath wraps look so that each name is searched for once. Every
// sync warns about missing tools, and on Windows each search for a tool
// that is not installed stats every PATH directory with every PATHEXT
// extension: hundreds of syncs filled the test log with 600,000 stats.
// Tests that need a given set of tools still install stubLookPath.
func memoLookPath(look func(string) (string, error)) func(string) (string, error) {
	type result struct {
		path string
		err  error
	}
	var mu sync.Mutex
	seen := make(map[string]result)
	return func(name string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if r, ok := seen[name]; ok {
			return r.path, r.err
		}
		p, err := look(name)
		seen[name] = result{p, err}
		return p, err
	}
}

func TestMemoLookPathSearchesEachNameOnce(t *testing.T) {
	calls := map[string]int{}
	look := memoLookPath(func(name string) (string, error) {
		calls[name]++
		if name == "go" {
			return "/bin/go", nil
		}
		return "", exec.ErrNotFound
	})
	for range 3 {
		if p, err := look("go"); p != "/bin/go" || err != nil {
			t.Fatalf("look(go) = %q, %v", p, err)
		}
		if _, err := look("lefthook"); !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("look(lefthook) err = %v, want exec.ErrNotFound", err)
		}
	}
	if calls["go"] != 1 || calls["lefthook"] != 1 {
		t.Errorf("searches = %v, want one per name", calls)
	}
}
