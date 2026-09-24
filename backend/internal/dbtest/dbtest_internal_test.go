package dbtest

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestBuildTemplate_FailsOnUnwritableDir proves buildTemplate actually
// surfaces a failure when the migration step can't create the database file,
// rather than silently returning a path to nothing. This is the direct,
// package-internal exercise of template()'s error path that the process-wide
// sync.Once makes impossible to reach through the public New/NewAt API once
// any other test in the binary has already built the template successfully.
func TestBuildTemplate_FailsOnUnwritableDir(t *testing.T) {
	badDir := filepath.Join(t.TempDir(), "does-not-exist")

	path, err := buildTemplate(badDir)
	require.Error(t, err)
	assert.Empty(t, path)
}

// TestBuildTemplate_SucceedsOnRealDir is the positive control: a real,
// writable directory produces a usable template file. Without this, the
// failure test above could pass for the wrong reason (e.g. a typo that made
// buildTemplate fail unconditionally).
func TestBuildTemplate_SucceedsOnRealDir(t *testing.T) {
	path, err := buildTemplate(t.TempDir())
	require.NoError(t, err)
	assert.FileExists(t, path)
}

// TestFinalizeTemplate_FailsWhenGormHasNoConnPool proves finalizeTemplate
// surfaces db.DB()'s error instead of panicking or silently returning a
// usable-looking path. A zero-value *gorm.DB has no ConnPool, so gorm.DB()
// returns gorm.ErrInvalidDB -- this is the one realistic way to reach that
// branch without making the real migration itself fail.
func TestFinalizeTemplate_FailsWhenGormHasNoConnPool(t *testing.T) {
	path, err := finalizeTemplate(&gorm.DB{Config: &gorm.Config{}}, "unused")
	require.Error(t, err)
	assert.Empty(t, path)
}

// TestFinalizeTemplate_FailsWhenCheckpointFails proves finalizeTemplate
// surfaces a WAL-checkpoint failure. Closing the underlying *sql.DB before
// calling finalizeTemplate leaves the *gorm.DB's ConnPool pointing at an
// already-closed connection, so the checkpoint Exec genuinely fails with
// "sql: database is closed" -- a real failure, not a mock.
func TestFinalizeTemplate_FailsWhenCheckpointFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "template.db")
	db, err := database.InitDB(p)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	path, err := finalizeTemplate(db, p)
	require.Error(t, err)
	assert.Empty(t, path)
}

// TestCopyFile_FailsWhenSrcMissing proves copyFile reports an error instead
// of silently producing an empty or missing destination when the source
// file does not exist.
func TestCopyFile_FailsWhenSrcMissing(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.db")
	err := copyFile(filepath.Join(t.TempDir(), "no-such-src.db"), dst)
	require.Error(t, err)
	assert.NoFileExists(t, dst)
}

// TestCopyFile_FailsWhenDstDirMissing proves copyFile reports an error when
// the destination cannot be created.
func TestCopyFile_FailsWhenDstDirMissing(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.db")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))

	dst := filepath.Join(t.TempDir(), "missing-dir", "dst.db")
	err := copyFile(src, dst)
	require.Error(t, err)
}

// TestCopyFile_CopiesContent is the positive control for the two failure
// tests above.
func TestCopyFile_CopiesContent(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.db")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

	dst := filepath.Join(t.TempDir(), "dst.db")
	require.NoError(t, copyFile(src, dst))
	assert.FileExists(t, dst)
}
