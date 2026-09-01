package services

import (
	"errors"
	"fmt"
	"os"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// The scheduled-job de-duplication / catch-up primitive (issue #526, ADR 0011).
//
// The scheduler is in-process gocron with no persistent schedule, so a fire
// time the process slept through is never replayed. What rescues a missed run
// is the paired boot-time `initial` trigger next to every scheduled job: on
// start each job fires immediately, and acquireJobLock suppresses it if the
// row's LastRunAt is inside the job's de-dup window. That gives
// "catch up on next start, exactly once per scheduled period":
//
//  1. a missed occurrence fires at the next opportunity (process start / next tick);
//  2. at most one logical occurrence per job per scheduled period runs — two
//     missed daily occurrences produce one run, not two;
//  3. de-dup is ultimately on the user-visible event too (notificationDeliveryKey
//     for reminders), not only the job row;
//  4. the outcome is recorded on JobExecution (ran / deduped / caught_up /
//     failed), so an operator can tell a caught-up run from a suppressed one.

// JobCatchupMargin is the single shared slack subtracted from a job's
// scheduled period to get its de-dup window. One constant for the whole fleet
// (ADR 0011) — it must be large enough to absorb clock skew and a slow run so
// the *scheduled* tick right after a boot `initial` run is still suppressed,
// and small enough that a genuinely missed occasion is not.
const JobCatchupMargin = 30 * time.Minute

// JobCatchupWindow returns the de-dup window for a job scheduled every
// `period`: the period minus JobCatchupMargin, so the scheduled tick that
// fires right after a boot `initial` run is still suppressed while a genuinely
// missed occurrence (a full period elapsed) is not. For a sub-hour job the
// margin is clamped to a quarter of the period so the window stays close to
// `period` instead of collapsing. A run is suppressed when the last
// successful run was inside this window.
func JobCatchupWindow(period time.Duration) time.Duration {
	if period <= 0 {
		return JobCatchupMargin
	}
	margin := JobCatchupMargin
	if q := period / 4; margin > q {
		margin = q
	}
	return period - margin
}

// errJobRanTooRecently is the sentinel acquireJobLock's transaction returns
// when it suppresses a run inside the de-dup window (as opposed to the lock
// being held by another instance). It lets the caller record the `deduped`
// outcome outside the rolled-back transaction.
var errJobRanTooRecently = errors.New("job ran too recently")

// getInstanceID returns a unique identifier for this server instance.
func getInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// acquireJobLock attempts to acquire the lock for jobName. Returns true if the
// caller may run the job, false if it was run inside `minInterval` (derive it
// with JobCatchupWindow) or is currently locked by another instance.
//
// It also records the run's outcome on the JobExecution row (issue #526 rule
// 4): `deduped` when suppressed inside the window; `caught_up` when acquired
// after the row sat idle for >= 2*minInterval (roughly two full periods — an
// unambiguous "the process was down long enough to miss an occurrence" signal
// that never mislabels a normal cadence run); otherwise `running`, which
// releaseJobLock finalises to `ran` / `failed`.
func acquireJobLock(db *gorm.DB, jobName string, minInterval time.Duration) (bool, error) {
	now := time.Now()
	instanceID := getInstanceID()
	lockTimeout := 5 * time.Minute // Consider locks stale after 5 minutes

	err := db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution

		lookupErr := tx.Where("job_name = ?", jobName).First(&job).Error
		if lookupErr != nil && lookupErr != gorm.ErrRecordNotFound {
			return lookupErr
		}

		if lookupErr == gorm.ErrRecordNotFound {
			job = models.JobExecution{
				JobName:     jobName,
				LastRunAt:   now,
				LockedAt:    &now,
				LockedBy:    instanceID,
				LastOutcome: models.JobOutcomeRunning,
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock (first run)")
			return nil
		}

		timeSinceLastRun := now.Sub(job.LastRunAt)
		if timeSinceLastRun < minInterval {
			logger.Info().
				Str("job", jobName).
				Dur("since_last_run", timeSinceLastRun).
				Dur("min_interval", minInterval).
				Msg("Skipping job - ran too recently")
			return errJobRanTooRecently
		}

		// Another instance holding a fresh lock wins.
		if job.LockedAt != nil {
			lockAge := now.Sub(*job.LockedAt)
			if lockAge < lockTimeout && job.LockedBy != instanceID {
				logger.Info().
					Str("job", jobName).
					Str("locked_by", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Skipping job - locked by another instance")
				return fmt.Errorf("job locked by another instance")
			}
			if lockAge >= lockTimeout {
				logger.Warn().
					Str("job", jobName).
					Str("previous_instance", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Taking over stale lock")
			}
		}

		job.LockedAt = &now
		job.LockedBy = instanceID
		if timeSinceLastRun >= 2*minInterval {
			job.LastOutcome = models.JobOutcomeCaughtUp
		} else {
			job.LastOutcome = models.JobOutcomeRunning
		}
		if err := tx.Save(&job).Error; err != nil {
			return err
		}

		logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock")
		return nil
	})

	if errors.Is(err, errJobRanTooRecently) {
		// Record the suppression outside the rolled-back transaction (rule 4).
		if uErr := db.Model(&models.JobExecution{}).
			Where("job_name = ?", jobName).
			Update("last_outcome", models.JobOutcomeDeduped).Error; uErr != nil {
			logger.Warn().Err(uErr).Str("job", jobName).Msg("Failed to record deduped outcome")
		}
		return false, nil
	}
	return err == nil, nil
}

// releaseJobLock releases the lock, updates LastRunAt on success, and finalises
// the outcome recorded by acquireJobLock: a `running` marker becomes `ran`, a
// `caught_up` marker is kept, and any outcome on a failed run becomes `failed`.
func releaseJobLock(db *gorm.DB, jobName string, success bool) error {
	now := time.Now()
	instanceID := getInstanceID()

	return db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution
		if err := tx.Where("job_name = ?", jobName).First(&job).Error; err != nil {
			return err
		}

		if job.LockedBy != instanceID {
			logger.Warn().
				Str("job", jobName).
				Str("expected", instanceID).
				Str("actual", job.LockedBy).
				Msg("Lock was taken by another instance")
			return nil
		}

		if success {
			job.LastRunAt = now
			if job.LastOutcome == models.JobOutcomeRunning || job.LastOutcome == "" {
				job.LastOutcome = models.JobOutcomeRan
			}
			// A caught_up marker is deliberately preserved.
		} else {
			job.LastOutcome = models.JobOutcomeFailed
		}
		job.LockedAt = nil
		job.LockedBy = ""

		return tx.Save(&job).Error
	})
}
