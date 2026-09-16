// Version-number coupling, split from #923 into issue #997.
//
// TestMatrixRowsHaveCoverage (coverage_test.go) proves a matrix row's named
// CI job still EXISTS. It never proves that job still tests the SAME version
// the doc states as the floor -- min-version-tests.yml hardcodes each floor
// as its own literal (`go-version: '1.26.0'`, `node-version: '22.22.2'`, the
// `yarn-1.22.0` download, `api-level: 26` in android-tests.yml), entirely
// independently of docs/development/supported-runtime-matrix.md's prose. A
// PR that bumps one without the other passes both the doc's own claim ("this
// table and the jobs below cannot drift apart silently", matrix.md:71-77)
// and TestMatrixRowsHaveCoverage today, because that test only ever checked
// the job ID, not its pinned version. These tests parse both sides as plain
// text and fail the moment they disagree -- the same "parse the real source
// of truth, don't trust a comment" shape as ci_claims_test.go's checks.
package compatci

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// minVersionsWorkflowRel is min-version-tests.yml (bare filename, as
// jobExists/readWorkflow expect -- workflowsDirRel is joined on separately).
// androidWorkflowRel is android-tests.yml: the Android floor's job lives in a
// separate workflow from the others (it shares android-tests.yml's
// emulator/backend setup, per the matrix doc's own coverage table).
const (
	minVersionsWorkflowRel = "min-version-tests.yml"
	androidWorkflowRel     = "android-tests.yml"
)

// readWorkflow returns the full text of a .github/workflows/ file.
func readWorkflow(t *testing.T, file string) string {
	t.Helper()
	path := filepath.Join(workflowsDirRel, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading workflow %s: %v", path, err)
	}
	return string(data)
}

// jobBlock extracts one job's body out of a workflow file's text: from its
// "  <jobID>:" header (2-space indent, the level every job in this repo's
// workflows is declared at) up to the next sibling job header or EOF. Scoping
// the version-pattern searches below to just this block -- instead of
// grepping the whole file -- means a version literal appearing in a
// DIFFERENT job (e.g. go-below-minimum's deliberately-one-patch-lower Go
// version) can never be mistaken for the job under test.
func jobBlock(t *testing.T, workflowText, workflowFile, jobID string) string {
	t.Helper()
	header := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(jobID) + `:\s*$`)
	loc := header.FindStringIndex(workflowText)
	if loc == nil {
		t.Fatalf("%s: found no %q job header (expected a line matching \"  %s:\") -- did the job get renamed?",
			workflowFile, jobID, jobID)
	}
	rest := workflowText[loc[1]:]
	nextJob := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:\s*$`)
	if next := nextJob.FindStringIndex(rest); next != nil {
		return rest[:next[0]]
	}
	return rest
}

// matrixFloor runs pattern against the committed matrix doc and returns the
// single captured group, failing loudly if the row's format ever changes out
// from under the regex (the same failure-mode guard matrixRowPattern's own
// callers use).
func matrixFloor(t *testing.T, pattern *regexp.Regexp, describe string) string {
	t.Helper()
	doc, err := os.ReadFile(matrixDocRel)
	if err != nil {
		t.Fatalf("reading %s: %v", matrixDocRel, err)
	}
	m := pattern.FindSubmatch(doc)
	if m == nil {
		t.Fatalf("%s: found no %s -- did the table row's format change? update the pattern in version_test.go",
			matrixDocRel, describe)
	}
	return string(m[1])
}

// --- Go ----------------------------------------------------------------

var matrixGoFloorPattern = regexp.MustCompile("\\|\\s*\\*\\*Go\\*\\*\\s*\\|\\s*`([0-9]+\\.[0-9]+\\.[0-9]+)`")

var jobGoVersionPattern = regexp.MustCompile(`go-version:\s*'([0-9]+\.[0-9]+\.[0-9]+)'`)

// TestGoFloorMatchesMinVersionJob is the #997 regression guard: the matrix
// doc's Go row and min-version-tests.yml's go-minimum job must name the
// exact same version, not merely both exist.
func TestGoFloorMatchesMinVersionJob(t *testing.T) {
	docFloor := matrixFloor(t, matrixGoFloorPattern, "a `| **Go** | `X.Y.Z` ...` row")

	wf := readWorkflow(t, minVersionsWorkflowRel)
	block := jobBlock(t, wf, minVersionsWorkflowRel, "go-minimum")
	m := jobGoVersionPattern.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("%s: go-minimum job has no go-version: '...' step -- did setup-go's input change? "+
			"update jobGoVersionPattern in version_test.go", minVersionsWorkflowRel)
	}
	if docFloor != m[1] {
		t.Errorf("Go floor drift: %s states the floor as %q, but %s's go-minimum job installs %q -- "+
			"raising or lowering the floor means updating BOTH (and it's a breaking change under "+
			"docs/breaking-change-policy.md, MAINT-02)", matrixDocRel, docFloor, minVersionsWorkflowRel, m[1])
	}
}

// --- Node.js -------------------------------------------------------------

var matrixNodeFloorPattern = regexp.MustCompile("\\|\\s*\\*\\*Node\\.js\\*\\*[^|]*\\|\\s*`>=([0-9]+\\.[0-9]+\\.[0-9]+)`")

var jobNodeVersionPattern = regexp.MustCompile(`node-version:\s*'([0-9]+\.[0-9]+\.[0-9]+)'`)

// TestNodeFloorMatchesMinVersionJob is the Node.js half of the same #997
// coupling: the matrix doc's Node.js row and node-yarn-minimum's pinned
// setup-node input must agree.
func TestNodeFloorMatchesMinVersionJob(t *testing.T) {
	docFloor := matrixFloor(t, matrixNodeFloorPattern, "a `| **Node.js** ... | `>=X.Y.Z` ...` row")

	wf := readWorkflow(t, minVersionsWorkflowRel)
	block := jobBlock(t, wf, minVersionsWorkflowRel, "node-yarn-minimum")
	m := jobNodeVersionPattern.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("%s: node-yarn-minimum job has no node-version: '...' step -- did setup-node's input change? "+
			"update jobNodeVersionPattern in version_test.go", minVersionsWorkflowRel)
	}
	if docFloor != m[1] {
		t.Errorf("Node.js floor drift: %s states the floor as %q, but %s's node-yarn-minimum job installs "+
			"%q -- raising or lowering the floor means updating BOTH (and it's a breaking change under "+
			"docs/breaking-change-policy.md, MAINT-02)", matrixDocRel, docFloor, minVersionsWorkflowRel, m[1])
	}
}

// --- Yarn ------------------------------------------------------------------

var matrixYarnFloorPattern = regexp.MustCompile("\\|\\s*\\*\\*Yarn\\*\\*\\s*\\|[^`]*`>=([0-9]+\\.[0-9]+\\.[0-9]+)`")

