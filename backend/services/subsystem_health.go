package services

import (
	"context"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Per-subsystem last-known-good (LKG) state, issue #427.
//
// "A sync failed" is far less useful than "the last success was 17:04, it has
// failed 9 times in a row, and the first failure in this incident was 17:19".
// ComputeSubsystemHealth derives that per-subsystem state by folding the
// persisted operational-event stream (system_events, issue #424) — each
// completed / failed / sent event advances the fold. Nothing is stored: the
// state is recomputed on read, so it survives a restart for free and can never
// drift from the events it summarizes (the issue's "derive from the event
// stream rather than a parallel write path").
//
// This is the cross-subsystem generalization of models.SyncHealthFields
// (issue #390's per-subscription shape); the field names are kept aligned so
// the two converge. Consumers: /metrics (#389), error aggregation (#426),
// alerting on Healthy<->Failing transitions (#428).
//
// Known limitation: the scheduler and webhook subsystems emit only a failure
// token today (scheduler: job_failed on a recovered panic — ordinary
// completions are log lines by design, ADR 0005; webhook: integration_failed
// once a delivery exhausts its retry budget). They can therefore report
// `failing` / `unknown` and a rising consecutive-failure run, but not
// `healthy`, and their incident cannot auto-close until a success-side event
// exists (webhook: #422). The success/failure token sets below are lists so a
// later producer emitting job_completed / backup_completed needs no change
// here.

// Subsystem health status values.
const (
	SubsystemStatusHealthy = "healthy" // most recent terminal event is a success
	SubsystemStatusFailing = "failing" // most recent terminal event is a failure
	SubsystemStatusUnknown = "unknown" // no completed/failed event on record
)

// SubsystemHealth is the LKG state of one operational subsystem. Instance-wide
// (not user-scoped) and read-only — it is a projection of system_events.
type SubsystemHealth struct {
	// Subsystem is the logger.Component* value its producer stamps on the
	// event, which is also the system_events.component filter.
	Subsystem string `json:"subsystem"`
	Status    string `json:"status"`

	// LastAttemptAt is the most recent terminal event (success or failure);
	// LastSuccessAt / LastFailureAt are the most recent of each outcome. All
	// null when the subsystem has produced no terminal event yet.
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastFailureAt *time.Time `json:"last_failure_at"`

	// IncidentFirstFailureAt is the first failure in the current unbroken run
	// of failures — non-null exactly when ConsecutiveFailures > 0.
	IncidentFirstFailureAt *time.Time `json:"incident_first_failure_at"`
	ConsecutiveFailures    int        `json:"consecutive_failures"`

	// LastError is the sanitized error string of the most recent failure,
	// empty unless Status is failing.
	LastError string `json:"last_error"`
}

// subsystemDef maps one subsystem to the system_events rows that mark a
// completed run — a success outcome or a failure outcome.
type subsystemDef struct {
	name         string
	successTypes []string
	failureTypes []string
}

// subsystemDefs is the tracked set (issue #427), in a deterministic order the
// API preserves.
var subsystemDefs = []subsystemDef{
	{logger.ComponentContactSync, []string{models.SysEventSyncCompleted}, []string{models.SysEventSyncFailed}},
	{logger.ComponentCalendarSync, []string{models.SysEventSyncCompleted}, []string{models.SysEventSyncFailed}},
	{logger.ComponentNotify, []string{models.SysEventNotificationSent}, []string{models.SysEventNotificationFailed}},
	{logger.ComponentBackup, []string{models.SysEventRestoreTestCompleted, models.SysEventBackupCompleted}, []string{models.SysEventBackupFailed}},
	{logger.ComponentScheduler, []string{models.SysEventJobCompleted}, []string{models.SysEventJobFailed}},
	{logger.ComponentWebhook, nil, []string{models.SysEventIntegrationFailed}},
}

// ComputeSubsystemHealth derives the LKG state of every tracked subsystem from
// system_events, in subsystemDefs order.
func ComputeSubsystemHealth(ctx context.Context, db *gorm.DB) ([]SubsystemHealth, error) {
	out := make([]SubsystemHealth, 0, len(subsystemDefs))
	for _, d := range subsystemDefs {
		h, err := computeSubsystemHealth(ctx, db, d)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

func computeSubsystemHealth(ctx context.Context, db *gorm.DB, d subsystemDef) (SubsystemHealth, error) {
	h := SubsystemHealth{Subsystem: d.name, Status: SubsystemStatusUnknown}

	lastSuccess, err := lastTerminalEvent(ctx, db, d.name, d.successTypes)
	if err != nil {
		return h, err
	}
	lastFailure, err := lastTerminalEvent(ctx, db, d.name, d.failureTypes)
	if err != nil {
		return h, err
	}

	if lastSuccess != nil {
		t := lastSuccess.OccurredAt
		h.LastSuccessAt = &t
	}
	if lastFailure != nil {
		t := lastFailure.OccurredAt
		h.LastFailureAt = &t
	}
	h.LastAttemptAt = laterTime(h.LastSuccessAt, h.LastFailureAt)

	if lastSuccess == nil && lastFailure == nil {
		return h, nil // unknown — no attempts on record
	}

	failing := lastFailure != nil && (lastSuccess == nil || afterEvent(lastFailure, lastSuccess))
	if !failing {
		h.Status = SubsystemStatusHealthy
		return h, nil
	}

	h.Status = SubsystemStatusFailing
	h.LastError = lastFailure.Error

	count, first, err := failureRunSince(ctx, db, d.name, d.failureTypes, lastSuccess)
	if err != nil {
		return h, err
	}
	h.ConsecutiveFailures = count
	if !first.IsZero() {
		f := first
		h.IncidentFirstFailureAt = &f
	}
	return h, nil
}

// lastTerminalEvent returns the newest system_events row for the component
// whose event_type is one of types, or nil when there is none (or types is
// empty — a subsystem with no success token). Find (not Take/First) so an
// empty result is not logged as a "record not found" error every call.
func lastTerminalEvent(ctx context.Context, db *gorm.DB, component string, types []string) (*models.SystemEvent, error) {
	if len(types) == 0 {
		return nil, nil
	}
	var evs []models.SystemEvent
	err := db.WithContext(ctx).
		Where("component = ? AND event_type IN ?", component, types).
		Order("occurred_at DESC, id DESC").
		Limit(1).
		Find(&evs).Error
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 {
		return nil, nil
	}
	return &evs[0], nil
}

// failureRunSince counts the unbroken run of failure events for the component
// that occurred after its last success (all failures on record when there is
// no success), and returns that run's earliest occurrence — the first failure
// of the current incident.
func failureRunSince(ctx context.Context, db *gorm.DB, component string, failureTypes []string, lastSuccess *models.SystemEvent) (int, time.Time, error) {
	where := func() *gorm.DB {
		q := db.WithContext(ctx).Model(&models.SystemEvent{}).
			Where("component = ? AND event_type IN ?", component, failureTypes)
		if lastSuccess != nil {
			q = q.Where("occurred_at > ? OR (occurred_at = ? AND id > ?)",
				lastSuccess.OccurredAt, lastSuccess.OccurredAt, lastSuccess.ID)
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

	var earliest []models.SystemEvent
	if err := where().Order("occurred_at ASC, id ASC").Limit(1).Find(&earliest).Error; err != nil {
		return 0, time.Time{}, err
	}
	if len(earliest) == 0 {
		return int(count), time.Time{}, nil
	}
	return int(count), earliest[0].OccurredAt, nil
}

// afterEvent reports whether a occurred after b, breaking an exact-timestamp
// tie by autoincrement id (the later row is the later event).
func afterEvent(a, b *models.SystemEvent) bool {
	if a.OccurredAt.Equal(b.OccurredAt) {
		return a.ID > b.ID
	}
	return a.OccurredAt.After(b.OccurredAt)
}

func laterTime(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}
