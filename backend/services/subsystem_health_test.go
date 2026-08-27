package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newHealthDB returns a real migrated schema with the boot-time
// migration_completed event removed, so each test owns the full row set
// (mirrors controllers' setupSystemEventRouter).
func newHealthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "subsystem-health.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	return db
}

func seedEvent(t *testing.T, db *gorm.DB, component, eventType string, occurredAt time.Time, errMsg string) {
	t.Helper()
	ev := models.SystemEvent{
		Component:  component,
		EventType:  eventType,
		OccurredAt: occurredAt.UTC(),
		Error:      errMsg,
	}
	require.NoError(t, db.Create(&ev).Error)
}

func healthFor(t *testing.T, hs []SubsystemHealth, name string) SubsystemHealth {
	t.Helper()
	for _, h := range hs {
		if h.Subsystem == name {
			return h
		}
	}
	t.Fatalf("no subsystem health entry for %q", name)
	return SubsystemHealth{}
}

func TestComputeSubsystemHealth_FreshDB_AllUnknown(t *testing.T) {
	db := newHealthDB(t)

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, hs, len(subsystemDefs))

	for i, d := range subsystemDefs {
		assert.Equal(t, d.name, hs[i].Subsystem, "order preserved")
		assert.Equal(t, SubsystemStatusUnknown, hs[i].Status)
		assert.Zero(t, hs[i].ConsecutiveFailures)
		assert.Nil(t, hs[i].LastAttemptAt)
		assert.Nil(t, hs[i].LastSuccessAt)
		assert.Nil(t, hs[i].LastFailureAt)
		assert.Nil(t, hs[i].IncidentFirstFailureAt)
		assert.Empty(t, hs[i].LastError)
	}
}

func TestComputeSubsystemHealth_ConsecutiveFailures(t *testing.T) {
	db := newHealthDB(t)
	base := time.Now().Add(-10 * time.Hour)

	var first, last time.Time
	for i := 0; i < 9; i++ {
		at := base.Add(time.Duration(i) * time.Hour)
		if i == 0 {
			first = at.UTC()
		}
		last = at.UTC()
		msg := "sync error " + time.Duration(i).String()
		if i == 8 {
			msg = "final failure"
		}
		seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, at, msg)
	}

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentContactSync)

	assert.Equal(t, SubsystemStatusFailing, h.Status)
	assert.Equal(t, 9, h.ConsecutiveFailures)
	assert.Equal(t, "final failure", h.LastError)
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.WithinDuration(t, first, *h.IncidentFirstFailureAt, time.Second)
	require.NotNil(t, h.LastFailureAt)
	assert.WithinDuration(t, last, *h.LastFailureAt, time.Second)
	require.NotNil(t, h.LastAttemptAt)
	assert.WithinDuration(t, last, *h.LastAttemptAt, time.Second)
	assert.Nil(t, h.LastSuccessAt)

	// Other subsystems are untouched.
	assert.Equal(t, SubsystemStatusUnknown, healthFor(t, hs, logger.ComponentCalendarSync).Status)
}

func TestComputeSubsystemHealth_SuccessResetsIncident(t *testing.T) {
	db := newHealthDB(t)
	base := time.Now().Add(-10 * time.Hour)
	for i := 0; i < 9; i++ {
		seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, base.Add(time.Duration(i)*time.Hour), "boom")
	}
	lastFailureAt := base.Add(8 * time.Hour).UTC()
	successAt := time.Now().Add(-30 * time.Minute)
	seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncCompleted, successAt, "")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentContactSync)

	assert.Equal(t, SubsystemStatusHealthy, h.Status)
	assert.Zero(t, h.ConsecutiveFailures)
	assert.Nil(t, h.IncidentFirstFailureAt)
	assert.Empty(t, h.LastError)
	require.NotNil(t, h.LastSuccessAt)
	assert.WithinDuration(t, successAt.UTC(), *h.LastSuccessAt, time.Second)
	// The historical failure is still reported.
	require.NotNil(t, h.LastFailureAt)
	assert.WithinDuration(t, lastFailureAt, *h.LastFailureAt, time.Second)
	require.NotNil(t, h.LastAttemptAt)
	assert.WithinDuration(t, successAt.UTC(), *h.LastAttemptAt, time.Second)
}

