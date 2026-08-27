package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Alert condition keys — the stable primary keys of alert_states rows (issue
// #428). "sync:*" keys are namespaced by subsystem so the two sync subsystems
// alert independently.
const (
	alertConditionKeyBackup        = "backup"
	alertConditionKeyBackupStale   = "backup_stale"
	alertConditionKeySyncContact   = "sync:contact_sync"
	alertConditionKeySyncCalendar  = "sync:calendar_sync"
	alertConditionKeyNotifications = "notifications"
	alertConditionKeyIntegrations  = "integrations"
	alertConditionKeyDBIntegrity   = "db_integrity"
	alertConditionKeyDiskSpace     = "disk_space"
	alertConditionKeyJobStopped    = "job_stopped"
)

// alertConditionResult is one condition's verdict for a single evaluation run.
type alertConditionResult struct {
	key    string
	title  string // human-facing name for the notification subject
	firing bool
	detail string
	// failureCount feeds "recovered after N failures" — the consecutive
	// failure run for the subsystem conditions, the stale-job count for
	// job_stopped, unused (0) for the pure threshold conditions.
	failureCount int
}

// diskUsageFn resolves the used-space percentage of the filesystem holding
// path. Indirected for tests.
var diskUsageFn = statfsDiskUsage

func statfsDiskUsage(path string) (int, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return 0, fmt.Errorf("statfs reported zero blocks for %s", path)
	}
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	return int(used * 100 / total), nil
}

// evaluateAlertConditions computes every enabled condition's verdict for one
// run. `health` is services.ComputeSubsystemHealth's output (issue #427);
// `prev` is the current alert_states rows keyed by condition_key, used only by
// conditions with hysteresis (disk_space).
func evaluateAlertConditions(ctx context.Context, db *gorm.DB, cfg config.Config, health []SubsystemHealth, prev map[string]models.AlertState) []alertConditionResult {
	byName := make(map[string]SubsystemHealth, len(health))
	for _, h := range health {
		byName[h.Subsystem] = h
	}

	var out []alertConditionResult

	if cfg.AlertBackupEnabled {
		b := byName[logger.ComponentBackup]
		out = append(out, alertConditionResult{
			key:          alertConditionKeyBackup,
			title:        "Backup",
			firing:       b.Status == SubsystemStatusFailing,
			detail:       backupDetail(b),
			failureCount: b.ConsecutiveFailures,
		})
		out = append(out, backupStaleCondition(b, cfg))
	}

	out = append(out,
		syncCondition(alertConditionKeySyncContact, "Contact sync", byName[logger.ComponentContactSync], cfg.AlertSyncFailureThreshold),
		syncCondition(alertConditionKeySyncCalendar, "Calendar sync", byName[logger.ComponentCalendarSync], cfg.AlertSyncFailureThreshold),
		syncCondition(alertConditionKeyNotifications, "Notification delivery", byName[logger.ComponentNotify], cfg.AlertNotifyFailureThreshold),
		integrationsCondition(byName[logger.ComponentWebhook], cfg),
	)

	if cfg.AlertDBIntegrityEnabled {
		out = append(out, dbIntegrityCondition(db))
	}
	if cfg.AlertDiskUsagePercent > 0 {
		out = append(out, diskSpaceCondition(cfg, prev[alertConditionKeyDiskSpace]))
	}
	if cfg.AlertJobStoppedEnabled {
		out = append(out, jobStoppedCondition(ctx, db, cfg))
	}
	return out
}

func backupDetail(b SubsystemHealth) string {
	if b.Status != SubsystemStatusFailing {
		return ""
	}
	if b.LastError != "" {
		return "Last error: " + b.LastError
	}
	return "The last backup / restore-drill run failed."
}

// backupStaleCondition fires when backups have succeeded before but not
// recently. "Never succeeded" is deliberately not-firing: on a fresh instance
// the plain backup condition covers a broken drill, and a not-yet-run drill is
// not an incident.
func backupStaleCondition(b SubsystemHealth, cfg config.Config) alertConditionResult {
	maxAgeHours := cfg.AlertBackupMaxAgeHours
	if maxAgeHours == 0 {
		maxAgeHours = 2 * cfg.DBRestoreDrillIntervalHours
	}
	r := alertConditionResult{key: alertConditionKeyBackupStale, title: "Backup freshness"}
	if b.LastSuccessAt == nil {
		return r
	}
	age := time.Since(*b.LastSuccessAt)
	if age > time.Duration(maxAgeHours)*time.Hour {
		r.firing = true
		r.detail = fmt.Sprintf("Last successful backup was %s ago (threshold %dh).",
			age.Round(time.Hour), maxAgeHours)
	}
	return r
}

// syncCondition fires when a subsystem is failing AND has failed at least
// `threshold` times in a row — a single transient failure is not an alert, a
// run that repeats is. Recovers when the subsystem reports healthy again.
func syncCondition(key, title string, h SubsystemHealth, threshold int) alertConditionResult {
	r := alertConditionResult{key: key, title: title, failureCount: h.ConsecutiveFailures}
	if h.Status == SubsystemStatusFailing && h.ConsecutiveFailures >= threshold {
		r.firing = true
		r.detail = fmt.Sprintf("%d consecutive failures", h.ConsecutiveFailures)
		if h.IncidentFirstFailureAt != nil {
			r.detail += ", incident started " + h.IncidentFirstFailureAt.UTC().Format(time.RFC3339)
		}
		if h.LastError != "" {
			r.detail += ". Last error: " + h.LastError
		}
	}
	return r
}

