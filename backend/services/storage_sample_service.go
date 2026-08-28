package services

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/metrics"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Storage-growth trend, sampler and tiered thresholds (issue #652).
//
// /admin/system-status reports storage *point-in-time* (issue #388); this
// builds the small time-series that turns that into a trend: one
// storage_samples row per day (the sampler), growth deltas over 7/30/90-day
// windows plus a capacity-exhaustion projection (ComputeStorageTrend), and a
// tiered ok | warning | critical threshold with -5% hysteresis (StorageThreshold)
// that folds into the endpoint's overall status.

const (
	// storageSampleMinInterval is slightly less than the 24h cron cadence so a
	// natural clock-skew or overlap doesn't cause a skipped run (mirrors the
	// other daily jobs).
	storageSampleMinInterval = 23 * time.Hour

	// Trend windows (issue #652). Deltas are "latest sample minus the oldest
	// sample within the window".
	storageTrendWindow7D  = 7 * 24 * time.Hour
	storageTrendWindow30D = 30 * 24 * time.Hour
	storageTrendWindow90D = 90 * 24 * time.Hour

	// storageProjectionWindow is how much history the capacity projection fits;
	// storageProjectionMinSpan is the minimum elapsed history before a
	// projection is attempted (fewer than 14 days is too noisy to trust).
	storageProjectionWindow  = 30 * 24 * time.Hour
	storageProjectionMinSpan = 14 * 24 * time.Hour

	// storageThresholdHysteresis is the -5% hysteresis band reused from
	// diskSpaceCondition (alerting_conditions.go): once a tier is entered,
	// usage must drop 5 percentage points below its threshold before the tier
	// clears, so a filesystem hovering at the threshold doesn't flap.
	storageThresholdHysteresis = 5
)

// Storage threshold tier tokens (issue #652). The system-status storage block
// folds usage_percent against STORAGE_WARN_PERCENT / STORAGE_CRITICAL_PERCENT
// into one of these.
const (
	StorageThresholdOK       = "ok"
	StorageThresholdWarning  = "warning"
	StorageThresholdCritical = "critical"
)

// alertConditionKeyStorageThreshold is the alert_states row that persists the
// current storage tier so the hysteresis band survives restarts. It is NOT an
// alert condition the evaluator fires on (evaluateAlertConditions never returns
// this key) — it exists purely so StorageThreshold can read the previous tier
// back, mirroring how diskSpaceCondition reads prev from alert_states.
const alertConditionKeyStorageThreshold = "storage_threshold"

// StorageTrend is the derived storage-growth block on /admin/system-status
// (issue #652): growth deltas over the 7/30/90-day windows and the projected
// capacity-exhaustion date. Pointers so "not enough history" marshals as null,
// not zero — the frontend renders an em dash for those.
type StorageTrend struct {
	Growth7DBytes   *int64     `json:"growth_7d_bytes"`
	Growth30DBytes  *int64     `json:"growth_30d_bytes"`
	Growth90DBytes  *int64     `json:"growth_90d_bytes"`
	ProjectedFullAt *time.Time `json:"projected_full_at"`
}

// ComputeStorageTrend derives the growth deltas and capacity projection from
// the persisted storage_samples series. now is the reference time for the
// windows (tests seed samples relative to it). Deltas are over database_bytes
// — the app's own footprint — while the projection fits fs_used_bytes toward
// fs_total_bytes (the whole filesystem the DB sits on). Every field is null
// until the series has enough history, and the projection is additionally null
// for a flat or shrinking series.
func ComputeStorageTrend(ctx context.Context, db *gorm.DB, now time.Time) StorageTrend {
	trend := StorageTrend{}
	if db == nil {
		return trend
	}
	var samples []models.StorageSample
	if err := db.WithContext(ctx).Order("taken_at DESC").Find(&samples).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage trend: failed to read storage_samples")
		return trend
	}
	if len(samples) == 0 {
		return trend
	}
	trend.Growth7DBytes = growthOverWindow(samples, now, storageTrendWindow7D)
	trend.Growth30DBytes = growthOverWindow(samples, now, storageTrendWindow30D)
	trend.Growth90DBytes = growthOverWindow(samples, now, storageTrendWindow90D)
	trend.ProjectedFullAt = projectedFullAt(samples, now)
	return trend
}

