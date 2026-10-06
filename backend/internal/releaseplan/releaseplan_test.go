package releaseplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shaR   = "1111111111111111111111111111111111111111" // the validated (pre-fixture) commit
	shaTag = "2222222222222222222222222222222222222222" // the tagged (fixture) commit
	tagV   = "v1.2.3"
)

var gates = []string{"deploy-smoke", "e2e-tests", "unit-tests"}

func green(sha string) Ledger {
	var l Ledger
	for _, g := range gates {
		l = append(l, Entry{SHA: sha, ReleaseTag: tagV, Gate: g, Conclusion: ConclusionSuccess, RunID: "100"})
	}
	return l
}

func without(l Ledger, gate string) Ledger {
	var out Ledger
	for _, e := range l {
		if e.Gate != gate {
			out = append(out, e)
		}
	}
	return out
}

func with(l Ledger, gate, conclusion string) Ledger {
	out := append(Ledger{}, l...)
	for i := range out {
		if out[i].Gate == gate {
			out[i].Conclusion = conclusion
		}
	}
	return out
}

func TestComposedGates(t *testing.T) {
	t.Run("job ids with uses, sorted, bookkeeping jobs excluded", func(t *testing.T) {
		got, err := ComposedGates("jobs:\n  b-gate:\n    uses: ./.github/workflows/b.yml\n  build-candidate:\n    runs-on: x\n  a-gate:\n    uses: ./.github/workflows/a.yml\n  results:\n    runs-on: x\n")
		require.NoError(t, err)
		assert.Equal(t, []string{"a-gate", "b-gate"}, got)
	})
	t.Run("unparseable", func(t *testing.T) {
		_, err := ComposedGates("jobs: [unclosed")
		assert.ErrorContains(t, err, "does not parse")
	})
	t.Run("no gates", func(t *testing.T) {
		_, err := ComposedGates("jobs:\n  results:\n    runs-on: x\n")
		assert.ErrorContains(t, err, "no gate jobs")
	})
	t.Run("the real composer yields the known gates", func(t *testing.T) {
		b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release-validate.yml"))
		require.NoError(t, err)
		got, err := ComposedGates(string(b))
		require.NoError(t, err)
		assert.Contains(t, got, "unit-tests")
		assert.Contains(t, got, "deploy-smoke")
		assert.NotContains(t, got, CandidateJob)
		assert.NotContains(t, got, ResultsJob)
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found")
		}
		dir = parent
	}
}

func TestSplitList(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, SplitList(" a, b ,,a"))
	assert.Empty(t, SplitList(""))
	assert.Empty(t, SplitList(" , "))
}

func TestParseLedger(t *testing.T) {
	l, err := ParseLedger([]byte(" \n"))
	require.NoError(t, err)
	assert.Empty(t, l)
	l, err = ParseLedger([]byte(`[{"sha":"s","release_tag":"t","gate":"g","conclusion":"success","run_id":"1","carried_from_run":"0"}]`))
	require.NoError(t, err)
	assert.Equal(t, "0", l[0].origin(), "a carried entry's origin is the run that earned it")
	assert.Equal(t, "1", Entry{RunID: "1"}.origin())
	_, err = ParseLedger([]byte("{nope"))
	assert.ErrorContains(t, err, "does not parse")
}

