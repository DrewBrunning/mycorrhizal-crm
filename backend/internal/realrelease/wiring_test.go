package realrelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CI job is only as good as its wiring: it must run the leg script for
// the matrix derived from SupportedReleases, and the script must drive the
// subcommands this package ships.
func TestCIWiring(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	wf, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "migration-tests.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(wf), "real-release-upgrade:")
	assert.Contains(t, string(wf), "scripts/realrelease-leg.sh")
	assert.Contains(t, string(wf), "needs.release-matrix.outputs.matrix",
		"legs must derive from SupportedReleases via cmd/releaselist")

	info, err := os.Stat(filepath.Join(root, "scripts", "realrelease-leg.sh"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "the leg script must be executable")
	sh, err := os.ReadFile(filepath.Join(root, "scripts", "realrelease-leg.sh"))
	require.NoError(t, err)
	for _, sub := range []string{`" seed `, `" verify `, `" capture `, `" compare `, `print-secret`} {
		assert.Contains(t, string(sh), sub)
	}
}

// Issue #1713: the release-registration commit lands on main before the new
// tag's image is published, so the matrix leg for that tag must skip (never
// fail) -- but only via the decision script (a published release with a
// missing image still fails) -- and the leg must instead run from
// docker-publish.yml once the image exists.
func TestImageNotYetPublishedWiring(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	read := func(parts ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		require.NoError(t, err)
		return string(b)
	}

	mt := read(".github", "workflows", "migration-tests.yml")
	job := mt[strings.Index(mt, "  real-release-upgrade:"):]
	job = job[:strings.Index(job, "  large-dataset:")]
	assert.Contains(t, job, "id: expected")
	assert.Contains(t, job, ".github/scripts/realrelease-image-expected.sh")
	assert.Contains(t, job, "fetch-depth: 2", "the decision script diffs HEAD^..HEAD")
	// The leg step itself must be gated on the decision, and the decision
	// step must not be: otherwise skip is either unreachable or unconditional.
	legStep := job[strings.Index(job, "- name: Seed ${{ matrix.release }}"):]
	legStep = legStep[:strings.Index(legStep, "- name: Upload")]
	assert.Contains(t, legStep, "if: steps.expected.outputs.skip != 'true'")
	decideStep := job[strings.Index(job, "id: expected"):strings.Index(job, "- name: Set up Go")]
	assert.NotContains(t, decideStep, "if:")

	sh := read(".github", "scripts", "realrelease-image-expected.sh")
	assert.Contains(t, sh, "::notice::")
	assert.Contains(t, sh, "manifest unknown", "only a definitive absence may skip")
	assert.Contains(t, sh, "REALRELEASE_TAG_EXISTS_CMD")
	assert.Contains(t, sh, "git/ref/tags/", "a version pre-registered before its tag must skip")

	dp := read(".github", "workflows", "docker-publish.yml")
	i := strings.Index(dp, "\n  real-release-leg:")
	require.NotEqual(t, -1, i, "docker-publish.yml must run the new release's leg")
	leg := dp[i:]
	assert.Contains(t, leg, "needs: [post-publish-smoke]", "the leg runs only after the image is published and smoke-tested")
	assert.Contains(t, leg, "uses: ./.github/workflows/real-release-leg.yml")
	assert.Contains(t, leg, "-rc.", "RCs are never registered in SupportedReleases")

	rl := read(".github", "workflows", "real-release-leg.yml")
	assert.Contains(t, rl, "workflow_call:")
	assert.NotContains(t, rl, "push:", "reusable only; any trigger would need alert registration")
	assert.Contains(t, rl, "scripts/realrelease-leg.sh")
}