// growthOverWindow is the latest sample's database_bytes minus the oldest
// in-window sample's. samples must be sorted taken_at DESC. Nil when the
// window holds fewer than two samples (nothing to measure growth over).
func growthOverWindow(samples []models.StorageSample, now time.Time, window time.Duration) *int64 {
	cutoff := now.Add(-window)
	// samples are DESC, so the oldest in-window sample is the last one whose
	// taken_at is >= cutoff; the first sample before the cutoff ends the run.
	oldestIdx := -1
	for i := range samples {
		if samples[i].TakenAt.Before(cutoff) {
			break
		}
		oldestIdx = i
	}
	if oldestIdx <= 0 {
		return nil
	}
	delta := samples[0].DatabaseBytes - samples[oldestIdx].DatabaseBytes
	return &delta
}

// projectedFullAt fits fs_used_bytes over the last storageProjectionWindow
// against time (least-squares) and extrapolates to fs_total_bytes. samples must
// be sorted taken_at DESC. Returns nil when there is fewer than 14 days of
// history, fewer than two in-window samples, or the fitted slope is <= 0
// (flat or shrinking — no exhaustion date to project).
func projectedFullAt(samples []models.StorageSample, now time.Time) *time.Time {
	cutoff := now.Add(-storageProjectionWindow)
	in := make([]models.StorageSample, 0, len(samples))
	for _, s := range samples {
		if !s.TakenAt.Before(cutoff) {
			in = append(in, s)
		}
	}
	if len(in) < 2 {
		return nil
	}
	oldest := in[len(in)-1] // samples are sorted desc, so the last is the oldest
	if in[0].TakenAt.Sub(oldest.TakenAt) < storageProjectionMinSpan {
		return nil
	}

	slope, intercept := fitUsedBytes(in)
	if slope <= 0 {
		return nil
	}

	// Time until the fitted line reaches fs_total_bytes, measured from the
	// oldest in-window sample (where intercept is the fit's value).
	remaining := float64(in[0].FSTotalBytes) - intercept
	if remaining <= 0 {
		// Already at/over capacity by the fit — project to the newest sample.
		t := in[0].TakenAt
		return &t
	}
	hours := remaining / slope
	t := oldest.TakenAt.Add(time.Duration(hours * float64(time.Hour)))
	return &t
}

