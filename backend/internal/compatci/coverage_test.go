// Package compatci is the structural coupling between
// docs/development/supported-runtime-matrix.md (COMPAT-01, issue #472) and
// the CI jobs that actually exercise each row's stated minimum (COMPAT-02,
// issue #473). It has no runtime purpose — it exists purely so
// TestMatrixRowsHaveCoverage fails when the two drift apart, per #473's "How
// to verify": "a row in #472 with no corresponding job should fail a
// structural check, so the two cannot drift apart." Kept entirely in a
// _test.go file (matrixCoverage included) rather than a regular .go file,
// the same shape as integrations/int02_coverage_test.go's int02Coverage —
// there is no runtime code here for anything to import.
package compatci

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Paths are relative to this package (backend/internal/compatci -> repo root
// is three levels up).
const (
	matrixDocRel    = "../../../docs/development/supported-runtime-matrix.md"
	workflowsDirRel = "../../../.github/workflows"
)

// coverage records how one matrix row's declared minimum is proven in CI.
//
// A row either names the workflow file + job ID that builds/tests at the
// floor (workflow/job), optionally paired with a second job that proves a
// version *below* the floor fails clearly (belowFloorJob, per #473 action 4)
// — or, when the row has no independent CI job of its own, note explains why
// (it rides on another row's job, or it isn't mechanically checkable in CI at
// all, mirroring how the matrix doc itself documents the non-checkable rows
// under "Fail-clearly behavior").
type coverage struct {
	// workflow is the .github/workflows/ file (bare filename) that runs the
	// minimum-version job for this row. Empty means "see note".
	workflow string
	// job is the job ID within workflow that builds/tests at the declared
	// floor.
	job string
	// belowFloorWorkflow is the workflow file containing the job that
	// proves a version *below* the floor fails clearly. Defaults to
	// workflow when empty and belowFloorJob is set.
	belowFloorWorkflow string
	// belowFloorJob is the job ID that exercises one version under the
	// floor and asserts the failure is comprehensible (#473 action 4).
	// Empty when the below-floor case is covered by a non-workflow test
	// instead — see note.
	belowFloorJob string
	// note explains the row's coverage when it has no dedicated
	// workflow/job of its own (rides on another row, or isn't
	// independently checkable).
	note string
}

// matrixCoverage maps each supported-runtime-matrix.md row (its bolded
// component name, first table column) to how COMPAT-02 exercises it. Every
// row in the doc must have an entry here — TestMatrixRowsHaveCoverage fails
// otherwise — and every entry naming a workflow/job must point at a job that
// actually exists.
var matrixCoverage = map[string]coverage{
	"Go": {
		workflow:      "min-version-tests.yml",
		job:           "go-minimum",
		belowFloorJob: "go-below-minimum",
	},
	"Node.js": {
		workflow:      "min-version-tests.yml",
		job:           "node-yarn-minimum",
		belowFloorJob: "node-below-minimum",
	},
	"Yarn": {
		note: "Pinned to its exact floor (1.22.0) and exercised in the same " +
			"job as the Node.js row (min-version-tests.yml / node-yarn-minimum) " +
			"-- Yarn Classic has no independent build step of its own to run at " +
			"a different version.",
	},
	"Browsers": {
		workflow: "min-version-tests.yml",
		job:      "browser-minimum",
		note: "The below-floor case (a browser with no ES-module support at " +
			"all, well under the stated floor) is covered by a frontend unit " +
			"test (src/unsupportedBrowserFallback.test.ts), not a live old " +
			"browser in CI -- module-vs-nomodule script dispatch is a " +
			"guaranteed HTML5 platform behavior, not something this project's " +
			"code implements or needs a real ancient browser binary to prove.",
	},
	"SQLite": {
		note: "Ships via the pure-Go driver and has no version of its own -- " +
			"it travels with the Go row above (Go / go-minimum already builds " +
			"and tests every package that touches the sqlite driver).",
	},
	"Docker Engine": {
		workflow: "min-version-tests.yml",
		job:      "docker-compose-minimum",
	},
	"Docker Compose": {
		note: "Tested in the same job as Docker Engine " +
			"(min-version-tests.yml / docker-compose-minimum) -- the matrix " +
			"doc itself has no independent Compose-only floor beyond " +
			"\"whatever ships with the Engine minimum,\" so there is nothing to " +
			"test separately.",
	},
	"Host OS / architecture": {
		note: "Not independently checkable as a *minimum version* -- Linux " +
			"x86_64/arm64 is the only shape this project builds and tests at " +
			"all (every job in this repository already runs on that shape; " +
			"arm64 specifically is covered by docker-publish.yml's multi-arch " +
			"build, not a minimum-version job since there is no lower ISA " +
			"floor to test below).",
	},
	"Android": {
		workflow:      "android-tests.yml",
		job:           "android-e2e-min-sdk",
		belowFloorJob: "android-below-min-sdk",
	},
}

// Caching note (hand-verified): matrixDocRel/workflowsDirRel live outside
// the backend Go module, so go test's result cache does not see them as
// build inputs and can serve a stale PASS for an edit to only the doc or a
// workflow file, with no .go change, run back-to-back with no intervening
// `go clean -testcache`. unit-tests.yml's backend job always passes
// `-coverprofile`, which Go never caches results for, so real CI is
// unaffected -- a plain local `go test ./internal/compatci/...` (no
// -coverprofile) is not; add `-count=1` there if you need a guaranteed-fresh
// run.

// matrixRowPattern matches a table row's bolded first column, e.g.
// "| **Go** | `1.26.0` ... |" -> "Go", "| **Node.js** (contributor/CI...) | ..." -> "Node.js".
// Only the table body matters here (header/separator rows have no `**`).
var matrixRowPattern = regexp.MustCompile(`(?m)^\|\s*\*\*([^*]+)\*\*`)

// matrixRows extracts every component name from the committed matrix doc's
// table, in document order.
func matrixRows(t *testing.T) []string {
	t.Helper()
	doc, err := os.ReadFile(matrixDocRel)
	if err != nil {
		t.Fatalf("reading %s: %v", matrixDocRel, err)
	}
	matches := matrixRowPattern.FindAllStringSubmatch(string(doc), -1)
	if len(matches) == 0 {
		t.Fatalf("%s: found no `| **Component**` table rows -- did the table format change? "+
			"update matrixRowPattern in coverage_test.go", matrixDocRel)
	}
	rows := make([]string, len(matches))
	for i, m := range matches {
		rows[i] = m[1]
	}
	return rows
}

// workflowJob is the minimal shape this test needs from a GitHub Actions
// workflow file: just the set of job IDs.
type workflowFile struct {
	Jobs map[string]any `yaml:"jobs"`
}

// jobExists parses .github/workflows/<file> and reports whether it declares
// a job with the given ID, so a coverage entry can't silently point at a
// workflow/job that was renamed or removed.
func jobExists(t *testing.T, file, job string) bool {
	t.Helper()
	path := filepath.Join(workflowsDirRel, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("reading workflow %s (referenced by compatci's matrixCoverage): %v", path, err)
		return false
	}
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Errorf("parsing workflow %s: %v", path, err)
		return false
	}
	_, ok := wf.Jobs[job]
	return ok
}

