package reasonprose

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	apperrors "mycorrhizal/errors"
)

// Runner is the part of *testing.M that RunGuarded needs.
type Runner interface{ Run() int }

// RunGuarded runs a package's tests with an observer on every reason the
// package's code attaches (apperrors.ObserveReasons), and fails the run when
// any of them carries prose (issue #2920 item 2). Wire it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(reasonprose.RunGuarded(m)) }
//
// The per-assertion helpers (requireNoReasonProse) only check the responses a
// test author chose to inspect; this sees every WithReason the suite reaches,
// so a prose param on a path that some unrelated test happens to exercise
// still fails.
func RunGuarded(m Runner) int {
	return runGuarded(m, os.Stderr)
}

func runGuarded(m Runner, out io.Writer) int {
	var mu sync.Mutex
	seen := map[string]Problem{}
	restore := apperrors.ObserveReasons(func(code apperrors.ReasonCode, params map[string]any) {
		for _, p := range Check(string(code), params) {
			mu.Lock()
			seen[p.Code+"\x00"+p.Key+"\x00"+p.Value] = p
			mu.Unlock()
		}
	})
	code := m.Run()
	restore()
	if len(seen) == 0 {
		return code
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	_, _ = fmt.Fprintf(out, "FAIL: reason_params prose guard (issue #2920): %d prose value(s) attached during this package's tests\n", len(keys))
	for _, k := range keys {
		p := seen[k]
		_, _ = fmt.Fprintf(out, "  reason_params[%q] for %s is free prose: %q (%s)\n", p.Key, p.Code, p.Value, p.Why)
	}
	if code == 0 {
		code = 1
	}
	return code
}
