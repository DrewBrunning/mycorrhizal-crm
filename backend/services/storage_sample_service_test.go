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
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, gbFree(1), pctOnlyLimits))

	// Crossing warn (75) raises warning.
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 76, gbFree(1), pctOnlyLimits))

	// Dropping 4 points below 75 (to 71) does NOT clear it (hysteresis).
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 71, gbFree(1), pctOnlyLimits))

	// Dropping 6 points below 75 (to 69) clears it.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 69, gbFree(1), pctOnlyLimits))
}

func TestStorageThreshold_CriticalTierAndHysteresis(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	// Cross crit (90) directly from ok.
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 92, gbFree(1), pctOnlyLimits))

	// 4 points below 90 (86) stays critical.
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 86, gbFree(1), pctOnlyLimits))

	// Below crit-5 (85) but still above warn (75) falls back to warning, not ok.
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 84, gbFree(1), pctOnlyLimits))

	// All the way down clears to ok.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 69, gbFree(1), pctOnlyLimits))
}

func TestStorageThreshold_PersistsTierInAlertStates(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 95, gbFree(1), pctOnlyLimits))

	var row models.AlertState
	require.NoError(t, db.Where("condition_key = ?", alertConditionKeyStorageThreshold).First(&row).Error)
	assert.Equal(t, models.AlertStateAlerting, row.State)
	assert.Equal(t, StorageThresholdCritical, row.Detail)

	// Recovery flips the row back to ok and clears the detail.
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, gbFree(1), pctOnlyLimits))
	require.NoError(t, db.Where("condition_key = ?", alertConditionKeyStorageThreshold).First(&row).Error)
	assert.Equal(t, models.AlertStateOK, row.State)
	assert.Empty(t, row.Detail)
}

func TestStorageThreshold_ZeroConfigUsesDefaults(t *testing.T) {
	db, ctx := storageThresholdEnv(t)

	// A raw config.Config (no LoadConfig) has zero thresholds; they must
	// resolve to the documented defaults (75 / 90).
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 50, gbFree(1), StorageLimits{}))
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 80, gbFree(1), StorageLimits{}))
	assert.Equal(t, StorageThresholdCritical, StorageThreshold(ctx, db, 95, gbFree(1), StorageLimits{}))
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

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	useFakeClock(t, now)

	// Rows straddling the cutoff (now-3d): the exact boundary row is kept
	// (the prune is strictly "taken_at < cutoff"), one second older is not.
	cutoff := now.Add(-3 * 24 * time.Hour)
	seeds := map[string]time.Time{
		"1d":       now.Add(-24 * time.Hour),
		"2d":       now.Add(-48 * time.Hour),
		"boundary": cutoff,
		"expired":  cutoff.Add(-time.Second),
		"5d":       now.Add(-5 * 24 * time.Hour),
	}
	for label, at := range seeds {
		require.NoError(t, db.Create(&models.StorageSample{
			TakenAt:       at,
			DatabaseBytes: int64(len(label)),
			FSTotalBytes:  1099511627776,
		}).Error, label)
	}

	pruneStorageSamples(context.Background(), db, cfg)

	var kept []models.StorageSample
	require.NoError(t, db.Order("taken_at").Find(&kept).Error)
	var gotAt []time.Time
	for _, s := range kept {
		gotAt = append(gotAt, s.TakenAt.UTC())
	}
	assert.Equal(t, []time.Time{cutoff, seeds["2d"], seeds["1d"]}, gotAt,
		"only rows at or after the cutoff survive the production prune")
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

const gb = uint64(1) << 30

func gbFree(n uint64) uint64 { return n * gb }

// pctOnlyLimits is the floors-at-0 escape hatch: today's percentage-only rule.
var pctOnlyLimits = StorageLimits{WarnPercent: 75, CriticalPercent: 90}

// defaultLimits mirrors the shipped defaults (75/90 and 10 GB / 2 GB floors).
var defaultLimits = StorageLimits{
	WarnPercent: 75, CriticalPercent: 90,
	WarnMinFreeBytes: 10 * gb, CriticalMinFreeBytes: 2 * gb,
}

