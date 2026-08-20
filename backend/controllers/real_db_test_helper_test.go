package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// closeTestDBAtTeardown registers a cleanup that closes the per-test file DB
// before t.TempDir() removes it. Handlers spawn fire-and-forget goroutines
// (webhook deliveries, audit writes) that hold this *gorm.DB; if the file is
// deleted from under an open modernc/sqlite connection while one is still
// running, the driver recreates a transient "001" temp directory and
// t.TempDir's cleanup fails with "directory not empty" — a recurring CI flake
// in the merge tests (issue #264 follow-up). Closing the pool first makes
// straggler goroutines fail fast ("sql: database is closed") with no
// filesystem side effects. Cleanups run LIFO, so this close runs after any
// test-registered cleanups and before TempDir's RemoveAll.
func closeTestDBAtTeardown(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
}