// fitUsedBytes is a least-squares linear regression of fs_used_bytes against
// elapsed hours since the oldest in-window sample (x = 0 at t0). Returns the
// slope (bytes/hour) and the intercept (the fit's value at t0). A degenerate
// x-spread (all samples the same instant) yields a zero slope.
func fitUsedBytes(in []models.StorageSample) (slope, intercept float64) {
	n := len(in)
	t0 := in[n-1].TakenAt

	var sx, sy, sxx, sxy float64
	for _, s := range in {
		x := float64(s.TakenAt.Sub(t0)) / float64(time.Hour)
		y := float64(s.FSUsedBytes)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	mx := sx / float64(n)
	my := sy / float64(n)
	denom := sxx - float64(n)*mx*mx
	if denom == 0 {
		return 0, my
	}
	slope = (sxy - float64(n)*mx*my) / denom
	return slope, my - slope*mx
}

// StorageThreshold computes the current storage tier for usedPct with -5%
// hysteresis, persisting the tier in alert_states (key storage_threshold) so
// the band survives restarts — the same idea as diskSpaceCondition. warnPct /
// critPct of 0 fall back to the config defaults, so a raw config.Config built
// without LoadConfig still behaves. It never returns an error: a failure to
// read or persist the previous tier degrades to a fresh decision from the
// current usage.
func StorageThreshold(ctx context.Context, db *gorm.DB, usedPct, warnPct, critPct int) string {
	warn, crit := resolveStorageThresholds(warnPct, critPct)
	tier := storageThresholdTier(usedPct, warn, crit, storageThresholdPrev(ctx, db))
	persistStorageThreshold(ctx, db, tier)
	return tier
}

func resolveStorageThresholds(warnPct, critPct int) (int, int) {
	if warnPct <= 0 {
		warnPct = config.DefaultStorageWarnPercent
	}
	if critPct <= warnPct {
		critPct = config.DefaultStorageCriticalPercent
	}
	return warnPct, critPct
}

// storageThresholdTier is the pure threshold + hysteresis decision.
func storageThresholdTier(usedPct, warnPct, critPct int, prev string) string {
	if usedPct >= critPct {
		return StorageThresholdCritical
	}
	if prev == StorageThresholdCritical && usedPct >= critPct-storageThresholdHysteresis {
		return StorageThresholdCritical
	}
	if usedPct >= warnPct {
		return StorageThresholdWarning
	}
	if prev == StorageThresholdWarning && usedPct >= warnPct-storageThresholdHysteresis {
		return StorageThresholdWarning
	}
	return StorageThresholdOK
}

// storageThresholdPrev reads the persisted tier back. Find, not First: a
// never-computed threshold (fresh instance) is the common case and must not log
// a "record not found" every evaluation (subsystem_health.go's idiom).
func storageThresholdPrev(ctx context.Context, db *gorm.DB) string {
	if db == nil {
		return StorageThresholdOK
	}
	var rows []models.AlertState
	if err := db.WithContext(ctx).Where("condition_key = ?", alertConditionKeyStorageThreshold).Limit(1).Find(&rows).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage threshold: failed to read persisted tier")
		return StorageThresholdOK
	}
	if len(rows) == 0 || rows[0].State != models.AlertStateAlerting {
		return StorageThresholdOK
	}
	if rows[0].Detail == StorageThresholdCritical {
		return StorageThresholdCritical
	}
	return StorageThresholdWarning
}

// persistStorageThreshold upserts the tier into alert_states. alerting maps to
// the alert_states 'alerting' state with the tier in detail (warning or
// critical); ok maps to 'ok'.
func persistStorageThreshold(ctx context.Context, db *gorm.DB, tier string) {
	if db == nil {
		return
	}
	state := models.AlertStateOK
	detail := ""
	if tier != StorageThresholdOK {
		state = models.AlertStateAlerting
		detail = tier
	}

	var rows []models.AlertState
	if err := db.WithContext(ctx).Where("condition_key = ?", alertConditionKeyStorageThreshold).Limit(1).Find(&rows).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage threshold: failed to read persisted tier")
		return
	}
	if len(rows) == 0 {
		row := models.AlertState{ConditionKey: alertConditionKeyStorageThreshold, State: state, Since: time.Now().UTC(), Detail: detail}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("storage threshold: failed to persist tier")
		}
		return
	}
	if rows[0].State == state && rows[0].Detail == detail {
		return // no change — don't churn the updated_at
	}
	if err := db.WithContext(ctx).Model(&models.AlertState{}).
		Where("condition_key = ?", alertConditionKeyStorageThreshold).
		Updates(map[string]interface{}{"state": state, "detail": detail, "updated_at": time.Now().UTC()}).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage threshold: failed to persist tier")
	}
}

