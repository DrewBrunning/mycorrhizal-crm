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

// writeReconcileWorkflow writes a minimal, valid nightly-failure-reconcile.yml
// with the given workflow_file matrix -- most tests in this file only care
// about the name-based (nightly-failure-alert.yml) drift check, so they need
// a reconcile file present that agrees with their scenario's scheduled set,
// or the newer file-based check would report its own (irrelevant) findings.
// Deliberately has no `on: schedule:` of its own: the real file does (see
// nightly-failure-reconcile.yml itself, and TestRunAgainstCommittedFiles,
// which covers that it is correctly registered in nightly-failure-alert.yml
// for exactly this reason), but giving every synthetic fixture here the same
// trigger would make each test also have to register "Nightly failure alert
// reconciliation" by name -- an assertion belonging to its own dedicated
// test, not every unrelated fixture in this file.
func writeReconcileWorkflow(t *testing.T, dir string, files ...string) {
	t.Helper()
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = `"` + f + `"`
	}
	body := "name: Nightly failure alert reconciliation\non:\n  workflow_dispatch: {}\n" +
		"jobs:\n  reconcile:\n    strategy:\n      matrix:\n        workflow_file: [" + strings.Join(quoted, ", ") + "]\n"
	writeWorkflow(t, dir, "nightly-failure-reconcile.yml", body)
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
	writeReconcileWorkflow(t, root, "unit-tests.yml", "chaos-tests.yml")

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
	writeReconcileWorkflow(t, root, "unit-tests.yml")

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
	writeReconcileWorkflow(t, root, "unit-tests.yml")

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
	writeReconcileWorkflow(t, root)

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not parse")
}

// TestRunAtMissingReconcileFile mirrors TestRunAtMissingAlertFile: the
// reconciliation backstop's presence is just as mandatory as the alert
// file's -- a repo with no nightly-failure-reconcile.yml has no self-healing
// path for a dropped `workflow_run` delivery.
func TestRunAtMissingReconcileFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: []
    types: [completed]
`)

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "nightly-failure-reconcile.yml does not exist")
}

// TestRunAtMalformedReconcileFile covers ReconcileWorkflowFiles's
// parse-error branch: the reconciliation workflow file exists but isn't
// valid YAML.
func TestRunAtMalformedReconcileFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: []
    types: [completed]
`)
	writeWorkflow(t, root, "nightly-failure-reconcile.yml", "not: [valid yaml")

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not parse")
}

// TestRunAtReportsMissingFileRegistration is CheckFileDrift's regression: a
// scheduled workflow whose filename never made it into
// nightly-failure-reconcile.yml's workflow_file matrix, so a dropped
// `workflow_run` delivery for it would never self-heal.
func TestRunAtReportsMissingFileRegistration(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "nightly-failure-alert.yml", `
name: Nightly failure alert
on:
  workflow_run:
    workflows: ["Unit Tests", "Chaos Tests (failure injection)"]
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
	writeReconcileWorkflow(t, root, "unit-tests.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"chaos-tests.yml"`)
	assert.Contains(t, out.String(), "never self-heal")
}

// TestRunAtReportsStaleFileRegistration is the mirror case: a filename in
// the reconciliation matrix that no longer has a schedule: trigger (or was
// renamed) -- a stale entry that polls nothing useful.
func TestRunAtReportsStaleFileRegistration(t *testing.T) {
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
	writeReconcileWorkflow(t, root, "unit-tests.yml", "renamed-workflow.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"renamed-workflow.yml"`)
	assert.Contains(t, out.String(), "stale entry")
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
	writeReconcileWorkflow(t, root)
	workflowsPath := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(filepath.Join(workflowsPath, "a-subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsPath, "README.md"), []byte("not yaml at all"), 0o644))

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 0, code, out.String())
	assert.Contains(t, out.String(), "0 scheduled workflow(s)")
}