func TestBuildLedger(t *testing.T) {
	base := BuildInput{Gates: gates, SHA: shaR, Tag: tagV, RunID: "200",
		Results: map[string]string{"unit-tests": "success", "e2e-tests": "failure", "deploy-smoke": "success", CandidateJob: "success"}}

	t.Run("every composed gate records its own result; the candidate job is not a gate", func(t *testing.T) {
		l, err := BuildLedger(base)
		require.NoError(t, err)
		require.Len(t, l, 3)
		got := map[string]string{}
		for _, e := range l {
			got[e.Gate] = e.Conclusion
			assert.Equal(t, shaR, e.SHA)
			assert.Equal(t, tagV, e.ReleaseTag)
			assert.Equal(t, "200", e.RunID)
			assert.Empty(t, e.CarriedFromRun)
		}
		assert.Equal(t, map[string]string{"unit-tests": "success", "e2e-tests": "failure", "deploy-smoke": "success"}, got)
	})

	t.Run("a skipped gate records the carried success with the earning run, not this one", func(t *testing.T) {
		in := base
		in.Results = map[string]string{"unit-tests": "skipped", "e2e-tests": "success", "deploy-smoke": "success"}
		in.Skip = []string{"unit-tests"}
		in.Carried = Ledger{{SHA: shaR, ReleaseTag: tagV, Gate: "unit-tests", Conclusion: "success", RunID: "100"}}
		l, err := BuildLedger(in)
		require.NoError(t, err)
		var u Entry
		for _, e := range l {
			if e.Gate == "unit-tests" {
				u = e
			}
		}
		assert.Equal(t, Entry{SHA: shaR, ReleaseTag: tagV, Gate: "unit-tests", Conclusion: "success", RunID: "200", CarriedFromRun: "100"}, u)
	})

	t.Run("a re-carried entry keeps pointing at the original earning run", func(t *testing.T) {
		in := base
		in.Results = map[string]string{"unit-tests": "skipped", "e2e-tests": "success", "deploy-smoke": "success"}
		in.Skip = []string{"unit-tests"}
		in.Carried = Ledger{{SHA: shaR, ReleaseTag: tagV, Gate: "unit-tests", Conclusion: "success", RunID: "150", CarriedFromRun: "100"}}
		l, err := BuildLedger(in)
		require.NoError(t, err)
		for _, e := range l {
			if e.Gate == "unit-tests" {
				assert.Equal(t, "100", e.CarriedFromRun)
			}
		}
	})

	t.Run("tag-time reuse: the carried entry keeps the validated sha, not the tag's", func(t *testing.T) {
		in := base
		in.SHA = shaTag
		in.Results = map[string]string{"unit-tests": "skipped", "e2e-tests": "skipped", "deploy-smoke": "success"}
		in.Skip = []string{"unit-tests", "e2e-tests"}
		in.Carried = green(shaR)
		l, err := BuildLedger(in)
		require.NoError(t, err)
		for _, e := range l {
			switch e.Gate {
			case "deploy-smoke":
				assert.Equal(t, shaTag, e.SHA, "the retested gate ran against the tag")
			default:
				assert.Equal(t, shaR, e.SHA, "a carried gate never claims the tag's commit")
				assert.Equal(t, "success", e.Conclusion)
			}
		}
	})

	t.Run("a skipped gate with no carried success is recorded skipped, never success", func(t *testing.T) {
		in := base
		in.Results = map[string]string{"unit-tests": "skipped", "e2e-tests": "success", "deploy-smoke": "success"}
		in.Skip = []string{"unit-tests"}
		l, err := BuildLedger(in)
		require.NoError(t, err)
		for _, e := range l {
			if e.Gate == "unit-tests" {
				assert.Equal(t, "skipped", e.Conclusion)
			}
		}
	})

	t.Run("a skip-listed gate that nevertheless ran records what it did", func(t *testing.T) {
		in := base
		in.Skip = []string{"e2e-tests"}
		in.Carried = green(shaR)
		l, err := BuildLedger(in)
		require.NoError(t, err)
		for _, e := range l {
			if e.Gate == "e2e-tests" {
				assert.Equal(t, "failure", e.Conclusion, "the run was told to skip it but it reported failure: record the truth, do not launder a carried success over it")
				assert.Empty(t, e.CarriedFromRun)
			}
		}
	})

	t.Run("a gate with no needs result is an error", func(t *testing.T) {
		in := base
		delete(in.Results, "unit-tests")
		_, err := BuildLedger(in)
		assert.ErrorContains(t, err, `no result for gate "unit-tests"`)
	})

	for _, blank := range []string{"sha", "tag", "run"} {
		t.Run("missing "+blank, func(t *testing.T) {
			in := base
			switch blank {
			case "sha":
				in.SHA = ""
			case "tag":
				in.Tag = ""
			default:
				in.RunID = ""
			}
			_, err := BuildLedger(in)
			assert.ErrorContains(t, err, "required")
		})
	}
}

