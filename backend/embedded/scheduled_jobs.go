package embedded

import (
	"fmt"

	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

// This file extracts the scheduler wiring (originally inline in main()) into a
// standalone, testable function. Before this, nothing tested that every
// recurring job was actually registered with the scheduler — deleting a purge
// registration line (a real data-retention regression) failed no test at all —
// and every s.Every(...).Do(...) call discarded its error, so a malformed .At()
// time or an invalid interval would silently produce a job that never runs
// instead of failing startup.

// manualJobReasons lists every models.JobName* token that is deliberately
// NOT registered by registerScheduledJobs, with the reason it is
// operator-triggered / on-demand only rather than scheduled. Keep this in
// sync with the doc comments in models/job_execution.go — the every-job-name
// completeness test AST-scans that file for every JobName* constant and fails
// if one is neither registered here nor listed in this allowlist, so a new
// job name is caught automatically.
var manualJobReasons = map[string]string{
	models.JobNameSearchIndexRebuild: "operator-triggered via POST /admin/search/rebuild " +
		"and auto-run after a restore; not on a schedule (see its doc comment in " +
		"models/job_execution.go)",
	models.JobNameDerivedColumnsRebuild: "operator-triggered via POST /admin/contacts/rebuild-derived " +
		"and auto-run after a restore; not on a schedule (see its doc comment in " +
		"models/job_execution.go)",
}

// embeddedDisabledJobs is the set of models.JobName* tokens that are NOT
// registered in the embedded deployment (ADR 0028 Decision 2, spike #1256):
//
//   - restore_drill cannot place its backup snapshot under the Android app
//     cache dir (confirmed by the spike), and there is no operator to act on a
//     failed drill anyway;
//   - alert_eval dispatches through webhook + per-admin personal channels,
//     none of which exist on a single-user phone;
//   - webhook_retries / webhook_delivery_purge only serve the webhook surface,
//     which embedded mode does not register;
//   - storage_sample tracks a storage-growth trend against operator threshold
//     alerts, which do not exist on-device.
//
// Everything else — reminders, cadence, reach-out, the data purges, the
// integrity check, and calendar/Immich sync when the user configured them — is
// registered in both deployments.
var embeddedDisabledJobs = map[string]bool{
	models.JobNameRestoreDrill:         true,
	models.JobNameAlertEval:            true,
	models.JobNameWebhookRetries:       true,
	models.JobNameWebhookDeliveryPurge: true,
	models.JobNameStorageSample:        true,
}

// reminderTask builds the daily reminder digest + push-style channel send.
// Reports the number of sends that succeeded; a send failure marks the run
// failed (issue #391 item 3).
func reminderTask(db *gorm.DB, cfg config.Config) func() (int, error) {
	return func() (int, error) { return services.SendRemindersWithRateLimit(db, cfg) }
}

func webhookRetriesTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.ProcessWebhookRetries(db, cfg)
		return nil
	}
}

// calendarSyncTask syncs calendar subscriptions regularly (rate-limited via
// job lock).
func calendarSyncTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.SyncCalendarsWithRateLimit(db, cfg)
		return nil
	}
}

// cadenceOverdueTask emits overdue-cadence webhooks daily (T19). Reports the
// number emitted.
func cadenceOverdueTask(db *gorm.DB, cfg config.Config) func() (int, error) {
	return func() (int, error) { return services.ProcessOverdueCadences(db, cfg) }
}

// reachOutTask detects event-driven reach-out suggestions daily (issue #177).
// Reports the number of suggestions created.
func reachOutTask(db *gorm.DB, cfg config.Config) func() (int, error) {
	return func() (int, error) { return services.DetectReachOutSuggestions(db, cfg) }
}

// immichSyncTask syncs Immich enrichment regularly (T16).
func immichSyncTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.SyncImmichWithRateLimit(db, cfg)
		return nil
	}
}

// dbIntegrityTask checks the live database for corruption on a schedule
// (issue #273). Config-gated (DB_INTEGRITY_CHECK_ENABLED).
func dbIntegrityTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.CheckDBIntegrityScheduled(db, cfg)
		return nil
	}
}

// restoreDrillTask periodically proves a backup actually restores (issue
// #275). Config-gated (DB_RESTORE_DRILL_ENABLED).
func restoreDrillTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.RunRestoreDrillScheduled(db, cfg)
		return nil
	}
}

// alertEvalTask evaluates alert conditions on a schedule (issue #428).
// Config-gated (ALERTING_ENABLED).
func alertEvalTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.EvaluateAlerts(db, cfg)
		return nil
	}
}

// storageSampleTask is the daily storage-growth sampler (issue #652).
func storageSampleTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error {
		services.RecordStorageSampleScheduled(db, cfg)
		return nil
	}
}

// scheduledJobRegistration pairs a models.JobName* token with the closure
// that registers it on s. Kept as a slice (rather than a map) so
// registration order matches the original main.go ordering exactly, which
// matters for readability of a startup failure but not for correctness.
type scheduledJobRegistration struct {
	jobName  string
	register func(s *gocron.Scheduler) (*gocron.Job, error)
}

