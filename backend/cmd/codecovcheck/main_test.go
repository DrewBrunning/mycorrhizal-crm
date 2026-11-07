package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAgainstCommittedFiles(t *testing.T) {
	var out bytes.Buffer
	code := run(&out)
	require.Equalf(t, 0, code, "codecovcheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "codecov.yml OK")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
}

func TestRunSuccessMessageShape(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 0, run(&out))
	line := strings.TrimSpace(out.String())
	assert.True(t, strings.HasPrefix(line, "codecov.yml OK:"), line)
}

const fixtureDoc = "<!-- codecov-patch-status:begin -->\n\n```yaml\ncoverage:\n  status:\n    patch:\n      backend:\n        target: 95%\n        threshold: 5%\n        flags: [backend]\n        only_pulls: true\n```\n\n<!-- codecov-patch-status:end -->\n"

// fixtureStubWorkflow is a minimal unit-tests.yml carrying the issue #1188
// codecov-patch-stub job that codecovcheck also reads.
func fixtureStubWorkflow(areas string) string {
	return "jobs:\n  codecov-patch-stub:\n    if: >-\n      github.event_name == 'pull_request' &&\n" +
		"      needs.changes.outputs.backend != 'true' &&\n      needs.changes.outputs.frontend != 'true' &&\n" +
		"      needs.changes.outputs.android != 'true' &&\n      needs.changes.outputs.openapi != 'true' &&\n" +
		"      needs.changes.outputs.workflows != 'true'\n    steps:\n      - name: Post\n        env:\n          PATCH_AREAS: " + areas + "\n        run: echo\n"
}

// fixturePosterWorkflow is a minimal codecov-patch-stub-workflow-run.yml (the
// fork/Dependabot workflow_run poster) carrying its `post` job's PATCH_AREAS.
func fixturePosterWorkflow(areas string) string {
	return "jobs:\n  post:\n    steps:\n      - name: Post\n        env:\n          PATCH_AREAS: " + areas + "\n        run: echo\n"
}

// TestRunAtReportsTargetDrift exercises the "target changed without a
// matching doc change" finding end to end through runAt, the same fixture
// pattern cmd/governancecheck's test uses.
func TestRunAtReportsTargetDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 50%
        threshold: 5%
        flags: [backend]
        only_pulls: true
ignore:
  # a bare entry, no per-entry comment
  - "a/**"
  - "b/**"
`)
	write("docs/development/coverage.md", fixtureDoc)
	write(".github/workflows/unit-tests.yml", fixtureStubWorkflow("backend"))
	write(".github/workflows/codecov-patch-stub-workflow-run.yml", fixturePosterWorkflow("backend"))

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	s := out.String()
	assert.Contains(t, s, `target "50%" != docs/development/coverage.md target "95%"`)
	assert.Contains(t, s, `entry "b/**"`)
	assert.Contains(t, s, "codecov.yml finding(s)")
}

func TestRunAtMissingFileExits2(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, runAt(&out, t.TempDir()))
	assert.Contains(t, out.String(), "cannot read")
}

func TestRunAtCleanFixturePasses(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
ignore:
  # justified
  - "a/**"
`)
	write("docs/development/coverage.md", fixtureDoc)
	write(".github/workflows/unit-tests.yml", fixtureStubWorkflow("backend"))
	write(".github/workflows/codecov-patch-stub-workflow-run.yml", fixturePosterWorkflow("backend"))

	var out bytes.Buffer
	require.Equal(t, 0, runAt(&out, root))
}

// TestRunAtReportsStubDrift exercises the issue #1188 finding end to end: a
// codecov.yml patch area the codecov-patch-stub job does not post for would
// be stranded on a PR that uploads no coverage.
func TestRunAtReportsStubDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
`)
	write("docs/development/coverage.md", fixtureDoc)
	// Stub omits "backend" -- exactly the stranding drift.
	write(".github/workflows/unit-tests.yml", fixtureStubWorkflow("frontend"))
	write(".github/workflows/codecov-patch-stub-workflow-run.yml", fixturePosterWorkflow("backend"))

	var out bytes.Buffer
	require.Equal(t, 1, runAt(&out, root))
	assert.Contains(t, out.String(), "codecov-patch-stub's PATCH_AREAS does not list it")
}

// TestRunAtReportsPosterDrift pins the fork/Dependabot workflow_run poster to
// codecov.yml's patch areas, like the unit-tests.yml stub: an area it does not
// post for would be stranded on a fork PR that uploads no coverage.
func TestRunAtReportsPosterDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
`)
	write("docs/development/coverage.md", fixtureDoc)
	write(".github/workflows/unit-tests.yml", fixtureStubWorkflow("backend"))
	write(".github/workflows/codecov-patch-stub-workflow-run.yml", fixturePosterWorkflow("frontend"))

	var out bytes.Buffer
	require.Equal(t, 1, runAt(&out, root))
	assert.Contains(t, out.String(), "codecov-patch-stub's PATCH_AREAS does not list it")
}

// TestRunAtReportsStubGuardDrift: dropping an upload-suite filter from the
// stub's if: makes it disagree with the poster's verdict.
func TestRunAtReportsStubGuardDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
`)
	write("docs/development/coverage.md", fixtureDoc)
	write(".github/workflows/unit-tests.yml", strings.Replace(fixtureStubWorkflow("backend"), "needs.changes.outputs.openapi != 'true' &&", "", 1))
	write(".github/workflows/codecov-patch-stub-workflow-run.yml", fixturePosterWorkflow("backend"))

	var out bytes.Buffer
	require.Equal(t, 1, runAt(&out, root))
	assert.Contains(t, out.String(), "needs.changes.outputs.openapi")
}

// TestRunAtMissingPosterExits2: the workflow_run poster must exist -- deleting
// it would strand the required contexts on fork PRs again.
func TestRunAtMissingPosterExits2(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", "coverage:\n  status:\n    patch:\n      backend:\n        target: 95%\n        threshold: 5%\n        flags: [backend]\n        only_pulls: true\n")
	write("docs/development/coverage.md", fixtureDoc)
	write(".github/workflows/unit-tests.yml", fixtureStubWorkflow("backend"))

	var out bytes.Buffer
	assert.Equal(t, 2, runAt(&out, root))
	assert.Contains(t, out.String(), "codecov-patch-stub-workflow-run.yml")
}
