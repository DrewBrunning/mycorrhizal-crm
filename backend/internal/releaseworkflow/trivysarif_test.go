package releaseworkflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommittedTrivySARIFStepsLimitSeverities(t *testing.T) {
	v, scanned, err := CheckAllTrivySARIFSeverity(filepath.Join(repoRoot(t), ".github", "workflows"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, scanned, 3, "expected several workflows to use trivy-action; the scan is blind")
	assert.Empty(t, v)
}

const trivyStep = `
jobs:
  scan:
    steps:
      - name: Trivy
        uses: aquasecurity/trivy-action@abc # v0
        with:
          format: 'sarif'
          severity: 'CRITICAL,HIGH'
          EXTRA
`

func TestCheckTrivySARIFSeverity(t *testing.T) {
	with := func(extra string) string { return strings.Replace(trivyStep, "EXTRA", extra, 1) }

	assert.Len(t, CheckTrivySARIFSeverity("x.yml", with("exit-code: '1'")), 1)
	assert.Empty(t, CheckTrivySARIFSeverity("x.yml", with("limit-severities-for-sarif: 'true'")))
	assert.Len(t, CheckTrivySARIFSeverity("x.yml", with("limit-severities-for-sarif: 'false'")), 1)

	noSev := `
jobs:
  scan:
    steps:
      - uses: aquasecurity/trivy-action@abc
        with:
          format: sarif
`
	assert.Empty(t, CheckTrivySARIFSeverity("x.yml", noSev))
	tbl := `
jobs:
  scan:
    steps:
      - uses: aquasecurity/trivy-action@abc
        with:
          format: table
          severity: HIGH
`
	assert.Empty(t, CheckTrivySARIFSeverity("x.yml", tbl))
	assert.Len(t, CheckTrivySARIFSeverity("x.yml", "a: [\n"), 1)
}
