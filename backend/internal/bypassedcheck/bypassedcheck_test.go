package bypassedcheck

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	later := t0.Add(time.Hour)
	run := func(name, status, concl string, at time.Time) CheckRun {
		return CheckRun{Name: name, Status: status, Conclusion: concl, At: at}
	}
	green := func(n string) CheckRun { return run(n, "completed", "success", t0) }
	req := []string{"A", "B"}
	miss := "no check run or status"

	tests := []struct {
		name     string
		required []string
		runs     []CheckRun
		statuses []Status
		want     []Finding
	}{
		{"all green", req, []CheckRun{green("A"), green("B")}, nil, nil},
		{"one required red", req, []CheckRun{green("A"), run("B", "completed", "failure", t0)}, nil,
			[]Finding{{"B", Failed, "failure"}}},
		{"non-required red ignored", req, []CheckRun{green("A"), green("B"), run("X", "completed", "failure", t0)}, nil, nil},
		{"required missing", req, []CheckRun{green("A")}, nil, []Finding{{"B", Missing, miss}}},
		{"nothing at all", req, nil, nil, []Finding{{"A", Missing, miss}, {"B", Missing, miss}}},
		{"rerun green supersedes earlier red", req,
			[]CheckRun{green("A"), run("B", "completed", "failure", t0), run("B", "completed", "success", later)}, nil, nil},
		{"rerun red supersedes earlier green", req,
			[]CheckRun{green("A"), run("B", "completed", "success", t0), run("B", "completed", "failure", later)}, nil,
			[]Finding{{"B", Failed, "failure"}}},
		{"skipped and neutral pass", req,
			[]CheckRun{run("A", "completed", "skipped", t0), run("B", "completed", "neutral", t0)}, nil, nil},
		{"cancelled fails", req, []CheckRun{green("A"), run("B", "completed", "cancelled", t0)}, nil,
			[]Finding{{"B", Failed, "cancelled"}}},
		{"timed_out and action_required fail", req,
			[]CheckRun{run("A", "completed", "timed_out", t0), run("B", "completed", "action_required", t0)}, nil,
			[]Finding{{"A", Failed, "timed_out"}, {"B", Failed, "action_required"}}},
		{"in progress is incomplete", req, []CheckRun{green("A"), run("B", "in_progress", "", t0)}, nil,
			[]Finding{{"B", Incomplete, "in_progress"}}},
		{"queued is incomplete", req, []CheckRun{green("A"), run("B", "queued", "", t0)}, nil,
			[]Finding{{"B", Incomplete, "queued"}}},
		{"status success", []string{"codecov"}, nil, []Status{{"codecov", "success", t0}}, nil},
		{"status failure", []string{"codecov"}, nil, []Status{{"codecov", "failure", t0}},
			[]Finding{{"codecov", Failed, "failure"}}},
		{"status error", []string{"codecov"}, nil, []Status{{"codecov", "error", t0}},
			[]Finding{{"codecov", Failed, "error"}}},
		{"status pending is incomplete", []string{"codecov"}, nil, []Status{{"codecov", "pending", t0}},
			[]Finding{{"codecov", Incomplete, "pending"}}},
		{"status re-report success after failure", []string{"codecov"}, nil,
			[]Status{{"codecov", "failure", t0}, {"codecov", "success", later}}, nil},
		{"status and check run pooled, latest wins", []string{"A"},
			[]CheckRun{run("A", "completed", "failure", t0)}, []Status{{"A", "success", later}}, nil},
		{"queued rerun without timestamp beats older green", req,
			[]CheckRun{green("A"), green("B"), run("B", "queued", "", time.Time{})}, nil,
			[]Finding{{"B", Incomplete, "queued"}}},
		{"queued rerun without timestamp beats older red, any order", req,
			[]CheckRun{green("A"), run("B", "queued", "", time.Time{}), run("B", "completed", "failure", later)}, nil,
			[]Finding{{"B", Incomplete, "queued"}}},
		{"no required contexts", nil, []CheckRun{run("A", "completed", "failure", t0)}, nil, nil},
		{"findings follow ruleset order", []string{"B", "A"}, nil, nil,
			[]Finding{{"B", Missing, miss}, {"A", Missing, miss}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.required, tc.runs, tc.statuses)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestContexts(t *testing.T) {
	got := Contexts([]Finding{{Context: "b"}, {Context: "a"}})
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSettle(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	green := func(n string) CheckRun { return CheckRun{Name: n, Status: "completed", Conclusion: "success", At: t0} }
	red := func(n string) CheckRun { return CheckRun{Name: n, Status: "completed", Conclusion: "failure", At: t0} }
	running := func(n string) CheckRun { return CheckRun{Name: n, Status: "in_progress", At: t0} }
	req := []string{"A", "B"}
	step := 2 * time.Minute

	// script serves one snapshot per fetch call, repeating the last.
	script := func(snaps ...[]CheckRun) (Fetch, *int) {
		n := 0
		return func() ([]CheckRun, []Status, error) {
			i := n
			if i >= len(snaps) {
				i = len(snaps) - 1
			}
			n++
			return snaps[i], nil, nil
		}, &n
	}

	tests := []struct {
		name       string
		snaps      [][]CheckRun
		maxWait    time.Duration
		want       []Finding
		wantEarly  []string
		wantWaited time.Duration
		wantFetch  int
	}{
		{"green at once never waits", [][]CheckRun{{green("A"), green("B")}}, 10 * step, nil, nil, 0, 1},
		{"incomplete then green: merged early, no findings",
			[][]CheckRun{{green("A"), running("B")}, {green("A"), running("B")}, {green("A"), green("B")}},
			10 * step, nil, []string{"B"}, 2 * step, 3},
		{"incomplete then red: final is failed, still merged early",
			[][]CheckRun{{green("A"), running("B")}, {green("A"), red("B")}},
			10 * step, []Finding{{"B", Failed, "failure"}}, []string{"B"}, step, 2},
		{"still incomplete after the budget",
			[][]CheckRun{{green("A"), running("B")}},
			3 * step, []Finding{{"B", Incomplete, "in_progress"}}, []string{"B"}, 3 * step, 4},
		{"failed at once ends the wait despite other incompletes",
			[][]CheckRun{{red("A"), running("B")}},
			10 * step, []Finding{{"A", Failed, "failure"}, {"B", Incomplete, "in_progress"}}, []string{"B"}, 0, 1},
		{"missing at once ends the wait", [][]CheckRun{{running("A")}},
			10 * step, []Finding{{"A", Incomplete, "in_progress"}, {"B", Missing, "no check run or status"}}, []string{"A"}, 0, 1},
		{"zero budget evaluates once", [][]CheckRun{{green("A"), running("B")}},
			0, []Finding{{"B", Incomplete, "in_progress"}}, []string{"B"}, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetch, n := script(tc.snaps...)
			var slept time.Duration
			got, err := Settle(req, fetch, step, tc.maxWait, func(d time.Duration) { slept += d })
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Findings, tc.want) || !reflect.DeepEqual(got.MergedEarly, tc.wantEarly) ||
				got.Waited != tc.wantWaited || slept != tc.wantWaited || *n != tc.wantFetch {
				t.Fatalf("got %+v slept %v fetches %d", got, slept, *n)
			}
		})
	}

	t.Run("zero interval never loops", func(t *testing.T) {
		fetch, n := script([]CheckRun{running("A"), green("B")})
		got, _ := Settle(req, fetch, 0, time.Hour, func(time.Duration) { t.Fatal("slept") })
		if *n != 1 || len(got.Findings) != 1 {
			t.Fatalf("%+v %d", got, *n)
		}
	})
	t.Run("fetch error", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := Settle(req, func() ([]CheckRun, []Status, error) { return nil, nil, boom }, step, step, nil)
		if !errors.Is(err, boom) {
			t.Fatal(err)
		}
		calls := 0
		_, err = Settle(req, func() ([]CheckRun, []Status, error) {
			calls++
			if calls == 2 {
				return nil, nil, boom
			}
			return []CheckRun{running("A"), green("B")}, nil, nil
		}, step, 5*step, func(time.Duration) {})
		if !errors.Is(err, boom) {
			t.Fatal(err)
		}
	})
}
