package contract_test

import (
	"flag"
	"time"
)

// packageTimeout is this package's own test budget. Its guard probe
// fixtures run hundreds of hook processes each, and under go test ./...
// on a loaded machine the package can pass go test's ten-minute default
// although no test hangs (spec 0053). The budget lives here, in this
// repository's own test code, not in the generated Taskfile.yml or
// ci.yml that adopters receive.
const packageTimeout = 30 * time.Minute

// raiseDefaultTimeout raises -test.timeout to packageTimeout when it is
// go test's default of ten minutes (an explicit -timeout 10m looks the
// same). A shorter or longer -timeout, and 0 (no limit), are kept.
func raiseDefaultTimeout() {
	if !flag.Parsed() {
		flag.Parse()
	}
	f := flag.Lookup("test.timeout")
	if f == nil {
		return
	}
	if d, err := time.ParseDuration(f.Value.String()); err == nil && d == 10*time.Minute {
		_ = f.Value.Set(packageTimeout.String())
	}
}
