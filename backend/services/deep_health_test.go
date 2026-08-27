package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepHealthSnapshot_HealthyBaseline(t *testing.T) {
	ResetDeepHealthCache()
	defer ResetDeepHealthCache()
	db := freshDB(t, "deep-baseline.db")

	h := DeepHealthSnapshot(db, config.Config{})
	assert.Equal(t, "healthy", h.Status)
	assert.Equal(t, DeepStatusOK, h.Database.Status)
	assert.Equal(t, DeepStatusOK, h.Migrations.Status)
	assert.Equal(t, DeepStatusNotConfigured, h.IntegrityCheck.Status)
	assert.Empty(t, h.Integrations)
}

func TestDeepHealthSnapshot_CachesSlowSection(t *testing.T) {
	ResetDeepHealthCache()
	prev := SetDeepHealthCacheTTL(time.Minute)
	defer func() { SetDeepHealthCacheTTL(prev); ResetDeepHealthCache() }()

	db := freshDB(t, "deep-cache.db")
	cfg := config.Config{DBIntegrityCheckEnabled: true, DBIntegrityCheckIntervalHours: 24}

	// First call: enabled but no recorded result -> integrity_check degraded,
	// and that result is now cached.
	first := DeepHealthSnapshot(db, cfg)
	require.Equal(t, DeepStatusDegraded, first.IntegrityCheck.Status)

	// Record an OK result AFTER the cache is warm.
	require.NoError(t, db.Create(&models.OperationalCheckResult{
		CheckName: models.JobNameDBIntegrityCheck, Status: models.OpCheckStatusOK, CheckedAt: time.Now(),
	}).Error)

	// Still served from cache -> unchanged.
	cached := DeepHealthSnapshot(db, cfg)
	assert.Equal(t, DeepStatusDegraded, cached.IntegrityCheck.Status, "slow section must be cached")

	// After a reset it recomputes and sees the OK row.
	ResetDeepHealthCache()
	fresh := DeepHealthSnapshot(db, cfg)
	assert.Equal(t, DeepStatusOK, fresh.IntegrityCheck.Status)
}

func TestDeepHealthSnapshot_UnreachableEmailDegradesOverallNot503(t *testing.T) {
	ResetDeepHealthCache()
	defer ResetDeepHealthCache()
	db := freshDB(t, "deep-email.db")

	cfg := config.Config{
		UseSMTP:  true,
		SMTPHost: "127.0.0.1",
		SMTPPort: 1, // nothing listens here
	}
	h := DeepHealthSnapshot(db, cfg)

	require.Contains(t, h.Integrations, "email")
	assert.Equal(t, DeepStatusDegraded, h.Integrations["email"].Status)
	assert.Equal(t, "degraded", h.Status, "an unreachable optional integration is degraded, never unhealthy")
	assert.NotEqual(t, DeepStatusUnhealthy, h.Status)
}

func TestDeepHealthSnapshot_StaleOKResultIsDegraded(t *testing.T) {
	ResetDeepHealthCache()
	defer ResetDeepHealthCache()
	db := freshDB(t, "deep-stale.db")

	// An ok result, but older than 2x the 24h interval -> the job has silently
	// stopped running.
	require.NoError(t, db.Create(&models.OperationalCheckResult{
		CheckName: models.JobNameDBIntegrityCheck,
		Status:    models.OpCheckStatusOK,
		CheckedAt: time.Now().Add(-72 * time.Hour),
	}).Error)

	h := DeepHealthSnapshot(db, config.Config{
		DBIntegrityCheckEnabled: true, DBIntegrityCheckIntervalHours: 24,
	})
	assert.Equal(t, DeepStatusDegraded, h.IntegrityCheck.Status)
	assert.Contains(t, h.IntegrityCheck.Reason, "stale")
}

func TestDeepHealthSnapshot_MigrationBehindIsDegraded(t *testing.T) {
	ResetDeepHealthCache()
	defer ResetDeepHealthCache()
	db := freshDB(t, "deep-migration.db")
	require.NoError(t, db.Exec("UPDATE schema_migrations SET version = version - 1").Error)

	h := DeepHealthSnapshot(db, config.Config{})
	assert.Equal(t, DeepStatusDegraded, h.Migrations.Status)
	assert.Contains(t, h.Migrations.Reason, "behind the binary")
	assert.Equal(t, "degraded", h.Status)
}

func TestDeepHealthSnapshot_DatabaseDownIsUnhealthy(t *testing.T) {
	ResetDeepHealthCache()
	defer ResetDeepHealthCache()
	db := freshDB(t, "deep-dbdown.db")
	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Close())

	h := DeepHealthSnapshot(db, config.Config{})
	assert.Equal(t, DeepStatusUnhealthy, h.Database.Status)
	assert.Equal(t, DeepStatusUnhealthy, h.Status)
}
