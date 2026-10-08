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
	require.Equalf(t, 0, run(&out), "mainalertcheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "mainalertcheck OK")
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

// writeRegistry writes a minimal but structurally valid .github/release-gates.json
// naming one mandatory per-pr gate per given workflow file.
func writeRegistry(t *testing.T, dir string, gateWorkflows ...string) {
	t.Helper()
	gates := ""
	for i, wf := range gateWorkflows {
		if i > 0 {
			gates += ",\n"
		}
		gates += `{"name":"Gate ` + string(rune('A'+i)) + `","workflow":"` + wf +
			`","check_context":"Gate ` + string(rune('A'+i)) + `","check_kind":"check_run","tier":"per-pr","mandatory":true,"release_gate":false,"criterion":"x"}`
	}
	body := `{"gates":[` + gates + `]}`
	p := filepath.Join(dir, ".github", "release-gates.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func writeMainAlert(t *testing.T, dir, names string) {
	t.Helper()
	writeWorkflow(t, dir, mainalertFileForTest, `
name: Main failure alert
on:
  workflow_run:
    workflows: [`+names+`]
    types: [completed]
`)
}

// mainalertFileForTest mirrors mainalert.AlertWorkflowFile without importing
// the package into the test's identifiers.
const mainalertFileForTest = "main-failure-alert.yml"

// TestRunAtReportsMissingRegistration is the regression this command exists
// to catch: a workflow carries a mandatory per-pr gate but nobody added it to
// main-failure-alert.yml's workflows: list, so a red main push run of it
// alerts no one.
func TestRunAtReportsMissingRegistration(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests"`)
	writeWorkflow(t, root, "unit-tests.yml", "name: Unit Tests\non:\n  push:\n    branches: [main]\n")
	writeWorkflow(t, root, "zizmor.yml", "name: GitHub Actions Security (zizmor)\non:\n  push:\n    branches: [main]\n")
	writeRegistry(t, root, "unit-tests.yml", "zizmor.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"GitHub Actions Security (zizmor)"`)
	assert.Contains(t, out.String(), "raises no alert")
}

func TestRunAtOKWhenAllRegistered(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests", "Docker Build Check"`)
	writeWorkflow(t, root, "unit-tests.yml", "name: Unit Tests\non:\n  push:\n    branches: [main]\n")
	writeWorkflow(t, root, "docker-build-check.yml", "name: Docker Build Check\non:\n  push:\n    branches: [main]\n")
	writeRegistry(t, root, "unit-tests.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 0, code, out.String())
	assert.Contains(t, out.String(), "1 mandatory per-pr workflow(s)")
}

func TestRunAtMissingAlertFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, "unit-tests.yml", "name: Unit Tests\non:\n  push:\n    branches: [main]\n")
	writeRegistry(t, root, "unit-tests.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not exist")
}

func TestRunAtMissingWorkflowsDir(t *testing.T) {
	root := t.TempDir()

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "cannot read "+workflowsDir)
}

func TestRunAtMalformedAlertFile(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, mainalertFileForTest, "not: [valid yaml")
	writeRegistry(t, root, "unit-tests.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "does not parse")
}

func TestRunAtMissingRegistryFile(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests"`)

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 2, code)
	assert.Contains(t, out.String(), "cannot read .github/release-gates.json")
}

func TestRunAtMalformedRegistryFile(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests"`)
	p := filepath.Join(root, ".github", "release-gates.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("not json"), 0o644))

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "does not parse")
}

func TestRunAtMissingGateWorkflowFile(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests"`)
	writeRegistry(t, root, "ghost.yml")

	var out bytes.Buffer
	code := runAt(&out, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), `"ghost.yml"`)
	assert.Contains(t, out.String(), "does not exist")
}

// TestRunAtSkipsSubdirectoriesAndNonYAMLFiles covers the two directory-entry
// `continue` branches: a subdirectory and a non-.yml/.yaml file under
// .github/workflows must both be ignored rather than tripping the YAML parser.
func TestRunAtSkipsSubdirectoriesAndNonYAMLFiles(t *testing.T) {
	root := t.TempDir()
	writeMainAlert(t, root, `"Unit Tests"`)
	writeWorkflow(t, root, "unit-tests.yml", "name: Unit Tests\non:\n  push:\n    branches: [main]\n")
	writeRegistry(t, root, "unit-tests.yml")
	workflowsPath := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(filepath.Join(workflowsPath, "a-subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsPath, "README.md"), []byte("not yaml at all"), 0o644))

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 0, code, out.String())
	assert.Contains(t, out.String(), "1 mandatory per-pr workflow(s)")
}
