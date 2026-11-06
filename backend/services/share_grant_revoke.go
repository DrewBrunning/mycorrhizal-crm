package services

import (
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// RevokeShareGrantsForEntity ends every live (pending, active or blocked)
// standing-share grant that ownerID has on one entity, setting status
// `revoked` with reason `entity_deleted` (ADR 0042 decision 10, issue #2289).
// History rows (declined, revoked, expired) are left exactly as they are.
//
// It is the single helper behind "deleting the entity ends its grants": the
// contact delete cascade calls it from a CascadeMutate registry step (single
// delete, bulk delete and the merge loser all walk that registry), and the
// note, activity and circle delete paths call it directly. Callers must run it
// in the same transaction as the delete so the two cannot diverge. entityID is
// the entity's stable identity (Contact.VCardUID, or the note/activity/circle
// UUID), never an integer PK (decision 4).
//
// The match is scoped by from_user_id (trap #5): a grant is the owner's, and a
// foreign owner holding the same entity_id string is never touched. It writes
// through tx.Table so the explicit column list is the allowlist, and advances
// revision (ADR 0006) and updated_at so the change is observable. It returns
// the number of grants revoked.
func RevokeShareGrantsForEntity(tx *gorm.DB, ownerID uint, entityType, entityID string, now time.Time) (int64, error) {
	res := tx.Table("share_grants").
		Where("from_user_id = ? AND entity_type = ? AND entity_id = ? AND status IN ?",
			ownerID, entityType, entityID, models.ShareGrantLiveStatuses).
		UpdateColumns(map[string]any{
			"status":         models.ShareGrantStatusRevoked,
			"revoked_reason": models.ShareGrantReasonEntityDeleted,
			"revoked_at":     now,
			"updated_at":     now,
			"revision":       gorm.Expr("revision + 1"),
		})
	return res.RowsAffected, res.Error
}
