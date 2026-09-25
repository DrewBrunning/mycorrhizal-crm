package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAgainstCommittedFiles(t *testing.T) {
	var out bytes.Buffer
	require.Equalf(t, 0, run(&out), "nightlyalertcheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "nightlyalertcheck OK")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
}

func writeWorkflow(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, ".github", "workflows", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

// TestRunAtReportsMissingRegistration is the regression this whole command
// exists to catch: a new workflow gains a schedule: trigger but nobody adds
// it to nightly-failure-alert.yml's workflows: list.
func TestRunAtReportsMissingRegistration(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: ["Unit Tests"]
    types: [completed]
`)
	writeWorkflow(t, root, "unit-tests.yml", `
name: Unit Tests
on:
  schedule:
    - cron: '5 2 * * *'
`)
	writeWorkflow(t, root, "chaos-tests.yml", `
name: Chaos Tests (failure injection)
on:
  schedule:
    - cron: '55 3 * * *'
`)

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"Chaos Tests (failure injection)"`)
	assert.Contains(t, out.String(), "raises no alert")
}

// TestRunAtReportsStaleRegistration is the mirror case: a workflow was
// renamed or lost its schedule: trigger, but the alert file still lists the
// old name -- a silently dead entry rather than a caught rename.
func TestRunAtReportsStaleRegistration(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: ["Unit Tests", "Old Workflow Name"]
    types: [completed]
`)
	writeWorkflow(t, root, "unit-tests.yml", `
name: Unit Tests
on:
  schedule:
    - cron: '5 2 * * *'
`)

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"Old Workflow Name"`)
	assert.Contains(t, out.String(), "stale entry")
}

func TestRunAtOKWhenSetsMatch(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: ["Unit Tests"]
    types: [completed]
`)
	writeWorkflow(t, root, "unit-tests.yml", `
name: Unit Tests
on:
  schedule:
    - cron: '5 2 * * *'
`)
	writeWorkflow(t, root, "not-scheduled.yml", `
name: Not scheduled
on:
  push:
    branches: [main]
`)

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 0, code, out.String())
	assert.Contains(t, out.String(), "1 scheduled workflow(s)")
}

func TestRunAtMissingAlertFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "unit-tests.yml", `
name: Unit Tests
on:
  schedule:
    - cron: '5 2 * * *'
`)

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not exist")
}

// TestRunAtMissingWorkflowsDir covers the os.ReadDir failure branch: a repo
// root with no .github/workflows directory at all.
func TestRunAtMissingWorkflowsDir(t *testing.T) {
	root := t.TempDir()

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "cannot read "+workflowsDir)
}

// TestRunAtMalformedAlertFile covers RegisteredWorkflowNames's parse-error
// branch: the alert workflow file exists but isn't valid YAML.
func TestRunAtMalformedAlertFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", "not: [valid yaml")

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not parse")
}

// TestRunAtSkipsSubdirectoriesAndNonYAMLFiles covers the two directory-entry
// `continue` branches: a subdirectory (e.g. a stray editor swap dir) and a
// non-.yml/.yaml file under .github/workflows must both be ignored rather
// than tripping the YAML parser.
func TestRunAtSkipsSubdirectoriesAndNonYAMLFiles(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: []
    types: [completed]
`)
	workflowsPath := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(filepath.Join(workflowsPath, "a-subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsPath, "README.md"), []byte("not yaml at all"), 0o644))

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 0, code, out.String())
	assert.Contains(t, out.String(), "0 scheduled workflow(s)")
}
