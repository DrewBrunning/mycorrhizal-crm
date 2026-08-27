package models

import (
	"encoding/json"
	"time"
)

// SyncHealthFields is the per-subscription last-known-good state (issue #390),
// embedded identically in ContactSubscription and CalendarSubscription. The
// LastSync* fields on each model are the current pass/fail bit; these are the
// history an operator needs to tell a transient blip from a standing outage —
// when the subsystem last actually worked, how many runs have failed in a
// row, and when the current run of failures started.
//
// AdvanceForRun is the single writer: both CalendarSyncService.SyncSubscription
// and ContactSyncService.SyncSubscription call it from their one post-run
// bookkeeping block. The cross-subsystem generalization of this shape is
// issue #427.
//
// Explicit gorm:"column:..." tags on every field — GORM's name derivation
// disagrees with the hand-written migration SQL for exactly this kind of name
// (CLAUDE.md backend trap 1). Anonymous-embedded, so the columns land on the
// host table (same mechanism as gorm.Model).
type SyncHealthFields struct {
	// LastAttemptAt is set on every run, success or failure. (LastSyncedAt on
	// the host model has meant the same thing; it is kept untouched for the
	// existing JSON contract.)
	LastAttemptAt *time.Time `gorm:"column:last_attempt_at" json:"last_attempt_at"`
	// LastSuccessAt / LastFailureAt are the most recent run of each outcome.
	LastSuccessAt *time.Time `gorm:"column:last_success_at" json:"last_success_at"`
	LastFailureAt *time.Time `gorm:"column:last_failure_at" json:"last_failure_at"`
	// ConsecutiveFailures counts runs that have failed in a row; any success
	// resets it to 0.
	ConsecutiveFailures int `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`
	// IncidentFirstFailureAt is the timestamp of the first failure in the
	// current unbroken run of failures — NULL exactly when
	// ConsecutiveFailures is 0.
	IncidentFirstFailureAt *time.Time `gorm:"column:incident_first_failure_at" json:"incident_first_failure_at"`
	// LastRunDurationMS is the wall-clock duration of the last run.
	LastRunDurationMS *int64 `gorm:"column:last_run_duration_ms" json:"last_run_duration_ms"`
	// LastRunStats is the JSON-encoded per-run counter struct
	// (services.ContactSyncStats / services.CalendarSyncStats). Kept off the
	// model's own JSON — the response DTO re-emits it as a nested object.
	LastRunStats string `gorm:"column:last_run_stats;not null;default:''" json:"-"`
}

// AdvanceForRun folds one completed sync run into the health state, mutating
// the receiver, and returns the column→value map to persist alongside the
// caller's own last_sync_status / last_sync_error writes. now is the run's
// completion time, dur its wall-clock duration, statsJSON the JSON-encoded
// counter struct, and runErr its outcome (nil means success).
func (h *SyncHealthFields) AdvanceForRun(now time.Time, dur time.Duration, statsJSON string, runErr error) map[string]interface{} {
	ms := dur.Milliseconds()
	h.LastAttemptAt = &now
	h.LastRunDurationMS = &ms
	h.LastRunStats = statsJSON

	updates := map[string]interface{}{
		"last_attempt_at":      h.LastAttemptAt,
		"last_run_duration_ms": h.LastRunDurationMS,
		"last_run_stats":       h.LastRunStats,
	}

	if runErr != nil {
		h.LastFailureAt = &now
		h.ConsecutiveFailures++
		if h.IncidentFirstFailureAt == nil {
			h.IncidentFirstFailureAt = &now
		}
		updates["last_failure_at"] = h.LastFailureAt
		updates["consecutive_failures"] = h.ConsecutiveFailures
		updates["incident_first_failure_at"] = h.IncidentFirstFailureAt
		return updates
	}

	h.LastSuccessAt = &now
	h.ConsecutiveFailures = 0
	h.IncidentFirstFailureAt = nil
	updates["last_success_at"] = h.LastSuccessAt
	updates["consecutive_failures"] = 0
	updates["incident_first_failure_at"] = nil
	return updates
}

// SyncHealthResponse is the wire form of SyncHealthFields (issue #390),
// embedded in both subscription response DTOs. NewSyncHealthResponse builds
// it, turning the stored LastRunStats string into a real nested object ({}
// when empty, never null — CLAUDE.md frontend trap 8).
type SyncHealthResponse struct {
	LastAttemptAt          *time.Time      `json:"last_attempt_at"`
	LastSuccessAt          *time.Time      `json:"last_success_at"`
	LastFailureAt          *time.Time      `json:"last_failure_at"`
	ConsecutiveFailures    int             `json:"consecutive_failures"`
	IncidentFirstFailureAt *time.Time      `json:"incident_first_failure_at"`
	LastRunDurationMS      *int64          `json:"last_run_duration_ms"`
	LastRunStats           json.RawMessage `json:"last_run_stats"`
}

// NewSyncHealthResponse maps the embedded model fields to their wire form.
func NewSyncHealthResponse(h SyncHealthFields) SyncHealthResponse {
	stats := json.RawMessage(h.LastRunStats)
	if !json.Valid(stats) {
		stats = json.RawMessage("{}")
	}
	return SyncHealthResponse{
		LastAttemptAt:          h.LastAttemptAt,
		LastSuccessAt:          h.LastSuccessAt,
		LastFailureAt:          h.LastFailureAt,
		ConsecutiveFailures:    h.ConsecutiveFailures,
		IncidentFirstFailureAt: h.IncidentFirstFailureAt,
		LastRunDurationMS:      h.LastRunDurationMS,
		LastRunStats:           stats,
	}
}
