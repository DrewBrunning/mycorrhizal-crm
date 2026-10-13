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
	At         time.Time `json:"at"`         // completed_at, else started_at
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
	at     time.Time
	done   bool
	ok     bool
	detail string
}

// Evaluate returns the required contexts that were not green, in the order the
// ruleset lists them. An empty result means every required context passed.
func Evaluate(required []string, runs []CheckRun, statuses []Status) []Finding {
	latest := map[string]observation{}
	consider := func(ctx string, o observation) {
		cur, seen := latest[ctx]
		if !seen || o.at.After(cur.at) {
			latest[ctx] = o
		}
	}
	for _, r := range runs {
		o := observation{at: r.At, detail: r.Status}
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