// Issue #2182: the tier needs the percentage AND the absolute free-space floor.
func TestStorageThresholdTier_FreeSpaceFloor(t *testing.T) {
	tests := []struct {
		name string
		pct  int
		free uint64
		lim  StorageLimits
		want string
	}{
		{"80% used, 200 GB free is ok", 80, gbFree(200), defaultLimits, StorageThresholdOK},
		{"80% used, 5 GB free warns", 80, gbFree(5), defaultLimits, StorageThresholdWarning},
		{"95% used, 50 GB free is ok", 95, gbFree(50), defaultLimits, StorageThresholdOK},
		{"95% used, 5 GB free warns, not critical", 95, gbFree(5), defaultLimits, StorageThresholdWarning},
		{"95% used, 1 GB free is critical", 95, gbFree(1), defaultLimits, StorageThresholdCritical},
		{"low pct with tiny free is ok", 50, gbFree(1), defaultLimits, StorageThresholdOK},
		{"exactly at warn floor is ok (strictly below)", 80, 10 * gb, defaultLimits, StorageThresholdOK},
		{"one byte under warn floor warns", 80, 10*gb - 1, defaultLimits, StorageThresholdWarning},
		{"exactly at critical floor is not critical", 95, 2 * gb, defaultLimits, StorageThresholdWarning},
		{"floors 0 = percentage only: 80% warns", 80, gbFree(200), pctOnlyLimits, StorageThresholdWarning},
		{"floors 0 = percentage only: 95% critical", 95, gbFree(200), pctOnlyLimits, StorageThresholdCritical},
		{"warn floor only, critical pct-only", 95, gbFree(50), StorageLimits{WarnPercent: 75, CriticalPercent: 90, WarnMinFreeBytes: 10 * gb}, StorageThresholdCritical},
		{"critical floor only, warn pct-only", 80, gbFree(200), StorageLimits{WarnPercent: 75, CriticalPercent: 90, CriticalMinFreeBytes: 2 * gb}, StorageThresholdWarning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, storageThresholdTier(tt.pct, tt.free, tt.lim, StorageThresholdOK))
		})
	}
}

func TestStorageThresholdTier_FreeSpaceHysteresis(t *testing.T) {
	// Warning entered; free recovers just above the floor: still within the
	// hysteresis band, so it holds.
	assert.Equal(t, StorageThresholdWarning,
		storageThresholdTier(80, 10*gb+gb/2, defaultLimits, StorageThresholdWarning))
	// Free well above the floor clears it.
	assert.Equal(t, StorageThresholdOK,
		storageThresholdTier(80, gbFree(20), defaultLimits, StorageThresholdWarning))
	// Critical holds while free is just above its floor and usage is in band.
	assert.Equal(t, StorageThresholdCritical,
		storageThresholdTier(95, 2*gb+gb/20, defaultLimits, StorageThresholdCritical))
	// Critical clears to warning (not ok) once free is well above its floor.
	assert.Equal(t, StorageThresholdWarning,
		storageThresholdTier(95, gbFree(5), defaultLimits, StorageThresholdCritical))
}

func TestStorageThreshold_PersistedWithFreeFloor(t *testing.T) {
	db, ctx := storageThresholdEnv(t)
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 80, gbFree(200), defaultLimits))
	assert.Equal(t, StorageThresholdWarning, StorageThreshold(ctx, db, 80, gbFree(5), defaultLimits))
	assert.Equal(t, StorageThresholdOK, StorageThreshold(ctx, db, 80, gbFree(200), defaultLimits))
}

func TestStorageLimitsFromConfig(t *testing.T) {
	cfg := config.Config{StorageWarnPercent: 80, StorageCriticalPercent: 95, StorageWarnMinFreeGB: 10, StorageCriticalMinFreeGB: 2}
	lim := StorageLimitsFromConfig(cfg)
	assert.Equal(t, StorageLimits{WarnPercent: 80, CriticalPercent: 95, WarnMinFreeBytes: 10 * gb, CriticalMinFreeBytes: 2 * gb}, lim)

	// Zero percents fall back to defaults; zero floors stay 0 (pct-only).
	lim = StorageLimitsFromConfig(config.Config{})
	assert.Equal(t, config.DefaultStorageWarnPercent, lim.WarnPercent)
	assert.Equal(t, config.DefaultStorageCriticalPercent, lim.CriticalPercent)
	assert.Zero(t, lim.WarnMinFreeBytes)
	assert.Zero(t, lim.CriticalMinFreeBytes)
}
