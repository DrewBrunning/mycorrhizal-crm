package services

import (
	"context"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Per-job background-job run health, issue #391.
//
// "The reminder job failed" is far less useful than "the last success was
// 03:04, it has failed 9 times in a row since 03:19, and it normally takes 1s
// but the last run took 40s". ComputeJobRunHealth derives that per-job state
// by folding the persisted job_runs history (nothing is stored — the state is
// recomputed on read, so it survives a restart and can never drift from the
// rows it summarizes). It is the job-centric companion of ComputeSubsystemHealth
// (issue #427), which folds system_events per operational subsystem.
//
// A `skipped` run (job lock held, ran too recently) means the job did not
// execute, so it neither confirms a failure nor a recovery: skipped runs are
// transparent to status and to the consecutive-failure streak, and are
// excluded from the duration trend (their duration is ~0 and would skew it).

// Job-run health status values (aligned with SubsystemHealth's tri-state).
const (
	JobRunStatusHealthy = "healthy" // most recent executed run succeeded
	JobRunStatusFailing = "failing" // most recent executed run failed
	JobRunStatusUnknown = "unknown" // no executed run on record
)

// jobRunDurationSampleSize is how many recent executed runs the duration
// trend (avg / max) is computed over.
const jobRunDurationSampleSize = 20

// JobRunHealth is the folded state of one background job. Instance-wide (not
// user-scoped) and read-only — a projection of job_runs.
type JobRunHealth struct {
	JobName string `json:"job_name"`
	Status  string `json:"status"`

	// LastRunAt / LastResult / LastTrigger / LastItemsProcessed describe the
	// single most recent run of ANY result (including skipped) — what the job
	// did last. All zero when the job has never run.
	LastRunAt          *time.Time `json:"last_run_at"`
	LastResult         string     `json:"last_result"`
	LastTrigger        string     `json:"last_trigger"`
	LastDurationMS     *int64     `json:"last_duration_ms"`
	LastItemsProcessed *int       `json:"last_items_processed"`

	// LastSuccessAt / LastFailureAt are the most recent of each outcome,
	// ignoring skipped runs.
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastFailureAt *time.Time `json:"last_failure_at"`

	// LastError is the sanitized error of the most recent failure, empty
	// unless Status is failing.
	LastError string `json:"last_error"`

	// IncidentFirstFailureAt is the first failure in the current unbroken run
	// of failures — non-null exactly when ConsecutiveFailures > 0.
	IncidentFirstFailureAt *time.Time `json:"incident_first_failure_at"`
	ConsecutiveFailures    int        `json:"consecutive_failures"`

	// DurationSampleSize is how many recent executed runs AvgDurationMS /
	// MaxDurationMS were computed over (0 when the job has no executed run).
	DurationSampleSize int    `json:"duration_sample_size"`
	AvgDurationMS      *int64 `json:"avg_duration_ms"`
	MaxDurationMS      *int64 `json:"max_duration_ms"`
}

// ComputeJobRunHealth folds job_runs into one JobRunHealth per
// models.KnownJobNames, in that order (the API preserves it) — including jobs
// that have never run (status "unknown").
func ComputeJobRunHealth(ctx context.Context, db *gorm.DB) ([]JobRunHealth, error) {
	out := make([]JobRunHealth, 0, len(models.KnownJobNames))
	for _, name := range models.KnownJobNames {
		h, err := computeJobRunHealth(ctx, db, name)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

func computeJobRunHealth(ctx context.Context, db *gorm.DB, jobName string) (JobRunHealth, error) {
	h := JobRunHealth{JobName: jobName, Status: JobRunStatusUnknown}

	lastRun, err := latestJobRun(ctx, db, jobName, nil)
	if err != nil {
		return h, err
	}
	if lastRun == nil {
		return h, nil // never run
	}
	t := lastRun.StartedAt
	h.LastRunAt = &t
	h.LastResult = lastRun.Result
	h.LastTrigger = lastRun.Trigger
	d := lastRun.DurationMS
	h.LastDurationMS = &d
	h.LastItemsProcessed = lastRun.ItemsProcessed

	lastSuccess, err := latestJobRun(ctx, db, jobName, []string{models.JobRunResultSuccess})
	if err != nil {
		return h, err
	}
	lastFailure, err := latestJobRun(ctx, db, jobName, []string{models.JobRunResultFailure})
	if err != nil {
		return h, err
	}
	if lastSuccess != nil {
		s := lastSuccess.StartedAt
		h.LastSuccessAt = &s
	}
	if lastFailure != nil {
		f := lastFailure.StartedAt
		h.LastFailureAt = &f
	}

	switch {
	case lastSuccess == nil && lastFailure == nil:
		// Only skipped runs on record — the job has never actually executed.
		h.Status = JobRunStatusUnknown
	case lastFailure != nil && (lastSuccess == nil || afterJobRun(lastFailure, lastSuccess)):
		h.Status = JobRunStatusFailing
		h.LastError = lastFailure.Error
		count, first, err := jobRunFailureRunSince(ctx, db, jobName, lastSuccess)
		if err != nil {
			return h, err
		}
		h.ConsecutiveFailures = count
		if !first.IsZero() {
			ff := first
			h.IncidentFirstFailureAt = &ff
		}
	default:
		h.Status = JobRunStatusHealthy
	}

	avg, max, n, err := jobRunDurationTrend(ctx, db, jobName)
	if err != nil {
		return h, err
	}
	h.DurationSampleSize = n
	if n > 0 {
		a, m := avg, max
		h.AvgDurationMS = &a
		h.MaxDurationMS = &m
	}
	return h, nil
}

// latestJobRun returns the newest job_runs row for jobName whose result is one
// of results (any result when results is nil), or nil when there is none.
func latestJobRun(ctx context.Context, db *gorm.DB, jobName string, results []string) (*models.JobRun, error) {
	q := db.WithContext(ctx).Where("job_name = ?", jobName)
	if len(results) > 0 {
		q = q.Where("result IN ?", results)
	}
	var rows []models.JobRun
	if err := q.Order("started_at DESC, id DESC").Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// jobRunFailureRunSince counts the unbroken run of failure rows for jobName
// that occurred after its last success (all failures on record when there is
// no success), and returns that run's earliest start — the first failure of
// the current incident. Skipped rows are irrelevant: they are not counted and
// they do not break the streak.
func jobRunFailureRunSince(ctx context.Context, db *gorm.DB, jobName string, lastSuccess *models.JobRun) (int, time.Time, error) {
	where := func() *gorm.DB {
		q := db.WithContext(ctx).Model(&models.JobRun{}).
			Where("job_name = ? AND result = ?", jobName, models.JobRunResultFailure)
		if lastSuccess != nil {
			q = q.Where("started_at > ? OR (started_at = ? AND id > ?)",
				lastSuccess.StartedAt, lastSuccess.StartedAt, lastSuccess.ID)
		}
		return q
	}

	var count int64
	if err := where().Count(&count).Error; err != nil {
		return 0, time.Time{}, err
	}
	if count == 0 {
		return 0, time.Time{}, nil
	}

	var earliest []models.JobRun
	if err := where().Order("started_at ASC, id ASC").Limit(1).Find(&earliest).Error; err != nil {
		return 0, time.Time{}, err
	}
	if len(earliest) == 0 {
		return int(count), time.Time{}, nil
	}
	return int(count), earliest[0].StartedAt, nil
}

// jobRunDurationTrend returns the average and maximum duration (ms) over the
// last jobRunDurationSampleSize executed (non-skipped) runs of jobName, plus
// how many rows were sampled.
func jobRunDurationTrend(ctx context.Context, db *gorm.DB, jobName string) (avg int64, max int64, n int, err error) {
	var rows []models.JobRun
	if err = db.WithContext(ctx).
		Where("job_name = ? AND result <> ?", jobName, models.JobRunResultSkipped).
		Order("started_at DESC, id DESC").
		Limit(jobRunDurationSampleSize).
		Find(&rows).Error; err != nil {
		return 0, 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, 0, nil
	}
	var total int64
	for _, r := range rows {
		total += r.DurationMS
		if r.DurationMS > max {
			max = r.DurationMS
		}
	}
	return total / int64(len(rows)), max, len(rows), nil
}

// afterJobRun reports whether a started after b, breaking an exact-timestamp
// tie by autoincrement id.
func afterJobRun(a, b *models.JobRun) bool {
	if a.StartedAt.Equal(b.StartedAt) {
		return a.ID > b.ID
	}
	return a.StartedAt.After(b.StartedAt)
}

// JobRunFilter narrows a ListJobRuns query. All fields are optional.
type JobRunFilter struct {
	JobName string
	Result  string
	Since   *time.Time
	Until   *time.Time
	Limit   int
}

// jobRunListDefaultLimit / jobRunListMaxLimit bound the history page size.
const (
	jobRunListDefaultLimit = 100
	jobRunListMaxLimit     = 500
)

// ListJobRuns returns background-job run history newest-first, for the
// admin monitor's per-job drill-down.
func ListJobRuns(ctx context.Context, db *gorm.DB, f JobRunFilter) ([]models.JobRun, error) {
	q := db.WithContext(ctx).Model(&models.JobRun{})
	if f.JobName != "" {
		q = q.Where("job_name = ?", f.JobName)
	}
	if f.Result != "" {
		q = q.Where("result = ?", f.Result)
	}
	if f.Since != nil {
		q = q.Where("started_at >= ?", *f.Since)
	}
	if f.Until != nil {
		q = q.Where("started_at <= ?", *f.Until)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = jobRunListDefaultLimit
	}
	if limit > jobRunListMaxLimit {
		limit = jobRunListMaxLimit
	}

	var rows []models.JobRun
	if err := q.Order("started_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
