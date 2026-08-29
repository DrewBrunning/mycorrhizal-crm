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

func TestStoredPathRejectsEmptyDir(t *testing.T) {
	_, err := StoredPath("", "some-uuid")
	assert.Error(t, err, "StoredPath must reject an empty directory")
}

func TestStoredPathAcceptsBareName(t *testing.T) {
	dir := t.TempDir()
	path, err := StoredPath(dir, "01234567-89ab-cdef-0123-456789abcdef")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "01234567-89ab-cdef-0123-456789abcdef"), path)
}

func TestSaveCreatesDirectoryRecursively(t *testing.T) {
	// The directory does not exist yet; Save must create it.
	dir := filepath.Join(t.TempDir(), "nested", "sub")
	stored, err := Save([]byte("data"), dir)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, stored))
	require.NoError(t, err)
}

func TestSaveRejectsInvalidStoredPath(t *testing.T) {
	// Save derives the stored name itself, so StoredPath cannot reject it;
	// this test pins that Save still surfaces the write failure deterministically
	// by pointing it at a read-only directory (MkdirAll succeeds — the dir
	// exists — and WriteFile then fails with permission denied). Skipped as
	// root, where POSIX permission bits are not enforced.
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	require.NoError(t, os.MkdirAll(dir, 0o500))
	_, err := Save([]byte("data"), dir)
	assert.Error(t, err, "Save must surface the write failure")
}

func TestSaveFailsWhenDirIsAFile(t *testing.T) {
	// MkdirAll errors when the target path is an existing regular file —
	// deterministic, no permission dependency.
	dir := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0o600))
	_, err := Save([]byte("data"), dir)
	assert.Error(t, err, "Save must fail when the directory path is a file")
}

func TestRemoveRejectsInvalidStoredName(t *testing.T) {
	err := Remove(t.TempDir(), "../escape.txt")
	assert.Error(t, err, "Remove must reject a traversal stored name")
}

func TestRemoveFailurePropagates(t *testing.T) {
	// A stored name that StoredPath accepts but whose file cannot be removed
	// must surface the error rather than swallowing it. Skipped as root,
	// where POSIX permission bits are not enforced.
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	name, err := Save([]byte("x"), dir)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // let TempDir's RemoveAll succeed
	err = Remove(dir, name)
	assert.Error(t, err, "Remove must propagate a non-IsNotExist removal failure")
}