// jobYarnVersionPattern matches the pinned Yarn Classic release URL
// node-yarn-minimum downloads by hand (Yarn Classic has no setup-action
// version input the way Go/Node do) -- see that job's own comment on why
// it's a checksummed direct download rather than a marketplace action.
var jobYarnVersionPattern = regexp.MustCompile(`yarnpkg/yarn/releases/download/v([0-9]+\.[0-9]+\.[0-9]+)/yarn-v([0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz`)

// TestYarnFloorMatchesMinVersionJob is the Yarn half: the matrix doc's Yarn
// row and node-yarn-minimum's hand-pinned download must agree.
func TestYarnFloorMatchesMinVersionJob(t *testing.T) {
	docFloor := matrixFloor(t, matrixYarnFloorPattern, "a `| **Yarn** | Classic v1, `>=X.Y.Z` ...` row")

	wf := readWorkflow(t, minVersionsWorkflowRel)
	block := jobBlock(t, wf, minVersionsWorkflowRel, "node-yarn-minimum")
	m := jobYarnVersionPattern.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("%s: node-yarn-minimum job has no recognizable yarnpkg/yarn release download -- did the "+
			"pinning approach change? update jobYarnVersionPattern in version_test.go", minVersionsWorkflowRel)
	}
	if m[1] != m[2] {
		t.Fatalf("%s: node-yarn-minimum job's download URL and archive filename name different Yarn "+
			"versions (%q vs %q) -- fix the download step before this test can trust it", minVersionsWorkflowRel, m[1], m[2])
	}
	if docFloor != m[1] {
		t.Errorf("Yarn floor drift: %s states the floor as %q, but %s's node-yarn-minimum job downloads "+
			"%q -- raising or lowering the floor means updating BOTH (and it's a breaking change under "+
			"docs/breaking-change-policy.md, MAINT-02), including re-pinning that step's sha256sum", matrixDocRel, docFloor, minVersionsWorkflowRel, m[1])
	}
}

// --- Android ---------------------------------------------------------------

var matrixAndroidFloorPattern = regexp.MustCompile("\\|\\s*\\*\\*Android\\*\\*\\s*\\|\\s*`minSdk ([0-9]+)`")

var jobAPILevelPattern = regexp.MustCompile(`api-level:\s*(\d+)`)

// TestAndroidFloorMatchesMinVersionJob is the Android half: the matrix doc's
// minSdk and android-tests.yml's android-e2e-min-sdk emulator API level must
// agree (the below-floor job, android-below-min-sdk, is deliberately one
// level under this and is not asserted here).
func TestAndroidFloorMatchesMinVersionJob(t *testing.T) {
	docFloor := matrixFloor(t, matrixAndroidFloorPattern, "a `| **Android** | `minSdk N` ...` row")

	wf := readWorkflow(t, androidWorkflowRel)
	block := jobBlock(t, wf, androidWorkflowRel, "android-e2e-min-sdk")
	m := jobAPILevelPattern.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("%s: android-e2e-min-sdk job has no api-level: N -- did the emulator-runner input change? "+
			"update jobAPILevelPattern in version_test.go", androidWorkflowRel)
	}
	if docFloor != m[1] {
		t.Errorf("Android floor drift: %s states minSdk %q, but %s's android-e2e-min-sdk job boots API "+
			"level %q -- raising or lowering the floor means updating BOTH (and it's a breaking change "+
			"under docs/breaking-change-policy.md, MAINT-02)", matrixDocRel, docFloor, androidWorkflowRel, m[1])
	}
}
