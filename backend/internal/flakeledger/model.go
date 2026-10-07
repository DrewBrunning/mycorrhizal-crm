// Package flakeledger builds the longitudinal flake record (issue #1488).
//
// Flakes were handled by retry-and-forget: gotestsum --rerun-fails, Playwright
// retries and the Android second emulator boot each turn a failed first
// attempt into a green check, and the signal only reached a log line. This
// package turns those per-run retry signals into a per-test ledger over a
// trailing window ({runs, failed-first-attempt, passed-on-retry, failed}),
// renders it, and names the tests that cross the auto-issue policy threshold.
//
// It is advisory by design: nothing here is ever a PR gate.
//
// Inputs are small "flake-*" artifacts each suite uploads (see
// docs/development/testing.md "Flake ledger"):
//
//   - flake-go-<leg>:         junit-<leg>.xml + rerun-fails-<leg>.txt (gotestsum)
//   - flake-playwright-<job>: results.json (Playwright --reporter=json)
//   - flake-android-<leg>:    connected/*.xml (final) + flaky-attempt-N/*.xml
//   - flake-signals-<name>:   neutral JSON retry signals (see Signals) so any
//     other retry mechanism can feed the ledger without a new parser.
package flakeledger

import "sort"

// Outcome is what happened to one test in one CI run.
type Outcome int

const (
	// Passed: passed on its first attempt.
	Passed Outcome = iota
	// PassedOnRetry: failed an attempt, then passed. The flake signal.
	PassedOnRetry
	// Failed: failed every attempt the run gave it.
	Failed
)

func (o Outcome) String() string {
	switch o {
	case Passed:
		return "passed"
	case PassedOnRetry:
		return "passed_on_retry"
	default:
		return "failed"
	}
}

// ParseOutcome is the inverse of String for the neutral signal format.
func ParseOutcome(s string) (Outcome, bool) {
	switch s {
	case "passed":
		return Passed, true
	case "passed_on_retry":
		return PassedOnRetry, true
	case "failed":
		return Failed, true
	}
	return Passed, false
}

// Observations maps a test id to its outcome within one file/suite.
type Observations map[string]Outcome

// Merge folds o into obs keeping the most severe outcome per test.
func (obs Observations) Merge(o Observations) {
	for k, v := range o {
		obs.record(k, v)
	}
}

func (obs Observations) record(test string, o Outcome) {
	if cur, ok := obs[test]; !ok || o > cur {
		obs[test] = o
	}
}

// Run is one completed workflow run (metadata from the Actions API).
type Run struct {
	Workflow   string `json:"workflow"`
	RunID      int64  `json:"run_id"`
	URL        string `json:"url"`
	Event      string `json:"event"`
	CreatedAt  string `json:"created_at"`
	Conclusion string `json:"conclusion"`
}

// RunData is a Run plus what its artifacts said, keyed by suite.
type RunData struct {
	Run    Run
	Suites map[string]Observations
}

// Hit is one run in which a test did not pass first time.
type Hit struct {
	URL     string
	Time    string // RFC3339 created_at of the run
	Outcome Outcome
}

// Stats is one ledger row.
type Stats struct {
	Suite         string
	Test          string
	Runs          int
	PassedOnRetry int
	Failed        int
	Hits          []Hit
}

// FailedFirstAttempt is every run in which attempt 1 did not pass.
func (s Stats) FailedFirstAttempt() int { return s.PassedOnRetry + s.Failed }

// FlakeRate is the share of runs whose first attempt failed.
func (s Stats) FlakeRate() float64 {
	if s.Runs == 0 {
		return 0
	}
	return float64(s.FailedFirstAttempt()) / float64(s.Runs)
}

func sortStats(rows []Stats) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.FlakeRate() != b.FlakeRate() {
			return a.FlakeRate() > b.FlakeRate()
		}
		if a.PassedOnRetry != b.PassedOnRetry {
			return a.PassedOnRetry > b.PassedOnRetry
		}
		if a.Suite != b.Suite {
			return a.Suite < b.Suite
		}
		return a.Test < b.Test
	})
}