// TestMatrixRowsHaveCoverage is the #473 structural check: "a row in #472
// with no corresponding job should fail a structural check, so the two
// cannot drift apart." A matrix row with no matrixCoverage entry fails here;
// so does a matrixCoverage entry naming a workflow/job that doesn't exist.
func TestMatrixRowsHaveCoverage(t *testing.T) {
	for _, row := range matrixRows(t) {
		cov, ok := matrixCoverage[row]
		if !ok {
			t.Errorf("matrix row %q has no compatci.matrixCoverage entry -- "+
				"add one naming the CI job that tests this row's minimum, or a "+
				"Note explaining why it doesn't need one", row)
			continue
		}
		if cov.workflow == "" && cov.job == "" && cov.note == "" {
			t.Errorf("compatci.matrixCoverage[%q] is empty -- name a Workflow/Job or a Note", row)
			continue
		}
		if cov.workflow != "" {
			if cov.job == "" {
				t.Errorf("compatci.matrixCoverage[%q] names Workflow %q with no Job", row, cov.workflow)
			} else if !jobExists(t, cov.workflow, cov.job) {
				t.Errorf("compatci.matrixCoverage[%q] names job %q, which does not exist in "+
					".github/workflows/%s", row, cov.job, cov.workflow)
			}
		}
		if cov.belowFloorJob != "" {
			wf := cov.belowFloorWorkflow
			if wf == "" {
				wf = cov.workflow
			}
			if wf == "" {
				t.Errorf("compatci.matrixCoverage[%q] names BelowFloorJob %q with no workflow to find it in",
					row, cov.belowFloorJob)
			} else if !jobExists(t, wf, cov.belowFloorJob) {
				t.Errorf("compatci.matrixCoverage[%q] names below-floor job %q, which does not exist in "+
					".github/workflows/%s", row, cov.belowFloorJob, wf)
			}
		}
	}

	// The reverse direction: a matrixCoverage entry for a row the doc no
	// longer has is stale and should be deleted, the same shape as
	// integrations' int02Coverage reverse check.
	rows := make(map[string]bool)
	for _, row := range matrixRows(t) {
		rows[row] = true
	}
	for name := range matrixCoverage {
		if !rows[name] {
			t.Errorf("compatci.matrixCoverage has an entry for %q, which is not a row in %s -- "+
				"delete the stale entry or fix the name", name, matrixDocRel)
		}
	}
}
