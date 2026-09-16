package schemafixture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// gateRootFuncs are the functions whose call means "this test only runs
// under MYCORRHIZAL_LARGE_TESTS=1" — the actual gate, defined once in
// large_mig_test.go's largeTestsEnabled.
var gateRootFuncs = map[string]bool{
	"largeTestsEnabled": true,
}

// migrationWorkflow is the subset of migration-tests.yml this test cares
// about: the large-dataset job's steps, each carrying its env and run text.
type migrationWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Env map[string]string `yaml:"env"`
			Run string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestGatedLargeDatasetTestsAreWiredIntoCI is the regression test for issue
// #923: TestCrossVersionRestoreAtScale skips itself unless
// MYCORRHIZAL_LARGE_TESTS=1 is set (see largeTestsEnabled, called via
// buildLargeFixture, in large_mig_test.go), and the ONLY thing that ever sets
// that variable is migration-tests.yml's large-dataset CI job — but that
// job's `-run` alternation never named the test, so it never ran anywhere:
// not in the default suite (self-skipped), not in the gated job (not
// selected by -run). `go test ./internal/schemafixture/...` alone cannot
// catch this: the test is well-formed and passes when invoked directly by
// name, and the break lives entirely in the workflow YAML, outside the Go
// module.
//
// This test statically finds every top-level test function in this package
// that is gated — directly, or transitively through a helper such as
// buildLargeFixture — behind MYCORRHIZAL_LARGE_TESTS=1, then checks that
// migration-tests.yml's large-dataset job has some MYCORRHIZAL_LARGE_TESTS:
// "1" step whose `run` text names each one. A newly added gated test that
// nobody wires into the workflow now fails here instead of silently never
// running, in CI or otherwise.
func TestGatedLargeDatasetTestsAreWiredIntoCI(t *testing.T) {
	dir, err := findRepoRoot()
	require.NoError(t, err)

	pkgDir := filepath.Join(dir, "backend", "internal", "schemafixture")
	gated := gatedTestFuncNames(t, pkgDir)
	require.Contains(t, gated, "TestCrossVersionRestoreAtScale",
		"sanity check: the known MYCORRHIZAL_LARGE_TESTS-gated test from issue #923 must be detected by this scan")

	wfPath := filepath.Join(dir, ".github", "workflows", "migration-tests.yml")
	wfBytes, err := os.ReadFile(wfPath) // #nosec G304 -- fixed repo-relative path, not request input
	require.NoError(t, err)

	runBlob := largeDatasetGatedRunSteps(t, wfBytes)

	for _, name := range gated {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		assert.Truef(t, re.MatchString(runBlob),
			"%s is gated behind MYCORRHIZAL_LARGE_TESTS=1 (directly or via a helper) but migration-tests.yml's "+
				"large-dataset job has no MYCORRHIZAL_LARGE_TESTS: \"1\" step whose `run` names it — it will never "+
				"run anywhere. Add it to that job's -run alternation.", name)
	}
}

// gatedTestFuncNames returns, sorted, every top-level `func TestXxx(t
// *testing.T)` in pkgDir's _test.go files whose body — or a same-package
// helper it calls, one or more levels deep — calls a function in
// gateRootFuncs.
func gatedTestFuncNames(t *testing.T, pkgDir string) []string {
	t.Helper()

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	bodySource := map[string]string{} // func name -> its body's source text
	var testFuncs []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(pkgDir, e.Name())
		src, err := os.ReadFile(path) // #nosec G304 -- enumerated from a fixed repo-relative directory, not request input
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, path, src, 0)
		require.NoErrorf(t, err, "parsing %s", path)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			start := fset.Position(fn.Body.Lbrace).Offset
			end := fset.Position(fn.Body.Rbrace).Offset
			bodySource[fn.Name.Name] = string(src[start:end])

			if strings.HasPrefix(fn.Name.Name, "Test") &&
				fn.Type.Params != nil && len(fn.Type.Params.List) == 1 {
				testFuncs = append(testFuncs, fn.Name.Name)
			}
		}
	}
	require.NotEmpty(t, testFuncs, "found no top-level Test functions under %s — is the path right?", pkgDir)

	// Fixed point over the same-package call graph: a function is "gated"
	// if it calls a gate-root function, or another function already known
	// to be gated (this is what makes buildLargeFixture's callers gated
	// through one level of indirection, without hand-listing helpers).
	gatedFuncs := map[string]bool{}
	for name := range gateRootFuncs {
		gatedFuncs[name] = true
	}
	for changed := true; changed; {
		changed = false
		for name, body := range bodySource {
			if gatedFuncs[name] {
				continue
			}
			for g := range gatedFuncs {
				if regexp.MustCompile(`\b` + regexp.QuoteMeta(g) + `\s*\(`).MatchString(body) {
					gatedFuncs[name] = true
					changed = true
					break
				}
			}
		}
	}

	var result []string
	for _, name := range testFuncs {
		if gatedFuncs[name] {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

// largeDatasetGatedRunSteps parses migration-tests.yml and returns the
// concatenated `run` text of every step in the large-dataset job that sets
// MYCORRHIZAL_LARGE_TESTS: "1" — the only steps that could possibly execute a
// gated test.
func largeDatasetGatedRunSteps(t *testing.T, wfBytes []byte) string {
	t.Helper()

	var wf migrationWorkflow
	require.NoError(t, yaml.Unmarshal(wfBytes, &wf))

	job, ok := wf.Jobs["large-dataset"]
	require.True(t, ok, "migration-tests.yml must have a large-dataset job")

	var blob strings.Builder
	for _, step := range job.Steps {
		if step.Env["MYCORRHIZAL_LARGE_TESTS"] == "1" {
			blob.WriteString(step.Run)
			blob.WriteString("\n")
		}
	}
	require.NotZero(t, blob.Len(),
		"migration-tests.yml's large-dataset job has no step with MYCORRHIZAL_LARGE_TESTS: \"1\" set")
	return blob.String()
}