// RecordStorageSampleScheduled is the daily storage-growth sampler (issue
// #652): job-lock guarded so a multi-instance deploy writes exactly one
// storage_samples row per day, then prunes rows past the retention window and
// emits one system_events row for the operational timeline.
func RecordStorageSampleScheduled(db *gorm.DB, cfg config.Config) {
	ctx := logger.JobContext(models.JobNameStorageSample)

	acquired, err := acquireJobLock(db, models.JobNameStorageSample, storageSampleMinInterval)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage sampler: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameStorageSample, true); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("storage sampler: failed to release job lock")
		}
	}()

	RecordStorageSample(ctx, db, cfg)
}

// RecordStorageSample is the lock-free core of the sampler: measure the current
// on-disk footprint, insert one storage_samples row, and prune rows past the
// retention window. Split out so tests can drive it directly without the job
// lock. Reuses the same sizing helpers as the point-in-time block (issue #388):
// metrics.DatabaseBytes / metrics.FilesystemBytes / services.StorageUsage.
func RecordStorageSample(ctx context.Context, db *gorm.DB, cfg config.Config) {
	now := time.Now().UTC()

	used, total := int64(0), int64(0)
	if free, tot, ok := metrics.FilesystemBytes(filepath.Dir(cfg.DBPath)); ok {
		used = int64(tot - free)
		total = int64(tot)
	}

	photoBytes, attachmentBytes := storageDirBytes(cfg)

	sample := models.StorageSample{
		TakenAt:            now,
		DatabaseBytes:      metrics.DatabaseBytes(cfg.DBPath),
		FSUsedBytes:        used,
		FSTotalBytes:       total,
		PhotoDirBytes:      photoBytes,
		AttachmentDirBytes: attachmentBytes,
	}
	if err := db.WithContext(ctx).Create(&sample).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("storage sampler: failed to insert storage sample")
		return
	}

	pruneStorageSamples(ctx, db, cfg)

	detail := fmt.Sprintf("database=%d bytes", sample.DatabaseBytes)
	if total > 0 {
		detail += fmt.Sprintf(" fs_used=%d%%", int64(float64(used)/float64(total)*100))
	}
	models.RecordSystemEvent(ctx, db, models.SystemEvent{
		EventType: models.SysEventJobCompleted,
		Component: logger.ComponentStorage,
		Operation: models.JobNameStorageSample,
		Result:    models.SysResult(logger.ResultSuccess),
		Detail:    detail,
	})
}

// storageDirBytes walks the two configured storage directories, reusing the
// memoized StorageUsage helper. A missing directory yields a zero total (the
// walk's truthful answer for a not-yet-provisioned path).
func storageDirBytes(cfg config.Config) (int64, int64) {
	var photo, attach int64
	for _, d := range StorageUsage([]string{cfg.ProfilePhotoDir, cfg.AttachmentsDir}) {
		switch d.Path {
		case cfg.ProfilePhotoDir:
			photo = d.Bytes
		case cfg.AttachmentsDir:
			attach = d.Bytes
		}
	}
	return photo, attach
}

// pruneStorageSamples deletes storage_samples rows older than the retention
// window (STORAGE_SAMPLE_RETENTION_DAYS, default 180). Runs in the same
// scheduled job that writes, so the table's size is bounded by construction.
// A misconfigured <=0 retention is treated as "use the default", never "delete
// everything" (the same fail-safe posture as the other retention purges).
func pruneStorageSamples(ctx context.Context, db *gorm.DB, cfg config.Config) {
	days := cfg.StorageSampleRetentionDays
	if days <= 0 {
		days = config.DefaultStorageSampleRetentionDays
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	result := db.WithContext(ctx).Exec("DELETE FROM storage_samples WHERE taken_at < ?", cutoff)
	if result.Error != nil {
		logger.Ctx(ctx).Error().Err(result.Error).Msg("storage sampler: failed to prune expired samples")
		return
	}
	if result.RowsAffected > 0 {
		logger.Ctx(ctx).Info().
			Int64("rows", result.RowsAffected).
			Time("cutoff", cutoff).
			Str(logger.FieldComponent, logger.ComponentStorage).
			Msg("Pruned expired storage samples")
	}
}
