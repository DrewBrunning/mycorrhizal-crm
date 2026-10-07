package realrelease

import (
	"os"
	"path/filepath"
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