// integrationsCondition fires when the webhook subsystem is failing and its
// most recent failure is recent. The webhook subsystem emits no success token
// (#427 known limitation), so recovery cannot be "a success landed" — instead
// it clears once no new integration_failed event has landed for the configured
// quiet window.
func integrationsCondition(h SubsystemHealth, cfg config.Config) alertConditionResult {
	r := alertConditionResult{key: alertConditionKeyIntegrations, title: "Integrations", failureCount: h.ConsecutiveFailures}
	quiet := time.Duration(cfg.AlertIncidentQuietHours) * time.Hour
	if h.Status == SubsystemStatusFailing && h.LastFailureAt != nil && time.Since(*h.LastFailureAt) <= quiet {
		r.firing = true
		r.detail = fmt.Sprintf("%d webhook delivery failures; last at %s",
			h.ConsecutiveFailures, h.LastFailureAt.UTC().Format(time.RFC3339))
		if h.LastError != "" {
			r.detail += ". Last error: " + h.LastError
		}
	}
	return r
}

// dbIntegrityCondition reads the persisted DB-integrity-check outcome directly:
// the check emits no system_event, so ComputeSubsystemHealth cannot see it.
func dbIntegrityCondition(db *gorm.DB) alertConditionResult {
	r := alertConditionResult{key: alertConditionKeyDBIntegrity, title: "Database integrity"}
	// Find, not First: a not-yet-run check is the common case and must not log
	// a "record not found" every evaluation (subsystem_health.go's idiom).
	var rows []models.OperationalCheckResult
	if err := db.Where("check_name = ?", models.JobNameDBIntegrityCheck).Limit(1).Find(&rows).Error; err != nil {
		logger.Error().Err(err).Msg("alerting: failed to read db-integrity check result")
		return r
	}
	if len(rows) == 0 {
		return r // never run — not an incident
	}
	row := rows[0]
	if row.Status == models.OpCheckStatusFailed || row.Status == models.OpCheckStatusError {
		r.firing = true
		r.detail = "PRAGMA integrity_check " + row.Status
		if row.Detail != "" {
			r.detail += ": " + row.Detail
		}
	}
	return r
}

// diskSpaceCondition fires at cfg.AlertDiskUsagePercent and clears only once
// usage drops 5 points below it — hysteresis so a filesystem hovering at the
// threshold does not flap.
func diskSpaceCondition(cfg config.Config, prev models.AlertState) alertConditionResult {
	r := alertConditionResult{key: alertConditionKeyDiskSpace, title: "Disk space"}
	dir := filepath.Dir(cfg.DBPath)
	used, err := diskUsageFn(dir)
	if err != nil {
		logger.Error().Err(err).Str("path", dir).Msg("alerting: failed to stat filesystem")
		return r
	}
	threshold := cfg.AlertDiskUsagePercent
	clearBelow := threshold - 5
	if clearBelow < 1 {
		clearBelow = 1
	}
	wasAlerting := prev.State == models.AlertStateAlerting
	if used >= threshold || (wasAlerting && used >= clearBelow) {
		r.firing = true
		r.detail = fmt.Sprintf("Filesystem holding %s is %d%% full (threshold %d%%).", dir, used, threshold)
	}
	return r
}

// jobStaleDef is one scheduled job the job_stopped condition watches, with the
// interval it is expected to complete within. last_run_at only advances on a
// successful releaseJobLock, so staleness means "stopped running or failing
// every time".
type jobStaleDef struct {
	name     string
	interval time.Duration
}

func trackedJobs(cfg config.Config) []jobStaleDef {
	hours := func(h int) time.Duration { return time.Duration(h) * time.Hour }
	defs := []jobStaleDef{
		{models.JobNameDailyReminders, 24 * time.Hour},
		{models.JobNameCalendarSync, hours(cfg.CalDAVSyncIntervalHours)},
		{models.JobNameWebhookRetries, 5 * time.Minute},
		{models.JobNamePurgeDeleted, 24 * time.Hour},
		{models.JobNameAuditPurge, 24 * time.Hour},
		{models.JobNameSystemEventPurge, 24 * time.Hour},
		{models.JobNameCadenceOverdue, 24 * time.Hour},
		{models.JobNameReachOutDetection, 24 * time.Hour},
		{models.JobNameImmichSync, hours(cfg.ImmichSyncIntervalHours)},
	}
	if cfg.DBIntegrityCheckEnabled {
		defs = append(defs, jobStaleDef{models.JobNameDBIntegrityCheck, hours(cfg.DBIntegrityCheckIntervalHours)})
	}
	if cfg.DBRestoreDrillEnabled {
		defs = append(defs, jobStaleDef{models.JobNameRestoreDrill, hours(cfg.DBRestoreDrillIntervalHours)})
	}
	return defs
}

func jobStoppedCondition(ctx context.Context, db *gorm.DB, cfg config.Config) alertConditionResult {
	r := alertConditionResult{key: alertConditionKeyJobStopped, title: "Background jobs"}
	mult := time.Duration(cfg.AlertJobStaleMultiplier)

	var stale []string
	for _, j := range trackedJobs(cfg) {
		// Find, not First: a job that has never run (fresh instance) is normal
		// and must not log a "record not found" every evaluation.
		var rows []models.JobExecution
		if err := db.WithContext(ctx).Where("job_name = ?", j.name).Limit(1).Find(&rows).Error; err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("job", j.name).Msg("alerting: failed to read job execution")
			continue
		}
		if len(rows) == 0 || rows[0].LastRunAt.IsZero() {
			continue // never run yet — not stale
		}
		if age := time.Since(rows[0].LastRunAt); age > j.interval*mult {
			stale = append(stale, fmt.Sprintf("%s (last completed %s ago)", j.name, age.Round(time.Minute)))
		}
	}

	sort.Strings(stale)
	if len(stale) > 0 {
		r.firing = true
		r.failureCount = len(stale)
		r.detail = "Stale: " + strings.Join(stale, "; ")
	}
	return r
}
