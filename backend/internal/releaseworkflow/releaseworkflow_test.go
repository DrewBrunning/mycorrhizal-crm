package releaseworkflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRelease(t *testing.T) {
	// A conforming workflow: version-keyed, non-cancelling, readiness uploaded,
	// and both final-only obligations invoked through the shared script.
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
      - name: Gate - ASVS/MASVS re-verification obligation
        run: |
          bash .github/scripts/release-obligations.sh asvs --ack "${ACK_ASVS_CURRENT:-}"
      - name: Gate - per-release adversarial delta
        run: |
          bash .github/scripts/release-obligations.sh adversarial --release "$VERSION"
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

// TestCheckReleaseRequiresObligations pins issue #1195: release.yml's final
// path must invoke both final-only obligations through the shared script.
func TestCheckReleaseRequiresObligations(t *testing.T) {
	base := `
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
	f := CheckRelease(base)
	require.Len(t, f, 2)
	assert.Contains(t, f[0], "release-obligations.sh asvs")
	assert.Contains(t, f[1], "release-obligations.sh adversarial")
}

// TestCheckPromote pins issue #1195: the RC-promotion path must run the two
// final-only obligations release.yml defers for an RC, and expose the ack
// escape-hatch inputs.
func TestCheckPromote(t *testing.T) {
	good := `
on:
  workflow_dispatch:
    inputs:
      rc_tag:
        required: true
      ack_asvs_current:
        required: false
      ack_adversarial_delta:
        required: false
jobs:
  promote:
    steps:
      - name: Gate - ASVS/MASVS re-verification obligation
        run: |
          bash .github/scripts/release-obligations.sh asvs --ack "${ACK_ASVS_CURRENT:-}"
      - name: Gate - per-release adversarial delta
        run: |
          bash .github/scripts/release-obligations.sh adversarial --release "$FINAL_TAG"
`
	assert.Empty(t, CheckPromote(good))

	// Neither obligation invoked and neither ack input declared: two gate
	// findings plus two input findings.
	bad := `
on:
  workflow_dispatch:
    inputs:
      rc_tag:
        required: true
jobs:
  promote:
    steps:
      - name: Validate
        run: echo hi
`
	f := CheckPromote(bad)
	require.Len(t, f, 4)
	assert.Contains(t, f[0], "release-obligations.sh asvs")
	assert.Contains(t, f[1], "release-obligations.sh adversarial")
	assert.Contains(t, f[2], "ack_asvs_current")
	assert.Contains(t, f[3], "ack_adversarial_delta")

	// Unparseable YAML is reported, not panicked on.
	assert.Contains(t, CheckPromote(":\tnot yaml")[0], "does not parse")
}

// TestCommittedReleaseWorkflow runs the real .github/workflows/release.yml
// through the check — the same thing cmd/releasegatecheck does.
func TestCommittedReleaseWorkflow(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	require.NoError(t, err)
	assert.Empty(t, CheckRelease(string(b)))
}

// TestCommittedPromoteRCWorkflow does the same for promote-rc.yml.
func TestCommittedPromoteRCWorkflow(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "promote-rc.yml"))
	require.NoError(t, err)
	assert.Empty(t, CheckPromote(string(b)))
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
