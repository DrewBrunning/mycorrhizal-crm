// Package bypassedcheck decides whether a merged pull request's head commit
// was green on every required status check (issue #1661).
//
// A required check does not protect main from an admin-bypass merge, or from a
// merge that lands before the checks finish (the branch reaper then cancels the
// still-running jobs). Both are silent today; PR #1642 merged with
// `Backend (Go)` red and nobody noticed for days. This package is the pure
// decision logic behind .github/workflows/bypassed-check-alert.yml: given the
// ruleset's required contexts and the head SHA's check runs and commit
// statuses, it names every required context that was not green.
//
// Semantics deliberately mirror how GitHub evaluates a required status check:
//
//   - A check run passes on conclusion success, neutral or skipped. (GitHub
//     counts skipped and neutral as passing for required checks; a path-
//     filtered job that was skipped must not raise a false alarm.)
//   - A commit status passes on state success only; failure and error fail it.
//   - A check run that is not completed, or a status in state pending, is
//     "incomplete": the PR merged before that check finished.
//   - A context with no check run and no status at all is "missing".
//   - Check runs and statuses of one context are pooled and the latest by
//     timestamp wins, so a re-run that went green supersedes the earlier red.
//     A check run that is not completed and has no timestamp (a queued re-run
//     has no started_at yet) is the newest of all: it was created after
//     whatever completed before it.
package bypassedcheck

import (
	"sort"
	"time"
)

// Category classifies why a required context was not green.
type Category string

const (
	// Failed: the latest result is a non-passing conclusion/state
	// (failure, cancelled, timed_out, action_required, stale, error, ...).
	Failed Category = "failed"
	// Incomplete: the latest result had not finished at evaluation time, i.e.
	// the PR was merged before the required check completed.
	Incomplete Category = "incomplete"
	// Missing: no check run or status exists for the context.
	Missing Category = "missing"
)

// CheckRun is the subset of a GitHub check run this package reads.
type CheckRun struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`     // queued, in_progress, completed
	Conclusion string    `json:"conclusion"` // success, failure, ... (completed only)
	At         time.Time `json:"at"`         // completed_at, else started_at; zero when a queued run has neither
}

// Status is the subset of a GitHub commit status this package reads.
type Status struct {
	Context string    `json:"context"`
	State   string    `json:"state"` // success, failure, error, pending
	At      time.Time `json:"at"`    // created_at
}

// Finding is one required context that was not green.
type Finding struct {
	Context  string   `json:"context"`
	Category Category `json:"category"`
	// Detail is the raw conclusion/state ("failure", "in_progress", ...) or
	// "no check run or status" for a missing context.
	Detail string `json:"detail"`
}

type observation struct {
	at time.Time
	// unstamped: an unfinished check run with no timestamp. It outranks
	// every stamped observation (see Evaluate).
	unstamped bool
	done      bool
	ok        bool
	detail    string
}

// Evaluate returns the required contexts that were not green, in the order the
// ruleset lists them. An empty result means every required context passed.
func Evaluate(required []string, runs []CheckRun, statuses []Status) []Finding {
	latest := map[string]observation{}
	consider := func(ctx string, o observation) {
		cur, seen := latest[ctx]
		switch {
		case !seen:
			latest[ctx] = o
		case cur.unstamped && !o.unstamped:
			// A queued re-run stays newest until it gets a timestamp.
		case o.unstamped, o.at.After(cur.at):
			latest[ctx] = o
		}
	}
	for _, r := range runs {
		o := observation{at: r.At, detail: r.Status}
		if r.Status != "completed" && r.At.IsZero() {
			o.unstamped = true
		}
		if r.Status == "completed" {
			o.done = true
			o.detail = r.Conclusion
			switch r.Conclusion {
			case "success", "neutral", "skipped":
				o.ok = true
			}
		}
		consider(r.Name, o)
	}
	for _, s := range statuses {
		o := observation{at: s.At, detail: s.State}
		switch s.State {
		case "success":
			o.done, o.ok = true, true
		case "failure", "error":
			o.done = true
		}
		consider(s.Context, o)
	}

	var out []Finding
	for _, ctx := range required {
		o, seen := latest[ctx]
		switch {
		case !seen:
			out = append(out, Finding{ctx, Missing, "no check run or status"})
		case !o.done:
			out = append(out, Finding{ctx, Incomplete, o.detail})
		case !o.ok:
			out = append(out, Finding{ctx, Failed, o.detail})
		}
	}
	return out
}

// Contexts returns the contexts of findings, sorted, for display.
func Contexts(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Context)
	}
	sort.Strings(out)
	return out
}

// Fetch returns the current check runs and commit statuses of the commit.
type Fetch func() ([]CheckRun, []Status, error)

// Settlement is the outcome of Settle.
type Settlement struct {
	// Findings is the final verdict (see Settle for when polling stops).
	Findings []Finding
	// MergedEarly lists the contexts that were incomplete at the first look,
	// i.e. the PR merged before they finished. A context in here that is
	// absent from Findings later finished green.
	MergedEarly []string
	// Waited is the time spent polling.
	Waited time.Duration
}

// Settle evaluates once and, while required contexts are only Incomplete,
// re-evaluates every interval until none are or maxWait is spent. It stops
// at once when any context is Failed or Missing: that is already an alert
// and must not wait on the slow checks. maxWait <= 0 never waits. The
// caller supplies sleep so tests do not.
func Settle(required []string, fetch Fetch, interval, maxWait time.Duration, sleep func(time.Duration)) (Settlement, error) {
	var s Settlement
	first := true
	for {
		runs, statuses, err := fetch()
		if err != nil {
			return s, err
		}
		s.Findings = Evaluate(required, runs, statuses)
		if first {
			for _, f := range s.Findings {
				if f.Category == Incomplete {
					s.MergedEarly = append(s.MergedEarly, f.Context)
				}
			}
			first = false
		}
		onlyIncomplete := len(s.Findings) > 0
		for _, f := range s.Findings {
			if f.Category != Incomplete {
				onlyIncomplete = false
			}
		}
		if !onlyIncomplete || interval <= 0 || s.Waited+interval > maxWait {
			return s, nil
		}
		sleep(interval)
		s.Waited += interval
	}
}
