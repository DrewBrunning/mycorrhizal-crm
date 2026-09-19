package releaseworkflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRelease(t *testing.T) {
	// A conforming workflow: version-keyed, non-cancelling, readiness uploaded.
	good := `
concurrency:
  group: release-${{ inputs.version }}
  cancel-in-progress: false
jobs:
  release:
    steps:
      - name: Upload release-readiness artifact
        with:
          name: release-readiness-${{ env.VERSION }}
`
	assert.Empty(t, CheckRelease(good))

	// A group not keyed on the version is a finding.
	badGroup := `
concurrency:
  group: release
  cancel-in-progress: false
jobs: {}
`
	assert.Contains(t, CheckRelease(badGroup)[0], "inputs.version")

	// Cancelling an in-flight cut is a finding.
	badCancel := `
concurrency:
  group: release-${{ inputs.version }}
  cancel-in-progress: true
jobs: {}
`
	f := CheckRelease(badCancel)
	require.NotEmpty(t, f)
	assert.Contains(t, f[0], "cancel-in-progress")

	// Missing readiness upload is a finding.
	noReady := `
concurrency:
  group: release-${{ inputs.version }}
  cancel-in-progress: false
jobs: {}
`
	assert.Contains(t, CheckRelease(noReady)[0], "release-readiness")

	// Unparseable YAML is reported, not panicked on.
	assert.Contains(t, CheckRelease(":\tnot yaml")[0], "does not parse")
}

// TestCommittedReleaseWorkflow runs the real .github/workflows/release.yml
// through the check — the same thing cmd/releasegatecheck does.
func TestCommittedReleaseWorkflow(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	require.NoError(t, err)
	assert.Empty(t, CheckRelease(string(b)))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = parent
	}
}
