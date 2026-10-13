package bypassedcheck

import (
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
