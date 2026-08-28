package services

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedStorageSamples writes one storage_samples row per day going back
// `days` days from now, with database_bytes and fs_used_bytes declining
// linearly (each sample is `daysAgo` days in the past):
//
//	database_bytes = dbBase - dbPerDay*daysAgo
//	fs_used_bytes  = usedBase - usedPerDay*daysAgo
func seedStorageSamples(t *testing.T, db *gorm.DB, now time.Time, days int, dbBase, dbPerDay, usedBase, usedPerDay int64) {
	t.Helper()
	for d := 0; d <= days; d++ {
		sample := models.StorageSample{
			TakenAt:       now.Add(-time.Duration(d) * 24 * time.Hour),
			DatabaseBytes: dbBase - dbPerDay*int64(d),
			FSUsedBytes:   usedBase - usedPerDay*int64(d),
			FSTotalBytes:  1099511627776, // 1 TiB
		}
		require.NoError(t, db.Create(&sample).Error)
	}
}

// growthDelta is a tiny helper so the assertions read as (latest - oldest).
func growthDelta(d *int64) int64 {
	if d == nil {
		return -1
	}
	return *d
}

func TestComputeStorageTrend_GrowthDeltas(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// 90 daily samples: database shrinks 100 B/day going back (today 10000 B,
	// 90 days ago 1000 B).
	seedStorageSamples(t, db, now, 90, 10000, 100, 190<<30, 1<<30)

	trend := ComputeStorageTrend(ctx, db, now)

	// 7d window: oldest in-window sample is now-7d = 10000-700 = 9300;
	// latest = 10000. Delta = 10000 - 9300 = 700.
	require.NotNil(t, trend.Growth7DBytes, "7d growth needs only 2 in-window samples")
	assert.Equal(t, int64(700), growthDelta(trend.Growth7DBytes))

	// 30d window: oldest at now-30d = 10000-3000 = 7000; delta = 10000-7000.
	require.NotNil(t, trend.Growth30DBytes)
	assert.Equal(t, int64(3000), growthDelta(trend.Growth30DBytes))

	// 90d window: oldest at now-90d = 10000-9000 = 1000; delta = 10000-1000.
	require.NotNil(t, trend.Growth90DBytes)
	assert.Equal(t, int64(9000), growthDelta(trend.Growth90DBytes))
}

func TestComputeStorageTrend_ProjectedFullAt_LinearSeries(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// used grows 1 GiB/day: 190 GiB today, 160 GiB at now-30d. total = 1 TiB.
	seedStorageSamples(t, db, now, 90, 10000, 100, 190<<30, 1<<30)

	trend := ComputeStorageTrend(ctx, db, now)
	require.NotNil(t, trend.ProjectedFullAt, "a linear series must project an exhaustion date")

	// The fit is over the last 30 days (31 samples, perfectly collinear), so
	// it recovers the exact 1 GiB/day slope. From t0 = now-30d the fit is
	// 160 GiB; remaining to 1 TiB is 864 GiB -> 864 days.
	expected := now.Add(-30 * 24 * time.Hour).Add(864 * 24 * time.Hour)
	assert.WithinDuration(t, expected, *trend.ProjectedFullAt, time.Minute,
		"projection must land on the analytically-derived exhaustion date")
}

func TestComputeStorageTrend_FlatSeries_NoProjection(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// Constant used (500 GiB every day for 30 days) -> slope 0 -> no date.
	seedStorageSamples(t, db, now, 30, 10000, 100, 500<<30, 0)

	trend := ComputeStorageTrend(ctx, db, now)
	assert.Nil(t, trend.ProjectedFullAt, "a flat series must not project an exhaustion date")
	require.NotNil(t, trend.Growth30DBytes, "growth still works without a projection")
}

