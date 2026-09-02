package models

import "time"

// Operational self-check outcome statuses, matching the CHECK constraint in
// 000037_operational_check_results.up.sql.
const (
	// OpCheckStatusOK — the check ran and passed.
	OpCheckStatusOK = "ok"
	// OpCheckStatusFailed — the check ran and diagnosed a real problem
	// (corruption found, restored row counts don't match).
	OpCheckStatusFailed = "failed"
	// OpCheckStatusError — the check itself could not complete (an error
	// running it), which is distinct from a clean "failed" verdict.
	OpCheckStatusError = "error"
)

// Operational self-check names that are not themselves scheduled jobs. Job-
// backed checks reuse their JobName* constant as the CheckName; these do not.
const (
	// CheckNameDataIntegrity is the application-invariant pass (DB-01, issue
	// #460): relationships referencing missing/soft-deleted contacts, orphaned
	// join rows, dangling external references, malformed canonical records,
	// derived-index divergence. It runs alongside the storage-level
	// JobNameDBIntegrityCheck (PRAGMA) pass, on the same schedule and config
	// gate, but records its outcome under this distinct name so an operator
	// can tell "the disk is failing" from "the data has a logical hole".
	CheckNameDataIntegrity = "data_integrity_check"
)

// OperationalCheckResult holds the most recent outcome of one named
// operational self-check (issue #421). One row per CheckName, upserted by
// services.RecordOperationalCheckResult after every scheduled run of the
// DB-integrity-check (issue #273) and restore-drill (issue #275) jobs. The
// deep GET /health endpoint reads it.
//
// Not user-authored content: hard state, no DeletedAt (CLAUDE.md backend trap
// #7). Explicit gorm column tags because "check_name"/"checked_at" are exactly
// the kind of names GORM's derivation gets subtly wrong (trap #1).
type OperationalCheckResult struct {
	// CheckName is the stable identifier of the check — reuse the JobName*
	// constants (JobNameDBIntegrityCheck, JobNameRestoreDrill).
	CheckName string `gorm:"column:check_name;primaryKey" json:"check_name"`
	// Status is one of OpCheckStatus{OK,Failed,Error}.
	Status string `gorm:"column:status;not null" json:"status"`
	// Detail is a short human-readable elaboration (mismatch list, error
	// string, integrity_check problem lines). Empty when Status is ok.
	Detail string `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	// CheckedAt is when this outcome was recorded.
	CheckedAt time.Time `gorm:"column:checked_at;not null" json:"checked_at"`
	CreatedAt time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName pins the table name so it never drifts from the migration.
func (OperationalCheckResult) TableName() string {
	return "operational_check_results"
}
