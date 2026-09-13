package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackupStaleConditionMeasuresOperatorBackups is the regression test for
// issue #943: backup_stale must track `make backup` (backup_completed tagged
// component=backup), not the weekly restore drill (restore_test_completed).
// Before the fix, a dead operator backup cron stayed green as long as the
// drill kept running.
func TestBackupStaleConditionMeasuresOperatorBackups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backup-stale.db")
	db := dbtest.NewAt(t, dbPath)
	cfg := alertTestConfig(dbPath)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	}

	t.Run("never backed up does not fire", func(t *testing.T) {
		reset(t)
		r := backupStaleCondition(ctx, db, cfg)
		assert.False(t, r.firing, "a fresh instance with no backup yet is not an incident")
	})

	t.Run("a fresh operator backup does not fire", func(t *testing.T) {
		reset(t)
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-time.Hour), "")
		r := backupStaleCondition(ctx, db, cfg)
		assert.False(t, r.firing)
	})

	t.Run("a stale operator backup fires and names make backup", func(t *testing.T) {
		reset(t)
		// Default threshold is 2 * 168h = 336h; 400h is past it.
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-400*time.Hour), "")
		r := backupStaleCondition(ctx, db, cfg)
		require.True(t, r.firing)
		assert.Contains(t, r.detail, "operator backup")
		assert.Contains(t, r.detail, "make backup")
	})

	t.Run("a fresh restore drill does NOT satisfy operator-backup freshness", func(t *testing.T) {
		reset(t)
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-400*time.Hour), "")
		seedEvent(t, db, logger.ComponentBackup, models.SysEventRestoreTestCompleted, time.Now().Add(-time.Hour), "")
		r := backupStaleCondition(ctx, db, cfg)
		assert.True(t, r.firing, "the weekly drill must not mask a dead operator backup cron")
	})

	t.Run("a pre-migration backup does NOT satisfy freshness", func(t *testing.T) {
		reset(t)
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-400*time.Hour), "")
		seedEvent(t, db, "migration", models.SysEventBackupCompleted, time.Now().Add(-time.Hour), "")
		r := backupStaleCondition(ctx, db, cfg)
		assert.True(t, r.firing, "an upgrade rollback point is not a routine operator backup")
	})

	t.Run("a fresh operator backup clears the alert", func(t *testing.T) {
		reset(t)
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-400*time.Hour), "")
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-time.Minute), "")
		r := backupStaleCondition(ctx, db, cfg)
		assert.False(t, r.firing)
	})

	t.Run("an explicit threshold overrides the default", func(t *testing.T) {
		reset(t)
		tight := cfg
		tight.AlertBackupMaxAgeHours = 24
		seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-48*time.Hour), "")
		r := backupStaleCondition(ctx, db, tight)
		require.True(t, r.firing)
		assert.Contains(t, r.detail, "threshold 24h")
	})
}

// TestEvaluateAlertConditionsWiresOperatorBackupFreshness guards the call site:
// backup_stale must be produced by the evaluator with the operator-backup query,
// not the old combined subsystem health.
func TestEvaluateAlertConditionsWiresOperatorBackupFreshness(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wiring.db")
	db := dbtest.NewAt(t, dbPath)
	cfg := alertTestConfig(dbPath)
	cfg.AlertDiskUsagePercent = 0 // keep the disk condition off the real filesystem

	seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupCompleted, time.Now().Add(-400*time.Hour), "")

	results := evaluateAlertConditions(context.Background(), db, cfg, nil, nil)
	var found bool
	for _, r := range results {
		if r.key == alertConditionKeyBackupStale {
			found = true
			assert.True(t, r.firing, "a 400h-old operator backup must make backup_stale fire")
		}
	}
	assert.True(t, found, "the evaluator must emit the backup_stale condition")
}
