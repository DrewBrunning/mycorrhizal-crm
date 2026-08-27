package models

import (
	"context"
	"time"

	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// SystemEvent is one persisted operational event — application start/stop, a
// migration, a scheduled job run, a sync run, a notification dispatch, a
// backup/restore drill. It is the chronological "what happened to the system"
// record an admin consults first when something is wrong, and it survives a
// restart (issue #424).
//
// Distinct from AuditEvent (who changed user data) and JobExecution (lock +
// last-run only). System-generated, hard-delete (no DeletedAt): rows are
// removed only by PurgeExpiredSystemEvents (SYSTEM_EVENT_RETENTION_DAYS),
// mirroring AuditEvent's lifecycle — CLAUDE.md backend trap 7.
//
// Every field carries an explicit gorm:"column:..." tag: GORM's name
// derivation disagrees with hand-written migration SQL for acronyms/IDs and
// AutoMigrate-based tests cannot see it (CLAUDE.md backend trap 1). The
// event-type / severity / result vocabularies are mirrored in migration
// 000037's CHECK constraint and in frontend/src/api/systemEvents.ts — keep
// the three in sync (frontend trap 4).
type SystemEvent struct {
	ID        uint      `gorm:"column:id;primarykey" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`

	// OccurredAt is when the event actually happened (defaulted to now by
	// RecordSystemEvent); CreatedAt is when the row was written. They differ
	// only if an emitter backdates an event.
	OccurredAt time.Time `gorm:"column:occurred_at;not null" json:"occurred_at"`

	EventType string `gorm:"column:event_type;not null;index" json:"event_type"`
	Severity  string `gorm:"column:severity;not null;default:'info'" json:"severity"`
	Component string `gorm:"column:component;not null;default:''" json:"component"`
	Operation string `gorm:"column:operation;not null;default:''" json:"operation"`

	// DurationMS is the wall-clock duration of the operation in milliseconds,
	// nil when not applicable (an instantaneous event).
	DurationMS *int64 `gorm:"column:duration_ms" json:"duration_ms,omitempty"`

	// Result is success | failure | skipped, nil for events that are neither
	// (application_started).
	Result *string `gorm:"column:result" json:"result,omitempty"`

	// CorrelationID ties this event to the chain of work it belongs to — an
	// HTTP request ("<request id>") or a scheduled run ("job:<name>:<uuid>").
	// RecordSystemEvent fills it from the context when the caller leaves it
	// empty.
	CorrelationID string `gorm:"column:correlation_id;not null;default:'';index" json:"correlation_id"`

	// Error is a sanitized, length-capped error string on the failure path.
	Error string `gorm:"column:error;not null;default:''" json:"error,omitempty"`

	// Detail is a bounded, low-cardinality string for extra context — counts,
	// a subsystem name, a from/to version. Never a contact ID or a raw URL
	// (issue #424 non-goal).
	Detail string `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`

	// UserID is the acting user when the event is attributable to one
	// (a user-triggered sync), nil for purely background work.
	UserID *uint `gorm:"column:user_id" json:"user_id,omitempty"`
}

// SystemEvent type tokens. Mirrored by migration 000037's CHECK constraint
// and frontend/src/api/systemEvents.ts.
const (
	SysEventApplicationStarted = "application_started"
	SysEventApplicationStopped = "application_stopped"

	SysEventMigrationStarted   = "migration_started"
	SysEventMigrationCompleted = "migration_completed"
	SysEventMigrationFailed    = "migration_failed"

	SysEventJobStarted   = "job_started"
	SysEventJobCompleted = "job_completed"
	SysEventJobFailed    = "job_failed"

	SysEventSyncStarted   = "sync_started"
	SysEventSyncCompleted = "sync_completed"
	SysEventSyncFailed    = "sync_failed"

	SysEventNotificationSent   = "notification_sent"
	SysEventNotificationFailed = "notification_failed"

	SysEventBackupCompleted      = "backup_completed"
	SysEventBackupFailed         = "backup_failed"
	SysEventRestoreTestCompleted = "restore_test_completed"

	SysEventIntegrationFailed = "integration_failed"
)

// SystemEventTypes is the full token set, for validation and tests. Keep in
// sync with the consts above and migration 000037.
var SystemEventTypes = []string{
	SysEventApplicationStarted, SysEventApplicationStopped,
	SysEventMigrationStarted, SysEventMigrationCompleted, SysEventMigrationFailed,
	SysEventJobStarted, SysEventJobCompleted, SysEventJobFailed,
	SysEventSyncStarted, SysEventSyncCompleted, SysEventSyncFailed,
	SysEventNotificationSent, SysEventNotificationFailed,
	SysEventBackupCompleted, SysEventBackupFailed, SysEventRestoreTestCompleted,
	SysEventIntegrationFailed,
}

// maxSystemEventFieldLen caps the persisted Error / Detail strings.
const maxSystemEventFieldLen = 1024

// RecordSystemEvent persists one operational event, best-effort: it fills
// sensible defaults, sanitizes the free-text fields, and never returns an
// error or blocks the caller's real work — a diagnostic write must not be
// able to fail an operation. A failed insert is logged and dropped.
//
// The caller passes the operation's context so the correlation ID (and, where
// relevant, the acting user) rides along without being repeated at every call
// site.
func RecordSystemEvent(ctx context.Context, db *gorm.DB, ev SystemEvent) {
	if db == nil {
		return
	}

	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.CorrelationID == "" {
		ev.CorrelationID = logger.CorrelationID(ctx)
	}
	if ev.Severity == "" {
		ev.Severity = severityForResult(ev.Result)
	}
	ev.Error = truncateRunes(logger.SanitizeLogField(ev.Error), maxSystemEventFieldLen)
	ev.Detail = truncateRunes(logger.SanitizeLogField(ev.Detail), maxSystemEventFieldLen)

	// Own timestamp; the row is never updated after insert.
	ev.CreatedAt = time.Now().UTC()

	if err := db.WithContext(ctx).Create(&ev).Error; err != nil {
		logger.Ctx(ctx).Error().
			Err(err).
			Str(logger.FieldEvent, "system_event_write_failed").
			Str("system_event_type", ev.EventType).
			Msg("failed to persist system event")
	}
}

func severityForResult(result *string) string {
	if result != nil && *result == logger.ResultFailure {
		return logger.SeverityError
	}
	return logger.SeverityInfo
}

// Result helpers so call sites don't juggle *string literals.
func SysResult(v string) *string { return &v }

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
