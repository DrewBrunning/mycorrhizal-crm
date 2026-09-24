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
