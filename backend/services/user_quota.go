package services

import (
	"fmt"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// UserQuota is the opt-in per-user resource quota (issue #950, ASVS V12.1.3).
//
// Per-request caps already bound a single call (export rows, import rows,
// graph depth, search length), but contacts, notes, relationship edges and
// attachments had no cumulative bound: one authenticated account — or a
// compromised session — could fill the operator's disk and database one
// request at a time. A zero field means "unlimited" and is the shipped
// default; the operator opts in through the PER_USER_* config variables
// (config.Config, docs/configuration-reference.md).
//
// Every check is scoped to a single user_id, never the instance, so one
// tenant's usage can never count against another's — the cross-user
// isolation the multi-user decision (issue #558) makes a release-blocking
// property. The shared count is deliberately a live-row count: GORM's
// soft-delete scope skips rows the user can still restore, so the undo window
// is not charged against the quota.
type UserQuota struct {
	Contacts          int
	Notes             int
	RelationshipEdges int
	AttachmentBytes   int64
}

// UserQuotaFromConfig projects the config surface onto a UserQuota. Keeping
// the byte conversion (MiB -> bytes) in one place means the config unit and
// the enforcement unit cannot drift.
func UserQuotaFromConfig(cfg config.Config) UserQuota {
	return UserQuota{
		Contacts:          cfg.PerUserContactLimit,
		Notes:             cfg.PerUserNoteLimit,
		RelationshipEdges: cfg.PerUserRelationshipEdgeLimit,
		AttachmentBytes:   int64(cfg.PerUserAttachmentQuotaMB) << 20,
	}
}

// Enabled reports whether any quota dimension is active. Used by tests and
// callers that want to skip the counting work entirely.
func (q UserQuota) Enabled() bool {
	return q.Contacts > 0 || q.Notes > 0 || q.RelationshipEdges > 0 || q.AttachmentBytes > 0
}

// CheckContactCreate refuses a new contact with a 507 when the user already
// holds PerUserContactLimit live contacts.
func (q UserQuota) CheckContactCreate(db *gorm.DB, userID uint) *apperrors.AppError {
	return q.checkRowQuota(db, &models.Contact{}, "PER_USER_CONTACT_LIMIT", q.Contacts, userID, "contacts")
}

// CheckNoteCreate refuses a new note with a 507 when the user already holds
// PerUserNoteLimit live notes.
func (q UserQuota) CheckNoteCreate(db *gorm.DB, userID uint) *apperrors.AppError {
	return q.checkRowQuota(db, &models.Note{}, "PER_USER_NOTE_LIMIT", q.Notes, userID, "notes")
}

// CheckRelationshipEdgeCreate refuses a new relationship edge with a 507 when
// the user already holds PerUserRelationshipEdgeLimit live edges.
func (q UserQuota) CheckRelationshipEdgeCreate(db *gorm.DB, userID uint) *apperrors.AppError {
	return q.checkRowQuota(db, &models.RelationshipEdge{}, "PER_USER_RELATIONSHIP_EDGE_LIMIT", q.RelationshipEdges, userID, "relationship edges")
}

// CheckAttachmentUpload refuses an upload of incomingBytes with a 507 when it
// would push the user's summed live attachment bytes past
// PerUserAttachmentQuotaMB. It is a preflight: the caller must run it before
// writing the file to disk, so a refused upload leaves nothing behind.
func (q UserQuota) CheckAttachmentUpload(db *gorm.DB, userID uint, incomingBytes int64) *apperrors.AppError {
	if q.AttachmentBytes <= 0 {
		return nil
	}
	if incomingBytes < 0 { // # pragma: no cover — callers pass a length; guard against a future caller
		incomingBytes = 0
	}

	var used int64
	if err := db.Model(&models.Attachment{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&used).Error; err != nil {
		return apperrors.ErrDatabase("Failed to check the attachment quota").WithError(err)
	}

	if used > q.AttachmentBytes || incomingBytes > q.AttachmentBytes-used {
		return apperrors.ErrInsufficientStorage(fmt.Sprintf(
			"per-user attachment storage quota reached (%s of %s used); delete attachments or raise PER_USER_ATTACHMENT_QUOTA_MB",
			formatMiB(used), formatMiB(q.AttachmentBytes)))
	}
	return nil
}

// checkRowQuota is the shared live-row count behind every row-shaped quota.
// It returns nil when the limit is 0 (unlimited) or the user is under it, and
// a 507 naming the config variable once the count would reach the limit.
func (q UserQuota) checkRowQuota(db *gorm.DB, model any, envVar string, limit int, userID uint, resource string) *apperrors.AppError {
	if limit <= 0 {
		return nil
	}

	var count int64
	if err := db.Model(model).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return apperrors.ErrDatabase("Failed to check the " + resource + " quota").WithError(err)
	}
	if count >= int64(limit) {
		return apperrors.ErrInsufficientStorage(fmt.Sprintf(
			"per-user %s quota reached (%d of %d); delete some %s or raise %s",
			resource, count, limit, resource, envVar))
	}
	return nil
}

// formatMiB renders a byte count as mebibytes for an operator-facing message.
func formatMiB(bytes int64) string {
	return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(1<<20))
}