func TestPlanRerun(t *testing.T) {
	in := func(requested ...string) RerunInput {
		return RerunInput{Gates: gates, Requested: requested, Prior: with(green(shaR), "e2e-tests", "failure"), SHA: shaR, Tag: tagV}
	}

	t.Run("named gate re-runs and every other green gate is carried", func(t *testing.T) {
		p, err := PlanRerun(in("e2e-tests"))
		require.NoError(t, err)
		assert.Equal(t, []string{"e2e-tests"}, p.Rerun)
		assert.Equal(t, []string{"deploy-smoke", "unit-tests"}, p.Skip)
		assert.Len(t, p.Carried, 2)
		assert.Empty(t, p.Added)
	})

	t.Run("a gate the operator did not name but that is not green is forced to run and reported", func(t *testing.T) {
		p, err := PlanRerun(in("unit-tests"))
		require.NoError(t, err)
		assert.Equal(t, []string{"e2e-tests", "unit-tests"}, p.Rerun)
		assert.Equal(t, []string{"e2e-tests"}, p.Added)
		assert.Equal(t, []string{"deploy-smoke"}, p.Skip)
	})

	t.Run("the failed keyword re-runs exactly the not-green gates", func(t *testing.T) {
		p, err := PlanRerun(in(FailedKeyword))
		require.NoError(t, err)
		assert.Equal(t, []string{"e2e-tests"}, p.Rerun)
		assert.Empty(t, p.Added, "failed mode adds nothing the operator did not ask for")
	})

	t.Run("failed with a missing entry re-runs it", func(t *testing.T) {
		r := in(FailedKeyword)
		r.Prior = without(r.Prior, "unit-tests")
		p, err := PlanRerun(r)
		require.NoError(t, err)
		assert.Equal(t, []string{"e2e-tests", "unit-tests"}, p.Rerun)
	})

	t.Run("nothing carries from a different commit", func(t *testing.T) {
		r := in("e2e-tests")
		r.Prior = green("9999999999999999999999999999999999999999")
		p, err := PlanRerun(r)
		require.NoError(t, err)
		assert.Equal(t, gates, p.Rerun, "stale-commit successes prove nothing: everything re-runs")
		assert.Empty(t, p.Skip)
	})

	t.Run("nothing carries from a different release tag", func(t *testing.T) {
		r := in("e2e-tests")
		for i := range r.Prior {
			r.Prior[i].ReleaseTag = "v9.9.9"
		}
		p, err := PlanRerun(r)
		require.NoError(t, err)
		assert.Equal(t, gates, p.Rerun)
	})

	t.Run("a skipped or cancelled prior conclusion is not a carried success", func(t *testing.T) {
		for _, c := range []string{"skipped", "cancelled", "failure"} {
			r := in("e2e-tests")
			r.Prior = with(green(shaR), "unit-tests", c)
			p, err := PlanRerun(r)
			require.NoError(t, err)
			assert.Contains(t, p.Rerun, "unit-tests", c)
		}
	})

	t.Run("unknown gate is rejected naming the valid ones", func(t *testing.T) {
		_, err := PlanRerun(in("e2e-test"))
		assert.ErrorContains(t, err, "unknown gate(s) e2e-test")
		assert.ErrorContains(t, err, "unit-tests")
	})

	t.Run("failed cannot be mixed with names", func(t *testing.T) {
		_, err := PlanRerun(in(FailedKeyword, "e2e-tests"))
		assert.ErrorContains(t, err, "cannot be combined")
	})

	t.Run("empty request", func(t *testing.T) {
		_, err := PlanRerun(in())
		assert.ErrorContains(t, err, "empty")
	})

	t.Run("missing sha or tag", func(t *testing.T) {
		r := in("e2e-tests")
		r.SHA = ""
		_, err := PlanRerun(r)
		assert.ErrorContains(t, err, "required")
	})

	t.Run("everything already green and failed requested: nothing to do", func(t *testing.T) {
		r := in(FailedKeyword)
		r.Prior = green(shaR)
		_, err := PlanRerun(r)
		assert.ErrorContains(t, err, "nothing to rerun")
	})
}

