package perfbench

import (
	"testing"

	"mycorrhizal/internal/dbtest"

	"gorm.io/gorm"
)

// migratedOpener returns an EnvOptions.OpenMigrated hook backed by
// internal/dbtest: the real migrated schema is built ONCE per test binary and
// every Env gets a sub-millisecond byte copy of it, instead of re-running the
// full embedded migration set per Env (issue #632's optimisation — without it
// this suite blows the CI -race timeout). dbtest registers its own t.Cleanup
// to drain fire-and-forget goroutines and close the handle, so Env is told not
// to close seedDB itself (closeSeed stays false for an injected opener).
func migratedOpener(tb testing.TB) func(string) (*gorm.DB, error) {
	tb.Helper()
	return func(path string) (*gorm.DB, error) {
		return dbtest.NewAt(tb, path), nil
	}
}
