package models

import (
	"context"
	"encoding/json"
	"time"

	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// ImportRun is one persisted contact-import outcome (issue #651) — the
// per-import history the confirm endpoints never kept (the ImportResult was
// returned in the HTTP response and thrown away). An operator consults it on
// the Data settings page to answer "what did I import last week, and did
// anything fail?".
//
// User-scoped operational bookkeeping, hard-delete (no DeletedAt): a row
// belongs to the user who ran the import and is removed only by DeleteUser's
// manual cascade (CLAUDE.md backend trap #6). Immutable once written — the
// same lifecycle shape as JobRun / SystemEvent, minus the retention purge
// (an import is a rare, human-initiated action, so growth is self-bounded).
//
// Every field carries an explicit gorm:"column:..." tag: GORM's name
// derivation disagrees with hand-written migration SQL for acronyms/IDs and
// AutoMigrate-based tests cannot see it (CLAUDE.md backend trap #1). The
// Format vocabulary is mirrored in migration 000042's CHECK constraint,
// frontend/src/api/import.ts, and backend/openapi.yaml — keep them in sync
// (frontend trap #4).
type ImportRun struct {
	ID     uint   `gorm:"column:id;primarykey" json:"id"`
	UserID uint   `gorm:"column:user_id;not null;index" json:"user_id"`
	Format string `gorm:"column:format;not null" json:"format"`

	// The five counts mirror models.ImportResult, minus the error strings —
	// only how many, not which.
	TotalProcessed int `gorm:"column:total_processed;not null;default:0" json:"total_processed"`
	Created        int `gorm:"column:created;not null;default:0" json:"created"`
	Updated        int `gorm:"column:updated;not null;default:0" json:"updated"`
	Skipped        int `gorm:"column:skipped;not null;default:0" json:"skipped"`
	ErrorCount     int `gorm:"column:error_count;not null;default:0" json:"error_count"`

	// IssueCount is how many per-row issues are stored in IssuesJSON (issue
	// #2463). The list endpoint returns it so a client knows whether an
	// itemized report exists without loading the issues.
	IssueCount int `gorm:"column:issue_count;not null;default:0" json:"issue_count"`

	// IssuesJSON is the stored JSON array of ImportRowIssue: row, name, status
	// and reason only. Email and phone are never persisted (PII minimum, see
	// docs/security/pii-inventory.md). Never serialised directly: the detail
	// endpoint decodes it into ImportRunDetail.Issues.
	IssuesJSON string `gorm:"column:issues;not null;default:'[]'" json:"-"`

	// Issues is the write-side input of RecordImportRun (the confirm result's
	// issues). Not a column; the list and detail responses never read it.
	Issues []ImportRowIssue `gorm:"-" json:"-"`

	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// MaxImportRunIssues caps how many per-row issues are stored per run; the
// run's counts stay exact beyond it.
const MaxImportRunIssues = 1000

// ImportRunDetail is the GET /contacts/import/history/{id} response: the run
// plus its stored issues, always an array (never null).
type ImportRunDetail struct {
	ImportRun
	Issues []ImportRowIssue `json:"issues"`
}

// Detail decodes the stored issues. A malformed or empty column yields [].
func (r ImportRun) Detail() ImportRunDetail {
	issues := []ImportRowIssue{}
	if r.IssuesJSON != "" {
		if err := json.Unmarshal([]byte(r.IssuesJSON), &issues); err != nil || issues == nil {
			issues = []ImportRowIssue{}
		}
	}
	return ImportRunDetail{ImportRun: r, Issues: issues}
}

// storedImportIssues returns the capped, PII-minimised copy of issues that is
// persisted: email and phone blanked.
func storedImportIssues(issues []ImportRowIssue) []ImportRowIssue {
	if len(issues) > MaxImportRunIssues {
		issues = issues[:MaxImportRunIssues]
	}
	out := make([]ImportRowIssue, len(issues))
	for i, iss := range issues {
		iss.Email, iss.Phone = "", ""
		out[i] = iss
	}
	return out
}

// ImportRun format tokens. Mirrored by migration 000042's CHECK constraint
// (widened by 000046 for the source formats and 000068 for the account
// bundle), frontend/src/api/import.ts, and backend/openapi.yaml.
const (
	ImportFormatCSV         = "csv"
	ImportFormatVCF         = "vcf"
	ImportFormatJSContact   = "jscontact"
	ImportFormatRecords     = "records"
	ImportFormatMonica      = "monica"      // Monica import assistant (issue #549)
	ImportFormatMeerkat     = "meerkat"     // Meerkat import assistant (issue #550)
	ImportFormatMycorrhizal = "mycorrhizal" // account-bundle import source (issue #1260)
)

// ImportFormats is the full format vocabulary, for validation and tests.
var ImportFormats = []string{
	ImportFormatCSV,
	ImportFormatVCF,
	ImportFormatJSContact,
	ImportFormatRecords,
	ImportFormatMonica,
	ImportFormatMeerkat,
	ImportFormatMycorrhizal,
}

// RecordImportRun persists one import outcome, best-effort: it fills CreatedAt
// and never returns an error or blocks the caller — a history write must not
// be able to fail the import that just succeeded. A failed insert is logged
// and dropped. Mirrors models.RecordJobRun.
func RecordImportRun(ctx context.Context, db *gorm.DB, r ImportRun) {
	if db == nil {
		return
	}
	r.CreatedAt = time.Now().UTC()

	stored := storedImportIssues(r.Issues)
	r.IssueCount = len(stored)
	if raw, err := json.Marshal(stored); err == nil {
		r.IssuesJSON = string(raw)
	} else {
		r.IssueCount, r.IssuesJSON = 0, "[]"
	}

	if err := db.WithContext(ctx).Create(&r).Error; err != nil {
		logger.Ctx(ctx).Error().
			Err(err).
			Str(logger.FieldEvent, "import_run_write_failed").
			Uint("user_id", r.UserID).
			Str("format", r.Format).
			Msg("failed to persist import run")
	}
}
