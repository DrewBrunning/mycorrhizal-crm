package nightlyalert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduledWorkflowNames_CollectsScheduledOnly(t *testing.T) {
	files := map[string][]byte{
		"unit-tests.yml": []byte(`
name: Unit Tests
on:
  push: {}
`),
		"nightly.yml": []byte(`
name: Nightly Suite
on:
  schedule:
    - cron: '5 2 * * *'
`),
	}
	names, findings := ScheduledWorkflowNames(files)
	assert.Equal(t, []string{"Nightly Suite"}, names)
	assert.Empty(t, findings)
}

func TestScheduledWorkflowNames_DedupesSameName(t *testing.T) {
	files := map[string][]byte{
		"a.yml": []byte(`
name: Dup
on:
  schedule:
    - cron: '5 2 * * *'
`),
		"b.yml": []byte(`
name: Dup
on:
  schedule:
    - cron: '6 2 * * *'
`),
	}
	names, findings := ScheduledWorkflowNames(files)
	assert.Equal(t, []string{"Dup"}, names)
	assert.Empty(t, findings)
}

func TestScheduledWorkflowNames_ParseErrorIsAFindingNotAPanic(t *testing.T) {
	files := map[string][]byte{
		"broken.yml": []byte("not: [valid yaml"),
	}
	names, findings := ScheduledWorkflowNames(files)
	assert.Empty(t, names)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], "broken.yml does not parse")
}

func TestScheduledWorkflowNames_ScheduledWithNoNameIsAFinding(t *testing.T) {
	files := map[string][]byte{
		"nameless.yml": []byte(`
on:
  schedule:
    - cron: '5 2 * * *'
`),
	}
	names, findings := ScheduledWorkflowNames(files)
	assert.Empty(t, names)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], "has a schedule: trigger but no name:")
}

func TestRegisteredWorkflowNames_ReturnsSortedList(t *testing.T) {
	yaml := []byte(`
name: Nightly failure alert
on:
  workflow_run:
    workflows: ["B Workflow", "A Workflow"]
    types: [completed]
`)
	names, err := RegisteredWorkflowNames(yaml)
	require.NoError(t, err)
	assert.Equal(t, []string{"A Workflow", "B Workflow"}, names)
}

func TestRegisteredWorkflowNames_ParseError(t *testing.T) {
	_, err := RegisteredWorkflowNames([]byte("not: [valid yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), AlertWorkflowFile+" does not parse")
}

func TestCheckDrift_NoFindingsWhenSetsMatch(t *testing.T) {
	findings := CheckDrift([]string{"A", "B"}, []string{"B", "A"})
	assert.Empty(t, findings)
}

func TestCheckDrift_ScheduledButNotRegistered(t *testing.T) {
	findings := CheckDrift([]string{"A"}, nil)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `does not list "A"`)
}

func TestCheckDrift_RegisteredButNotScheduled(t *testing.T) {
	findings := CheckDrift(nil, []string{"A"})
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `lists "A"`)
	assert.Contains(t, findings[0], "stale entry")
}

func TestCheckDrift_BothDirectionsReported(t *testing.T) {
	findings := CheckDrift([]string{"Only Scheduled"}, []string{"Only Registered"})
	require.Len(t, findings, 2)
	joined := findings[0] + findings[1]
	assert.Contains(t, joined, "Only Registered")
	assert.Contains(t, joined, "Only Scheduled")
}

func TestScheduledWorkflowFiles_CollectsScheduledFilesOnly(t *testing.T) {
	files := map[string][]byte{
		"unit-tests.yml": []byte(`
name: Unit Tests
on:
  push: {}
`),
		"nightly.yml": []byte(`
name: Nightly Suite
on:
  schedule:
    - cron: '5 2 * * *'
`),
	}
	paths, findings := ScheduledWorkflowFiles(files)
	assert.Equal(t, []string{"nightly.yml"}, paths)
	assert.Empty(t, findings)
}

func TestScheduledWorkflowFiles_ExcludesAlertAndReconcileFilesThemselves(t *testing.T) {
	files := map[string][]byte{
		AlertWorkflowFile: []byte(`
name: Nightly failure alert
on:
  workflow_run:
    workflows: []
    types: [completed]
`),
		ReconcileWorkflowFile: []byte(`
name: Nightly failure alert reconciliation
on:
  schedule:
    - cron: '30 5 * * *'
`),
		"nightly.yml": []byte(`
name: Nightly Suite
on:
  schedule:
    - cron: '5 2 * * *'
`),
	}
	paths, findings := ScheduledWorkflowFiles(files)
	assert.Equal(t, []string{"nightly.yml"}, paths)
	assert.Empty(t, findings)
}

func TestScheduledWorkflowFiles_ParseErrorIsAFindingNotAPanic(t *testing.T) {
	files := map[string][]byte{
		"broken.yml": []byte("not: [valid yaml"),
	}
	paths, findings := ScheduledWorkflowFiles(files)
	assert.Empty(t, paths)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], "broken.yml does not parse")
}

func TestReconcileWorkflowFiles_ReturnsSortedList(t *testing.T) {
	yaml := []byte(`
name: Nightly failure alert reconciliation
on:
  schedule:
    - cron: '30 5 * * *'
jobs:
  reconcile:
    strategy:
      matrix:
        workflow_file: ["b.yml", "a.yml"]
`)
	files, err := ReconcileWorkflowFiles(yaml)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.yml", "b.yml"}, files)
}

func TestReconcileWorkflowFiles_ParseError(t *testing.T) {
	_, err := ReconcileWorkflowFiles([]byte("not: [valid yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), ReconcileWorkflowFile+" does not parse")
}

func TestCheckFileDrift_NoFindingsWhenSetsMatch(t *testing.T) {
	findings := CheckFileDrift([]string{"a.yml", "b.yml"}, []string{"b.yml", "a.yml"})
	assert.Empty(t, findings)
}

func TestCheckFileDrift_ScheduledButNotReconciled(t *testing.T) {
	findings := CheckFileDrift([]string{"a.yml"}, nil)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `does not list "a.yml"`)
	assert.Contains(t, findings[0], "never self-heal")
}

func TestCheckFileDrift_ReconciledButNotScheduled(t *testing.T) {
	findings := CheckFileDrift(nil, []string{"a.yml"})
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0], `lists "a.yml"`)
	assert.Contains(t, findings[0], "stale entry")
}

func TestCheckFileDrift_BothDirectionsReported(t *testing.T) {
	findings := CheckFileDrift([]string{"only-scheduled.yml"}, []string{"only-reconciled.yml"})
	require.Len(t, findings, 2)
	joined := findings[0] + findings[1]
	assert.Contains(t, joined, "only-reconciled.yml")
	assert.Contains(t, joined, "only-scheduled.yml")
}