// scheduledJobRegistrations is the ordered list of recurring jobs
// registerScheduledJobs wires up. Split out from the loop so the embedded-mode
// gating and the completeness test can both read it.
func scheduledJobRegistrations(db *gorm.DB, cfg *config.Config) []scheduledJobRegistration {
	return []scheduledJobRegistration{
		{models.JobNameDailyReminders, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(1).Day().At(cfg.ReminderTime).Do(
				recoverJobReport(db, models.JobNameDailyReminders, models.JobTriggerScheduled, reminderTask(db, *cfg)))
		}},
		{models.JobNameWebhookRetries, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(5).Minutes().Do(
				recoverJob(db, models.JobNameWebhookRetries, models.JobTriggerScheduled, webhookRetriesTask(db, *cfg)))
		}},
		{models.JobNameCalendarSync, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(cfg.CalDAVSyncIntervalHours).Hours().Do(
				recoverJob(db, models.JobNameCalendarSync, models.JobTriggerScheduled, calendarSyncTask(db, *cfg)))
		}},
		{models.JobNamePurgeDeleted, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNamePurgeDeleted, models.JobTriggerScheduled, purgeDeletedTask(db, *cfg)))
		}},
		{models.JobNameAuditPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNameAuditPurge, models.JobTriggerScheduled, auditPurgeTask(db, *cfg)))
		}},
		{models.JobNameSystemEventPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNameSystemEventPurge, models.JobTriggerScheduled, systemEventPurgeTask(db, *cfg)))
		}},
		{models.JobNameJobRunPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNameJobRunPurge, models.JobTriggerScheduled, jobRunPurgeTask(db, *cfg)))
		}},
		{models.JobNameWebhookDeliveryPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNameWebhookDeliveryPurge, models.JobTriggerScheduled, webhookDeliveryPurgeTask(db, *cfg)))
		}},
		{models.JobNameIdempotencyKeyPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(6).Hours().Do(
				recoverJob(db, models.JobNameIdempotencyKeyPurge, models.JobTriggerScheduled, idempotencyKeyPurgeTask(db, *cfg)))
		}},
		{models.JobNameSessionPurge, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(6).Hours().Do(
				recoverJob(db, models.JobNameSessionPurge, models.JobTriggerScheduled, sessionPurgeTask(db)))
		}},
		{models.JobNameCadenceOverdue, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJobReport(db, models.JobNameCadenceOverdue, models.JobTriggerScheduled, cadenceOverdueTask(db, *cfg)))
		}},
		{models.JobNameReachOutDetection, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJobReport(db, models.JobNameReachOutDetection, models.JobTriggerScheduled, reachOutTask(db, *cfg)))
		}},
		{models.JobNameImmichSync, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(cfg.ImmichSyncIntervalHours).Hours().Do(
				recoverJob(db, models.JobNameImmichSync, models.JobTriggerScheduled, immichSyncTask(db, *cfg)))
		}},
		{models.JobNameDBIntegrityCheck, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(cfg.DBIntegrityCheckIntervalHours).Hours().Do(
				recoverJob(db, models.JobNameDBIntegrityCheck, models.JobTriggerScheduled, dbIntegrityTask(db, *cfg)))
		}},
		{models.JobNameRestoreDrill, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(cfg.DBRestoreDrillIntervalHours).Hours().Do(
				recoverJob(db, models.JobNameRestoreDrill, models.JobTriggerScheduled, restoreDrillTask(db, *cfg)))
		}},
		{models.JobNameAlertEval, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(cfg.AlertEvalIntervalMinutes).Minutes().Do(
				recoverJob(db, models.JobNameAlertEval, models.JobTriggerScheduled, alertEvalTask(db, *cfg)))
		}},
		{models.JobNameStorageSample, func(s *gocron.Scheduler) (*gocron.Job, error) {
			return s.Every(24).Hours().Do(
				recoverJob(db, models.JobNameStorageSample, models.JobTriggerScheduled, storageSampleTask(db, *cfg)))
		}},
	}
}

// registerScheduledJobs wires every recurring background job onto s:
// checking every .Do()/.At() error (a malformed at-time or a zero/negative
// interval previously produced a job that silently never ran) and tagging
// each registered gocron.Job with its canonical models.JobName* token so a
// test can enumerate s.Jobs() and confirm every job that is supposed to be
// scheduled actually is. It returns the first registration error
// encountered; the caller should treat that as fatal (a job that failed to
// register at all is worse than one that runs).
//
// In embedded mode the tokens in embeddedDisabledJobs are skipped (ADR 0028
// Decision 2); server mode registers all of them.
//
// It does not fire any job's boot-time "Initial" trigger (see runJob's doc
// comment on trigger semantics) — those goroutines run real service logic
// against the live database and are dispatched separately, so this function is
// safe to call from a test against a scratch database.
func registerScheduledJobs(s *gocron.Scheduler, db *gorm.DB, cfg *config.Config) error {
	for _, reg := range scheduledJobRegistrations(db, cfg) {
		if cfg.IsEmbedded() && embeddedDisabledJobs[reg.jobName] {
			continue
		}
		job, err := reg.register(s)
		if err != nil {
			return fmt.Errorf("registering scheduled job %q: %w", reg.jobName, err)
		}
		job.Tag(reg.jobName)
	}

	return nil
}