func TestComputeSubsystemHealth_IncidentStartsAfterLastSuccess(t *testing.T) {
	db := newHealthDB(t)
	now := time.Now()
	seedEvent(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-10*time.Hour), "old")
	seedEvent(t, db, logger.ComponentNotify, models.SysEventNotificationSent, now.Add(-5*time.Hour), "")
	incidentStart := now.Add(-3 * time.Hour).UTC()
	seedEvent(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-3*time.Hour), "smtp timeout")
	seedEvent(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-2*time.Hour), "smtp timeout")
	seedEvent(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-1*time.Hour), "smtp timeout")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentNotify)

	assert.Equal(t, SubsystemStatusFailing, h.Status)
	assert.Equal(t, 3, h.ConsecutiveFailures, "only failures after the last success count")
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.WithinDuration(t, incidentStart, *h.IncidentFirstFailureAt, time.Second)
}

func TestComputeSubsystemHealth_PerComponentIsolation(t *testing.T) {
	db := newHealthDB(t)
	now := time.Now()
	// contact_sync and calendar_sync share the sync_completed/sync_failed
	// tokens — they must be told apart by component.
	seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-1*time.Hour), "carddav 401")
	seedEvent(t, db, logger.ComponentCalendarSync, models.SysEventSyncCompleted, now.Add(-1*time.Hour), "")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)

	assert.Equal(t, SubsystemStatusFailing, healthFor(t, hs, logger.ComponentContactSync).Status)
	assert.Equal(t, "carddav 401", healthFor(t, hs, logger.ComponentContactSync).LastError)
	assert.Equal(t, SubsystemStatusHealthy, healthFor(t, hs, logger.ComponentCalendarSync).Status)
}

func TestComputeSubsystemHealth_FailureOnlySubsystem(t *testing.T) {
	db := newHealthDB(t)
	now := time.Now()
	incidentStart := now.Add(-4 * time.Hour).UTC()
	seedEvent(t, db, logger.ComponentScheduler, models.SysEventJobFailed, now.Add(-4*time.Hour), "panic: nil map")
	seedEvent(t, db, logger.ComponentScheduler, models.SysEventJobFailed, now.Add(-2*time.Hour), "panic: nil map")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentScheduler)

	assert.Equal(t, SubsystemStatusFailing, h.Status)
	assert.Equal(t, 2, h.ConsecutiveFailures)
	assert.Nil(t, h.LastSuccessAt, "scheduler emits no success token today")
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.WithinDuration(t, incidentStart, *h.IncidentFirstFailureAt, time.Second)

	// webhook (integration_failed only) behaves the same way.
	seedEvent(t, db, logger.ComponentWebhook, models.SysEventIntegrationFailed, now.Add(-1*time.Hour), "gotify 401")
	hs, err = ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	w := healthFor(t, hs, logger.ComponentWebhook)
	assert.Equal(t, SubsystemStatusFailing, w.Status)
	assert.Equal(t, 1, w.ConsecutiveFailures)
}

func TestComputeSubsystemHealth_BackupUsesRestoreTestToken(t *testing.T) {
	db := newHealthDB(t)
	now := time.Now()
	seedEvent(t, db, logger.ComponentBackup, models.SysEventBackupFailed, now.Add(-2*time.Hour), "size mismatch")
	seedEvent(t, db, logger.ComponentBackup, models.SysEventRestoreTestCompleted, now.Add(-1*time.Hour), "")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentBackup)

	assert.Equal(t, SubsystemStatusHealthy, h.Status)
	assert.Zero(t, h.ConsecutiveFailures)
	require.NotNil(t, h.LastSuccessAt)
	require.NotNil(t, h.LastFailureAt)
}

func TestComputeSubsystemHealth_SameTimestampTiebreakByID(t *testing.T) {
	db := newHealthDB(t)
	at := time.Now().Add(-1 * time.Hour)
	// Success then failure at the identical occurred_at — the later row (higher
	// autoincrement id) is the later event, so the subsystem is failing.
	seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncCompleted, at, "")
	seedEvent(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, at, "flaky")

	hs, err := ComputeSubsystemHealth(context.Background(), db)
	require.NoError(t, err)
	h := healthFor(t, hs, logger.ComponentContactSync)

	assert.Equal(t, SubsystemStatusFailing, h.Status)
	assert.Equal(t, 1, h.ConsecutiveFailures)
	require.NotNil(t, h.IncidentFirstFailureAt)
}
