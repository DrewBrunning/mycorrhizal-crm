package compatci

// Issue #1687: dorny/test-reporter v3 has no `sha` input and, under
// workflow_run, attaches the check run to the PR's *current* head
// (workflow_run.pull_requests[].head.sha) rather than the commit the
// triggering run tested. test-report.yml therefore gates the job so a
// superseded run publishes nothing. This pins that gate.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func testReportJobIf(t *testing.T, src []byte) string {
	t.Helper()
	var w struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(src, &w))
	job, ok := w.Jobs["report"]
	require.True(t, ok, "test-report.yml has no report job")
	return job.If
}

func TestTestReportSkipsSupersededAndCancelledRuns(t *testing.T) {
	src := []byte(readWorkflow(t, "test-report.yml"))
	cond := testReportJobIf(t, src)
	assert.Contains(t, cond, "workflow_run.conclusion != 'cancelled'")
	assert.Contains(t, cond, "workflow_run.pull_requests[0].head.sha == github.event.workflow_run.head_sha")
	assert.Contains(t, cond, "pull_requests[0].head.sha == null", "fork PRs (empty pull_requests) must still report")
}