func TestDecideReuse(t *testing.T) {
	fixture := []string{"backend/database/testdata/schemas/" + tagV + ".sql", "backend/internal/schemafixture/releases.go"}
	in := func() ReuseInput {
		return ReuseInput{Gates: gates, Prior: green(shaR), Tag: tagV, TagSHA: shaTag, ChangedFiles: fixture}
	}

	t.Run("fixture-only diff reuses everything except the retest floor", func(t *testing.T) {
		d := DecideReuse(in())
		require.True(t, d.Reuse, d.Reason)
		assert.Equal(t, shaR, d.ValidatedSHA)
		assert.Equal(t, []string{"deploy-smoke"}, d.Retest)
		assert.Equal(t, []string{"e2e-tests", "unit-tests"}, d.Skip)
		require.Len(t, d.Carried, 2)
		for _, e := range d.Carried {
			assert.Equal(t, shaR, e.SHA)
		}
		assert.Contains(t, d.Reason, "schema-fixture registration")
	})

	t.Run("either fixture file alone is within the allowlist", func(t *testing.T) {
		for _, f := range fixture {
			r := in()
			r.ChangedFiles = []string{f}
			assert.True(t, DecideReuse(r).Reuse, f)
		}
	})

	t.Run("a path is compared after cleaning", func(t *testing.T) {
		r := in()
		r.ChangedFiles = []string{"./backend/internal/schemafixture/releases.go"}
		assert.True(t, DecideReuse(r).Reuse)
	})

	t.Run("an RC: the tag IS the validated commit", func(t *testing.T) {
		r := in()
		r.TagSHA = shaR
		r.ChangedFiles = nil
		d := DecideReuse(r)
		require.True(t, d.Reuse)
		assert.Equal(t, "tagged commit is the validated commit", d.Reason)
	})

	t.Run("different commit, identical tree", func(t *testing.T) {
		r := in()
		r.ChangedFiles = nil
		d := DecideReuse(r)
		require.True(t, d.Reuse)
		assert.Contains(t, d.Reason, "validated commit's tree")
	})

	t.Run("a source file in the diff forces the full battery", func(t *testing.T) {
		r := in()
		r.ChangedFiles = append([]string{"backend/controllers/contact_controller.go"}, fixture...)
		d := DecideReuse(r)
		assert.False(t, d.Reuse)
		assert.Contains(t, d.Reason, "backend/controllers/contact_controller.go")
		assert.Empty(t, d.Skip)
		assert.Empty(t, d.Carried)
	})

	t.Run("another release's schema dump is not on the allowlist", func(t *testing.T) {
		r := in()
		r.ChangedFiles = []string{"backend/database/testdata/schemas/v9.9.9.sql"}
		assert.False(t, DecideReuse(r).Reuse)
	})

	t.Run("a workflow change forces the full battery", func(t *testing.T) {
		r := in()
		r.ChangedFiles = []string{".github/workflows/release-validate.yml"}
		assert.False(t, DecideReuse(r).Reuse)
	})

	t.Run("no ledger", func(t *testing.T) {
		r := in()
		r.Prior = nil
		d := DecideReuse(r)
		assert.False(t, d.Reuse)
		assert.Contains(t, d.Reason, "no pre-tag gate ledger")
	})

	t.Run("a gate with no recorded success blocks reuse and is named", func(t *testing.T) {
		r := in()
		r.Prior = without(r.Prior, "e2e-tests")
		d := DecideReuse(r)
		assert.False(t, d.Reuse)
		assert.Contains(t, d.Reason, "e2e-tests")
	})

	t.Run("a non-success conclusion blocks reuse", func(t *testing.T) {
		for _, c := range []string{"failure", "skipped", "cancelled"} {
			r := in()
			r.Prior = with(r.Prior, "unit-tests", c)
			assert.False(t, DecideReuse(r).Reuse, c)
		}
	})

	t.Run("a ledger spanning two commits is ambiguous", func(t *testing.T) {
		r := in()
		r.Prior = append(green(shaR), Entry{SHA: shaTag, ReleaseTag: tagV, Gate: "x", Conclusion: "success", RunID: "1"})
		d := DecideReuse(r)
		assert.False(t, d.Reuse)
		assert.Contains(t, d.Reason, "exactly one commit")
	})

	t.Run("a ledger for another tag is not this release's", func(t *testing.T) {
		r := in()
		r.Tag = "v9.9.9"
		assert.False(t, DecideReuse(r).Reuse)
	})

	t.Run("an entry for another tag does not poison this tag's commit", func(t *testing.T) {
		r := in()
		r.Prior = append(Ledger{{SHA: "other", ReleaseTag: "v0.0.1", Gate: "x", Conclusion: "success", RunID: "1"}}, r.Prior...)
		assert.True(t, DecideReuse(r).Reuse)
	})

	t.Run("the retest floor is only applied when the composer has that gate", func(t *testing.T) {
		r := in()
		r.Gates = []string{"e2e-tests", "unit-tests"}
		d := DecideReuse(r)
		require.True(t, d.Reuse)
		assert.Empty(t, d.Retest)
		assert.Equal(t, []string{"e2e-tests", "unit-tests"}, d.Skip)
	})

	t.Run("the decision never aliases the caller's slice", func(t *testing.T) {
		r := in()
		d := DecideReuse(r)
		d.ChangedFiles[0] = "mutated"
		assert.Equal(t, fixture[0], r.ChangedFiles[0])
	})

	t.Run("allowlist is exactly the fixture commit's two files", func(t *testing.T) {
		assert.Equal(t, fixture, FixtureAllowlist(tagV))
		assert.True(t, strings.HasSuffix(FixtureAllowlist("v1.0.0")[0], "v1.0.0.sql"))
	})
}