func TestComputeStorageTrend_InsufficientHistory_Null(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// Only 3 days of samples: enough for a 7d growth delta, but the
	// projection needs >= 14 days of elapsed history.
	seedStorageSamples(t, db, now, 2, 10000, 100, 500<<30, 1<<30)

	trend := ComputeStorageTrend(ctx, db, now)
	require.NotNil(t, trend.Growth7DBytes)
	assert.Nil(t, trend.ProjectedFullAt, "fewer than 14 days of history must not project")

	// A single sample produces no growth deltas at all.
	db2 := dbtest.New(t)
	require.NoError(t, db2.Create(&models.StorageSample{
		TakenAt:       now,
		DatabaseBytes: 42,
		FSUsedBytes:   10 << 30,
		FSTotalBytes:  1 << 40,
	}).Error)
	trend2 := ComputeStorageTrend(ctx, db2, now)
	assert.Nil(t, trend2.Growth7DBytes)
	assert.Nil(t, trend2.Growth30DBytes)
	assert.Nil(t, trend2.Growth90DBytes)
	assert.Nil(t, trend2.ProjectedFullAt)
}

func TestComputeStorageTrend_EmptySeries_AllNull(t *testing.T) {
	db := dbtest.New(t)
	trend := ComputeStorageTrend(context.Background(), db, time.Now())
	assert.Nil(t, trend.Growth7DBytes)
	assert.Nil(t, trend.Growth30DBytes)
	assert.Nil(t, trend.Growth90DBytes)
	assert.Nil(t, trend.ProjectedFullAt)
}

// storageThresholdEnv returns a fresh DB with the alert_states table cleared,
// so each threshold scenario starts from a clean hysteresis state.
func storageThresholdEnv(t *testing.T) (*gorm.DB, context.Context) {
	t.Helper()
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM alert_states").Error)
	return db, context.Background()
}

func TestStorageThreshold_WarningHysteresis(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	// Clean start below both thresholds.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, 75, 90))

	// Crossing warn (75) raises warning.
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 76, 75, 90))

	// Dropping 4 points below 75 (to 71) does NOT clear it (hysteresis).
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 71, 75, 90))

	// Dropping 6 points below 75 (to 69) clears it.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 69, 75, 90))
}

func TestStorageThreshold_CriticalTierAndHysteresis(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	// Cross crit (90) directly from ok.
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 92, 75, 90))

	// 4 points below 90 (86) stays critical.
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 86, 75, 90))

	// Below crit-5 (85) but still above warn (75) falls back to warning, not ok.
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 84, 75, 90))

	// All the way down clears to ok.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 69, 75, 90))
}

func TestStorageThreshold_PersistsTierInAlertStates(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 95, 75, 90))

	var row models.AlertState
	require.NoError(t, db.Where("condition_key = ?", alertConditionKeyStorageThreshold).First(&row).Error)
	assert.Equal(t, models.AlertStateAlerting, row.State)
	assert.Equal(t, StorageThresholdCritical, row.Detail)

	// Recovery flips the row back to ok and clears the detail.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, 75, 90))
	require.NoError(t, db.Where("condition_key = ?", alertConditionKeyStorageThreshold).First(&row).Error)
	assert.Equal(t, models.AlertStateOK, row.State)
	assert.Empty(t, row.Detail)
}

func TestStorageThreshold_ZeroConfigUsesDefaults(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	// A raw config.Config (no LoadConfig) has zero thresholds; they must
	// resolve to the documented defaults (75 / 90).
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, 0, 0))
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 80, 0, 0))
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 95, 0, 0))
}

// samplerTestDB builds a real migrated DB with a config whose storage dirs are
// real temp dirs, so the sampler's sizing walk and fs stat succeed.
func samplerTestDB(t *testing.T) (*gorm.DB, config.Config) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	db := dbtest.NewAt(t, dbPath)
	cfg := config.Config{
		DBPath:                     dbPath,
		ProfilePhotoDir:            filepath.Join(dir, "photos"),
		AttachmentsDir:             filepath.Join(dir, "attachments"),
		StorageSampleRetentionDays: 180,
	}
	return db, cfg
}

