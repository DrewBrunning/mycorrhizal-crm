package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrityCheckReportsOKOnMigratedDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ok.db")
	require.NoError(t, MigrateUp(path))

	result, err := IntegrityCheck(path)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestIntegrityCheckFailsOnMissingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "does-not-exist.db")
	_, err := IntegrityCheck(path)
	require.Error(t, err)
}

func TestIntegrityCheckFailsOnCorruptFile(t *testing.T) {
	t.Parallel()
	// A real partial-write artifact (disk full, torn copy): a valid SQLite
	// header with no usable pages. IntegrityCheck must reject it, not report
	// "ok" — the fail-closed contract the chaos harness depends on.
	path := filepath.Join(t.TempDir(), "corrupt.db")
	require.NoError(t, os.WriteFile(path, make([]byte, 512), 0o600))

	result, err := IntegrityCheck(path)
	assert.Error(t, err, "a corrupt file must fail, never report ok")
	if err == nil {
		assert.NotEqual(t, "ok", result)
	}
}
