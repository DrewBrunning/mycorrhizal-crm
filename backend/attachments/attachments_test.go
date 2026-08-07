package attachments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveReadRemoveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	stored, err := Save([]byte("hello attachment"), dir)
	require.NoError(t, err)
	assert.NotEmpty(t, stored)
	assert.Equal(t, stored, filepath.Base(stored), "stored name must be a bare filename")

	path, err := StoredPath(dir, stored)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello attachment", string(data))

	require.NoError(t, Remove(dir, stored))
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "file must be gone after Remove")
}

func TestStoredPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"..", "../evil", "a/../evil", "/etc/passwd", "a/b", "..\\evil", ""} {
		_, err := StoredPath(dir, bad)
		assert.Error(t, err, "StoredPath must reject %q", bad)
	}
}

func TestRemoveMissingFileIsNotAnError(t *testing.T) {
	require.NoError(t, Remove(t.TempDir(), "nonexistent-uuid"))
}

func TestSaveGeneratesDistinctNames(t *testing.T) {
	dir := t.TempDir()
	a, err := Save([]byte("a"), dir)
	require.NoError(t, err)
	b, err := Save([]byte("b"), dir)
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.Len(t, a, 36, "a UUID stored name")
	assert.Len(t, strings.Split(a, ""), 36)
}
