package mainalert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/releasegates"
)

func reg(gates ...releasegates.Gate) releasegates.Registry {
	return releasegates.Registry{Gates: gates}
}

func perPR(name, workflow string) releasegates.Gate {
	return releasegates.Gate{Name: name, Workflow: workflow, Tier: "per-pr", Mandatory: true}
}

func TestExpectedWorkflowNames_ResolvesNamesFromPerPRGates(t *testing.T) {
	files := map[string][]byte{
		"unit-tests.yml": []byte("name: Unit Tests\non:\n  push:\n    branches: [main]\n"),
		"zizmor.yml":     []byte("name: GitHub Actions Security (zizmor)\non:\n  push:\n    branches: [main]\n"),
	}
	names, findings := ExpectedWorkflowNames(reg(
		perPR("Backend (Go)", "unit-tests.yml"),
		perPR("Frontend (Vitest)", "unit-tests.yml"),
		perPR("Scan workflows (zizmor)", "zizmor.yml"),
	), files)
	assert.Equal(t, []string{"GitHub Actions Security (zizmor)", "Unit Tests"}, names)
	assert.Empty(t, findings)
}

func TestExpectedWorkflowNames_IgnoresNonMandatoryAndNonPerPRGates(t *testing.T) {
	files := map[string][]byte{
		"unit-tests.yml":  []byte("name: Unit Tests\non:\n  push:\n    branches: [main]\n"),
		"scorecard.yml":   []byte("name: OpenSSF Scorecard\non:\n  schedule: []\n"),
		"min-version.yml": []byte("name: Minimum Versions\non:\n  push:\n    branches: [main]\n"),
	}
	names, findings := ExpectedWorkflowNames(reg(
		perPR("Backend (Go)", "unit-tests.yml"),
		releasegates.Gate{Name: "OpenSSF Scorecard", Workflow: "scorecard.yml", Tier: "advisory", Mandatory: false},
		releasegates.Gate{Name: "Test minimum supported versions", Workflow: "min-version.yml", Tier: "release-tier", Mandatory: true},
	), files)
	assert.Equal(t, []string{"Unit Tests"}, names)
	assert.Empty(t, findings)
}

func TestExpectedWorkflowNames_DedupesSharedWorkflow(t *testing.T) {
	files := map[string][]byte{
		"unit-tests.yml": []byte("name: Unit Tests\non:\n  push:\n    branches: [main]\n"),
	}
	names, findings := ExpectedWorkflowNames(reg(
		perPR("Backend (Go)", "unit-tests.yml"),
		perPR("Frontend (Vitest)", "unit-tests.yml"),
	), files)
	assert.Equal(t, []string{"Unit Tests"}, names)
	assert.Empty(t, findings)
}

func TestExpectedWorkflowNames_MissingWorkflowFileIsAFinding(t *testing.T) {
	names, findings := ExpectedWorkflowNames(reg(perPR("Backend (Go)", "ghost.yml")), map[string][]byte{})
	assert.Empty(t, names)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `"ghost.yml"`)
	assert.Contains(t, findings[0], "does not exist")
}

func TestExpectedWorkflowNames_ParseErrorIsAFindingNotAPanic(t *testing.T) {
	files := map[string][]byte{"broken.yml": []byte("not: [valid yaml")}
	names, findings := ExpectedWorkflowNames(reg(perPR("Backend (Go)", "broken.yml")), files)
	assert.Empty(t, names)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], "broken.yml does not parse")
}

func TestExpectedWorkflowNames_NamelessWorkflowIsAFinding(t *testing.T) {
	files := map[string][]byte{"nameless.yml": []byte("on:\n  push:\n    branches: [main]\n")}
	names, findings := ExpectedWorkflowNames(reg(perPR("Backend (Go)", "nameless.yml")), files)
	assert.Empty(t, names)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], "has no name:")
}

func TestRegisteredWorkflowNames_ReturnsSortedList(t *testing.T) {
	doc := []byte(`
name: Main failure alert
on:
  workflow_run:
    workflows: ["B Workflow", "A Workflow"]
    types: [completed]
`)
	names, err := RegisteredWorkflowNames(doc)
	require.NoError(t, err)
	assert.Equal(t, []string{"A Workflow", "B Workflow"}, names)
}

func TestRegisteredWorkflowNames_ParseError(t *testing.T) {
	_, err := RegisteredWorkflowNames([]byte("not: [valid yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), AlertWorkflowFile+" does not parse")
}

func TestCheckDrift_NoFindingsWhenExpectedIsRegistered(t *testing.T) {
	findings := CheckDrift([]string{"A", "B"}, []string{"B", "A", "Extra"})
	assert.Empty(t, findings)
}

func TestCheckDrift_ExpectedButNotRegistered(t *testing.T) {
	findings := CheckDrift([]string{"A"}, nil)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `does not list "A"`)
	assert.Contains(t, findings[0], "raises no alert")
}

func TestCheckDrift_RegisteredExtraIsNotAFinding(t *testing.T) {
	findings := CheckDrift(nil, []string{"Docker Build Check"})
	assert.Empty(t, findings)
}