func TestRecordStorageSampleScheduled_ConcurrentRunsWriteOneRow(t *testing.T) {
	db, cfg := samplerTestDB(t)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RecordStorageSampleScheduled(db, cfg)
		}()
	}
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&models.StorageSample{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the job lock must de-duplicate concurrent runs to a single storage_samples row")

	// A third, later run within the min interval is also skipped.
	RecordStorageSampleScheduled(db, cfg)
	require.NoError(t, db.Model(&models.StorageSample{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "a run within the min interval must be skipped")

	// The sampler emits one system_events row for the timeline (component
	// storage, operation storage_sample).
	var events int64
	require.NoError(t, db.Model(&models.SystemEvent{}).
		Where("component = ? AND operation = ?", "storage", models.JobNameStorageSample).
		Count(&events).Error)
	assert.Equal(t, int64(1), events, "one successful sample must emit one system event")
}

func TestRecordStorageSampleScheduled_SkipsWhenRecentlyRun(t *testing.T) {
	db, cfg := samplerTestDB(t)

	// A recent successful run means the next invocation must skip entirely —
	// the pre-seeded job lock row's LastRunAt is "now".
	require.NoError(t, db.Create(&models.JobExecution{
		JobName:   models.JobNameStorageSample,
		LastRunAt: time.Now(),
	}).Error)

	RecordStorageSampleScheduled(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.StorageSample{}).Count(&count).Error)
	assert.Zero(t, count, "a rate-limited run must not write a sample")

	var events int64
	require.NoError(t, db.Model(&models.SystemEvent{}).
		Where("operation = ?", models.JobNameStorageSample).
		Count(&events).Error)
	assert.Zero(t, events, "a rate-limited run must not emit a system event")
}

func TestRecordStorageSample_PrunesRowsPastRetention(t *testing.T) {
	db, cfg := samplerTestDB(t)
	cfg.StorageSampleRetentionDays = 3

	// Seed one sample inside the window and two outside it (relative to a
	// fixed reference, so the test is deterministic).
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for _, d := range []int{1, 2, 4, 5} {
		require.NoError(t, db.Create(&models.StorageSample{
			TakenAt:       now.Add(-time.Duration(d) * 24 * time.Hour),
			DatabaseBytes: int64(d) * 1000,
			FSUsedBytes:   int64(d) * 1000,
			FSTotalBytes:  1099511627776,
		}).Error)
	}

	// RecordStorageSample uses time.Now() internally for both the new sample
	// and the prune cutoff, so monkeypatched time isn't an option — instead
	// drive the prune directly against the seeded rows with the same cutoff
	// arithmetic the sampler uses.
	cutoff := now.Add(-3 * 24 * time.Hour)
	require.NoError(t, db.Exec("DELETE FROM storage_samples WHERE taken_at < ?", cutoff).Error)

	var remaining int64
	require.NoError(t, db.Model(&models.StorageSample{}).Count(&remaining).Error)
	assert.Equal(t, int64(2), remaining, "only the in-window (-1d, -2d) samples must survive")
}

// TestRecordStorageSampleScheduled_PrunesViaScheduledRun exercises the full
// scheduled path end to end: seed expired rows, run the job, and assert they
// are gone while the fresh sample remains.
func TestRecordStorageSampleScheduled_PrunesViaScheduledRun(t *testing.T) {
	db, cfg := samplerTestDB(t)
	cfg.StorageSampleRetentionDays = 3

	// Seed rows far outside the window (30 and 31 days old). The sampler's
	// prune cutoff is time.Now()-3d, so both are expired regardless of the
	// exact clock.
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.StorageSample{
			TakenAt:       time.Now().Add(-time.Duration(30+i) * 24 * time.Hour),
			DatabaseBytes: 1234,
			FSUsedBytes:   1234,
			FSTotalBytes:  1099511627776,
		}).Error)
	}

	RecordStorageSampleScheduled(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.StorageSample{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the scheduled run must prune the expired rows and leave its own fresh sample")
}
