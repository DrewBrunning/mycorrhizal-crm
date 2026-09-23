package controllers

import (
	"os"

	"mycorrhizal/attachments"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// deleteUserCascade removes every row owned by userID across the tables this
// project's delete-cascade checklist enumerates (backend trap #6). It is
// shared by the admin-only DeleteUser and the self-service DeleteOwnAccount,
// which differ only in their guards, audit handling, and response shape —
// the cascade mechanics themselves are the same regardless of who triggers
// them. Callers must run this inside a transaction and pre-load the deleted
// user's attachment stored names beforehand (deleteUserAttachmentFiles removes
// the actual files afterward, once the transaction has committed).
//
// SkipHooks is deliberate, extending the reasoning DeleteOwnAccount's
// "issue #972 decision 3" comment already applies to the top-level
// self-delete audit event to every per-entity CRUD audit hook (Contact,
// Note, Activity, Circle, Tag, Household, Reminder, ...) these deletes would
// otherwise fire: audit_events.user_id is a NOT NULL FK to users.id with ON
// DELETE CASCADE, and this transaction always ends by hard-deleting the user
// row, so any per-entity delete-audit row would either be cascade-deleted
// the instant that happens (if it landed earlier in this same transaction)
// or fail its FK and be dropped by the fire-and-forget audit logger (it
// always writes through a separate DB session — see models/audit.go's
// auditLogger doc comment — so ordering against this transaction's commit is
// never guaranteed). Recording it is pointless either way, and without
// SkipHooks it isn't even harmless: several of these hooks (Circle, Tag,
// Household, Reminder) have no soft-delete guard, so they fire once per bulk
// Delete call here regardless of how many rows actually matched — on an
// account with none of that entity type, they still fire with a zero-value
// model (empty entity ID, user_id 0), which can never satisfy the FK and
// logs a spurious warning on every single account deletion. SkipHooks
// removes the futile write (and its log noise) at the source instead of
// papering over it downstream; it is safe here because no model in this
// cascade has a BeforeDelete hook, and the only BeforeSave use above
// (promoting another user to admin in DeleteOwnAccount) runs before this
// call, not through it.
//
// Every per-table `return err` below is deliberately `# pragma: no cover`
// (docs/development/coverage.md's override path): each one only fires on a
// genuine SQLite/disk-level failure, never on a bad request — every FK from
// another table to `users` is either swept here or carries `ON DELETE
// CASCADE`, and controllers/delete_cascade_coverage_test.go proves that
// structurally, so a mid-cascade failure can never come from an orphaned
// reference this cascade forgot to clean up. One representative failure
// (TestDeleteUser_ReportsTransactionFailure /
// TestDeleteOwnAccount_CascadeTransactionFails, both via a dropped table)
// already proves the pattern — abort the whole transaction, roll back,
// 500 — generically; fault-injecting each of the ~50 other tables
// individually would just re-prove the same Go idiom fifty times over.
func deleteUserCascade(tx *gorm.DB, userID uint) error {
	tx = tx.Session(&gorm.Session{SkipHooks: true})

	// Delete attachments (N7 — hard: account gone, no tombstoning needed).
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Attachment{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete reminders (hard — user account gone, no tombstoning needed).
	// N9: their notification delivery state is hard-deleted first (the
	// reminder rows are Unscoped()-removed here, so the FK cascade would
	// cover it — this explicit pass keeps the manual-cascade checklist
	// complete rather than relying on the constraint).
	if err := tx.Where("reminder_id IN (SELECT id FROM reminders WHERE user_id = ?)", userID).Delete(&models.NotificationDelivery{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Reminder{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete contact shares where the user is either party (hard —
	// payload could carry data about the other party's contacts; no
	// tombstoning once the account is gone). ContactShare has no soft
	// delete of its own, so no Unscoped() needed here.
	if err := tx.Where("from_user_id = ? OR to_user_id = ?", userID, userID).Delete(&models.ContactShare{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete notes (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Note{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete activity_contacts associations (many-to-many)
	if err := tx.Exec("DELETE FROM activity_contacts WHERE activity_id IN (SELECT id FROM activities WHERE user_id = ?)", userID).Error; err != nil {
		return err
	}

	// Delete activities (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Activity{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete webhook deliveries, then webhooks (child before parent;
	// WebhookDelivery has no direct UserID, only WebhookID)
	if err := tx.Exec("DELETE FROM webhook_deliveries WHERE webhook_id IN (SELECT id FROM webhooks WHERE user_id = ?)", userID).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.Webhook{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete CardDAV contact sync links, then subscriptions (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.ContactSyncLink{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.ContactSubscription{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete household memberships, then households (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.HouseholdMember{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.Household{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete circle memberships, then circles (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.CircleMember{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.Circle{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete contact tags, then tags (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.ContactTag{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.Tag{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete custom field values, then field definitions (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.FieldValue{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.FieldDefinition{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete CardDAV sync token
	if err := tx.Where("user_id = ?", userID).Delete(&models.CardDAVSync{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete API tokens
	if err := tx.Where("user_id = ?", userID).Delete(&models.ApiToken{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete device grants (issue #722) — the biometric-login credentials
	// die with the account, exactly like API tokens.
	if err := tx.Where("user_id = ?", userID).Delete(&models.DeviceGrant{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete session rows (issue #866) — hard delete; the FK is ON DELETE
	// CASCADE but the manual enumeration is the convention (backend trap #6).
	if err := tx.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete reminder completions
	if err := tx.Where("user_id = ?", userID).Delete(&models.ReminderCompletion{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete calendar event links, then calendar subscriptions (child before parent)
	if err := tx.Where("user_id = ?", userID).Delete(&models.CalendarEventLink{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.CalendarSubscription{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete relationship-graph edges
	if err := tx.Where("user_id = ?", userID).Delete(&models.RelationshipEdge{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete life events (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.LifeEvent{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete life-event-suggestion resolution memory (ADR 0023, hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.LifeEventSuggestionResolution{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete preferences (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Preference{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete cadence policies (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.CadencePolicy{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete conversation agenda items (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ConversationAgenda{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete gift records (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Gift{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete external integration links and enrichment events (T14 —
	// hard delete, edge/join-shaped)
	if err := tx.Where("user_id = ?", userID).Delete(&models.ExternalIdentity{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.ExternalActivity{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete reach-out suggestions and the detection watermark (issue
	// #177 — hard delete, system-generated/cursor-shaped)
	if err := tx.Where("user_id = ?", userID).Delete(&models.ReachOutSuggestion{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.ReachOutCursor{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete CardDAV sync conflicts (issue #395 — hard delete,
	// system-generated; nothing left to review once the account is gone)
	if err := tx.Where("user_id = ?", userID).Delete(&models.ContactSyncConflict{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete the user's Immich connection config (T15/T16)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ImmichConfig{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete the user's file-integration connection configs (P2a/P2b/P2c —
	// Paperless-ngx, Seafile, Nextcloud/ownCloud WebDAV)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.PaperlessConfig{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.SeafileConfig{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.WebDAVConfig{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete the user's notification channel config and push device
	// subscriptions (N9 — hard: account gone, no tombstoning needed)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.NotificationConfig{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.PushSubscription{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Mobile push device registrations (M2 — account gone, no tombstoning
	// needed; matches PushSubscription above)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.DeviceRegistration{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete link field types (hard — user account gone, no
	// tombstoning needed; matches the other DeletedAt-bearing entities
	// above, e.g. CadencePolicy/Preference/LifeEvent)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.LinkFieldType{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// T93: duplicate-pair dismissal memory (hard, edge/join-shaped — account
	// gone, no tombstoning needed)
	if err := tx.Where("user_id = ?", userID).Delete(&models.DismissedDuplicatePair{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// N8: hashed 2FA recovery codes (hard, join-shaped — a code is its
	// hash; the FK cascade on recovery_codes.user_id would cover it, but
	// the manual-cascade checklist stays complete rather than relying on
	// the constraint)
	if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete import run history (issue #651 — hard, user-scoped
	// operational bookkeeping; the account is gone, nothing to keep).
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ImportRun{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete idempotency-key replay records (issue #459, CON-04 — hard,
	// user-scoped, transient dedup bookkeeping).
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.IdempotencyKey{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete contacts (hard)
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Contact{}).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	// Delete the user row itself (hard — accounts must be re-registerable, T26)
	if err := tx.Unscoped().Delete(&models.User{}, userID).Error; err != nil {
		return err // # pragma: no cover — DB/disk failure only (schema/FK integrity is proven elsewhere); see file doc comment
	}

	return nil
}

// deleteUserAttachmentFiles removes attachment files owned by a deleted user
// from disk (N7). storedNames were captured before the transaction deleted the
// rows.
func deleteUserAttachmentFiles(c *gin.Context, storedNames []string) {
	dir := currentConfig(c).AttachmentsDir
	if dir == "" || len(storedNames) == 0 {
		return
	}
	log := logger.FromContext(c)
	for _, name := range storedNames {
		path, err := attachments.StoredPath(dir, name)
		if err != nil {
			log.Warn().Err(err).Str("stored_name", name).Msg("Failed to resolve attachment path for user cleanup")
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", path).Msg("Failed to delete user attachment file")
		}
	}
}
