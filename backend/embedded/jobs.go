package embedded

import (
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/metrics"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"gorm.io/gorm"
)

// This file (with scheduled_jobs.go) is the scheduler orchestration that used
// to live in package main. It moved here so the library entry point
// (embedded.Start/Stop) can drive it, and so a test can drive it directly.

// runJob executes fn under a per-run correlation ID ("job:<name>:<uuid>") with
// panic recovery and standardized start/finish logging (issue #425), and
// persists one job_runs row per invocation (issue #391): job name, trigger,
// duration, and outcome — success, failure (a returned error or a recovered
// panic), or skipped (fn returns services.ErrJobSkipped: the job lock was held
// or it ran too recently). A recovered panic is additionally recorded as a
// job_failed operational event so it lands on the admin timeline (issue #424);
// the ordinary completion stays a log line + the job_runs row, not a
// system_events row, since the scheduler ticks often and a per-tick event
// would swamp that stream.
//
// jobName is the canonical models.JobName* token (so job_runs history groups
// per job regardless of which trigger fired it); trigger is
// models.JobTrigger{Scheduled,Initial}.
func runJob(db *gorm.DB, jobName, trigger string, fn func() error) {
	execJob(db, jobName, trigger, func() (*int, error) { return nil, fn() })
}

// runJobReport is runJob for a job that also reports how many items the run
// acted on (reminders sent, suggestions created) — persisted as
// job_runs.items_processed.
func runJobReport(db *gorm.DB, jobName, trigger string, fn func() (int, error)) {
	execJob(db, jobName, trigger, func() (*int, error) {
		n, err := fn()
		return &n, err
	})
}

// execJob is the shared implementation behind runJob / runJobReport.
func execJob(db *gorm.DB, jobName, trigger string, fn func() (*int, error)) {
	ctx := logger.JobContext(jobName)
	start := time.Now()
	logger.Ctx(ctx).Info().
		Str(logger.FieldEvent, models.SysEventJobStarted).
		Str(logger.FieldOperation, jobName).
		Str("trigger", trigger).
		Msg("scheduled job started")

	var (
		items  *int
		jobErr error
		panicV any
	)
	func() {
		defer func() { panicV = recover() }()
		items, jobErr = fn()
	}()

	durMS := time.Since(start).Milliseconds()
	result := logger.ResultSuccess
	errStr := ""

	switch {
	case panicV != nil:
		result = logger.ResultFailure
		errStr = fmt.Sprintf("panic: %v", panicV)
		items = nil
		logger.Ctx(ctx).Error().
			Str(logger.FieldEvent, models.SysEventJobFailed).
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Str(logger.FieldOperation, jobName).
			Str(logger.FieldResult, logger.ResultFailure).
			Int64(logger.FieldDurationMS, durMS).
			Interface("panic", panicV).
			Str("stack", string(debug.Stack())).
			Msg("scheduled job panicked — recovered")
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType:  models.SysEventJobFailed,
			Component:  logger.ComponentScheduler,
			Operation:  jobName,
			Result:     models.SysResult(logger.ResultFailure),
			DurationMS: &durMS,
			Error:      errStr,
		})
	case errors.Is(jobErr, services.ErrJobSkipped):
		result = logger.ResultSkipped
		items = nil
		logger.Ctx(ctx).Info().
			Str(logger.FieldEvent, models.SysEventJobCompleted).
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Str(logger.FieldOperation, jobName).
			Str(logger.FieldResult, logger.ResultSkipped).
			Int64(logger.FieldDurationMS, durMS).
			Msg("scheduled job skipped")
	case jobErr != nil:
		result = logger.ResultFailure
		errStr = jobErr.Error()
		items = nil
		logger.Ctx(ctx).Error().
			Str(logger.FieldEvent, models.SysEventJobFailed).
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Str(logger.FieldOperation, jobName).
			Str(logger.FieldResult, logger.ResultFailure).
			Int64(logger.FieldDurationMS, durMS).
			Err(jobErr).
			Msg("scheduled job failed")
	default:
		logger.Ctx(ctx).Info().
			Str(logger.FieldEvent, models.SysEventJobCompleted).
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Str(logger.FieldOperation, jobName).
			Str(logger.FieldResult, logger.ResultSuccess).
			Int64(logger.FieldDurationMS, durMS).
			Msg("scheduled job completed")
	}

	// Prometheus counters (issue #389): job_runs_total{job,result} +
	// job_duration_seconds{job}. result is already the folded outcome
	// (success / failure / skipped).
	metrics.JobRun(jobName, result, float64(durMS)/1000.0)

	models.RecordJobRun(ctx, db, models.JobRun{
		JobName:        jobName,
		Trigger:        trigger,
		StartedAt:      start,
		FinishedAt:     start.Add(time.Duration(durMS) * time.Millisecond),
		DurationMS:     durMS,
		Result:         result,
		Error:          errStr,
		ItemsProcessed: items,
	})
}

// safeGo runs fn in a goroutine via runJob (panic recovery + correlation ID +
// job_runs row), so an unhandled panic in a background task doesn't crash the
// server.
func safeGo(db *gorm.DB, jobName, trigger string, fn func() error) {
	go runJob(db, jobName, trigger, fn)
}

// safeGoReport is safeGo for a job that reports an item count (see runJobReport).
func safeGoReport(db *gorm.DB, jobName, trigger string, fn func() (int, error)) {
	go runJobReport(db, jobName, trigger, fn)
}

// recoverJob wraps fn for a recurring gocron registration (s.Every(...).Do(...)).
// gocron invokes the function it's given directly and, in the pinned v1.37.0,
// does not recover a job's own panics unless gocron.SetPanicHandler is set
// globally (it isn't here). Without this, a panic on any scheduled invocation
// would crash the whole process on its next scheduled tick. Always pass the
// wrapped result to .Do(), never fn itself.
func recoverJob(db *gorm.DB, jobName, trigger string, fn func() error) func() {
	return func() { runJob(db, jobName, trigger, fn) }
}

// recoverJobReport is recoverJob for a job that reports an item count.
func recoverJobReport(db *gorm.DB, jobName, trigger string, fn func() (int, error)) func() {
	return func() { runJobReport(db, jobName, trigger, fn) }
}

// The purge-task constructors below adapt each purge service's scheduled entry
// point into the func() error that execJob runs. They are package-level (rather
// than inline closures) so a test can assert the wiring: the error returned by
// the purge must reach execJob, otherwise a failing retention job records
// job_runs.result=success, last_run_at advances, and job_stopped can never fire
// (issue #975).
func purgeDeletedTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeDeletedRows(db, cfg) }
}

func auditPurgeTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeExpiredAuditEventsScheduled(db, cfg) }
}

func systemEventPurgeTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeExpiredSystemEventsScheduled(db, cfg) }
}

func jobRunPurgeTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeExpiredJobRunsScheduled(db, cfg) }
}

func webhookDeliveryPurgeTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeExpiredWebhookDeliveriesScheduled(db, cfg) }
}

func idempotencyKeyPurgeTask(db *gorm.DB, cfg config.Config) func() error {
	return func() error { return services.PurgeExpiredIdempotencyKeysScheduled(db, cfg) }
}

func sessionPurgeTask(db *gorm.DB) func() error {
	return func() error { return services.PurgeExpiredSessionsScheduled(db) }
}
