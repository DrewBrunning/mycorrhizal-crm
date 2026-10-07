package services

import (
	"fmt"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Contact delete cascade (issue #1495, ADR 0035).
//
// Soft delete never fires SQL ON DELETE CASCADE (CLAUDE.md backend trap #6), so
// every table that references a contact must be cleaned by hand. That checklist
// used to be ~30 hand-written statements inside a controller; it is now the
// declarative ContactCascadeRegistry below, consumed by every contact-delete
// path (DeleteContact, the bulk "delete" action, and CommitContactMerge's loser
// cleanup) so the paths cannot drift. Each step records whether it soft- or
// hard-deletes (trap #7) and why; contact_delete_registry_test.go proves the
// registry covers every table in the real migrated schema that references a
// contact, and that each step's mode agrees with its model.

// CascadeMode is how a registry step treats the rows it matches.
type CascadeMode string

const (
	// CascadeSoft soft-deletes (user-authored content: undo + sync tombstone,
	// trap #7). The step's model must carry gorm.DeletedAt.
	CascadeSoft CascadeMode = "soft"
	// CascadeHard physically deletes (edge/join-shaped or system-generated
	// rows, trap #7). The step's model must NOT carry gorm.DeletedAt, or GORM
	// would soft-delete it.
	CascadeHard CascadeMode = "hard"
	// CascadeMutate is not a delete: the step rewrites a row that must outlive
	// the contact (a dangling pointer cleared, a credential revoked).
	CascadeMutate CascadeMode = "mutate"
)

// ContactCascadeTarget identifies the contact being deleted. References are
// either the legacy Contact.ID FK or the Contact.VCardUID string reference.
type ContactCascadeTarget struct {
	ID       uint
	VCardUID string
	UserID   uint
}

// CascadeStep is one dependent table's cleanup. Steps run in registry order;
// order is load-bearing where noted on the step.
type CascadeStep struct {
	// Table is the dependent table's name in the migrated schema.
	Table string
	// NewModel returns a fresh zero model for a delete step (nil for a raw
	// join-table delete, or a CascadeMutate step that supplies Apply).
	NewModel func() any
	Mode     CascadeMode
	// Match returns the WHERE clause (and args) selecting this contact's rows,
	// always scoped by user_id (trap #5). Unused by steps that set Apply, which
	// carry their own scoped statement.
	Match func(t ContactCascadeTarget) (string, []any)
	// Apply replaces the default delete for CascadeMutate steps.
	Apply func(tx *gorm.DB, t ContactCascadeTarget, now time.Time) error
	// After runs once the step's delete succeeded.
	After func(tx *gorm.DB, t ContactCascadeTarget, now time.Time) error
	// Reason is the recorded justification for the step's mode (ADR 0004/0035).
	Reason string
}

func byID(t ContactCascadeTarget) (string, []any) {
	return "contact_id = ? AND user_id = ?", []any{t.ID, t.UserID}
}

func byEntity(t ContactCascadeTarget) (string, []any) {
	return "entity_id = ? AND user_id = ?", []any{t.VCardUID, t.UserID}
}

func byMemberUID(t ContactCascadeTarget) (string, []any) {
	return "member_vcard_uid = ? AND user_id = ?", []any{t.VCardUID, t.UserID}
}

func byContactUID(t ContactCascadeTarget) (string, []any) {
	return "contact_vcard_uid = ? AND user_id = ?", []any{t.VCardUID, t.UserID}
}

// ContactCascadeRegistry returns the ordered, declarative list of every
// contact-dependent table and how a contact delete treats it. It returns a
// fresh slice each call so callers cannot mutate the shared registry.
func ContactCascadeRegistry() []CascadeStep {
	return []CascadeStep{
		// Ordering: reminders precede life_events. LifeEvent-linked reminders
		// (life_event_id) reference life events deleted further down; if the
		// order flips, those reminders survive the cascade and dangle.
		{
			Table:    "notification_deliveries",
			NewModel: func() any { return &models.NotificationDelivery{} },
			Mode:     CascadeHard,
			Match: func(t ContactCascadeTarget) (string, []any) {
				return "reminder_id IN (SELECT id FROM reminders WHERE contact_id = ? AND user_id = ?)", []any{t.ID, t.UserID}
			},
			Reason: "N9: delivery state is a hard-deleted accessory of its reminder; the reminder is soft-deleted so the FK cascade never fires. Must run before the reminders step",
		},
		{
			Table: "reminders", NewModel: func() any { return &models.Reminder{} },
			Mode: CascadeSoft, Match: byID,
			Reason: "user-authored content: soft delete gives undo and a sync tombstone; carries an ON DELETE CASCADE FK to contacts that never fires on a soft-deleted parent",
		},
		{
			Table: "reminder_completions", NewModel: func() any { return &models.ReminderCompletion{} },
			Mode: CascadeSoft, Match: byID,
			Reason: "user-authored history: soft delete; FK cascade to contacts never fires",
		},
		{
			Table: "notes", NewModel: func() any { return &models.Note{} },
			Mode: CascadeSoft, Match: byID,
			// Bulk soft deletes skip Note.AfterDelete (it fires on a zero-value
			// model), so advance updated_at on the just-tombstoned notes or a
			// T17 change-feed cursor stored before this delete misses them.
			After: func(tx *gorm.DB, t ContactCascadeTarget, now time.Time) error {
				return tx.Model(&models.Note{}).Unscoped().
					Where("contact_id = ? AND user_id = ? AND deleted_at IS NOT NULL", t.ID, t.UserID).
					UpdateColumn("updated_at", now).Error
			},
			Reason: "user-authored content: soft delete; FK cascade to contacts never fires",
		},
		{
			Table: "activity_contacts", Mode: CascadeHard,
			Match: func(t ContactCascadeTarget) (string, []any) {
				return "contact_id = ? AND activity_id IN (SELECT id FROM activities WHERE user_id = ?)", []any{t.ID, t.UserID}
			},
			Reason: "many-to-many join table without a model; the activity itself survives other attendees",
		},
		{
			Table: "relationship_edges", NewModel: func() any { return &models.RelationshipEdge{} },
			Mode: CascadeHard,
			Match: func(t ContactCascadeTarget) (string, []any) {
				return "(source_id = ? OR target_id = ?) AND user_id = ?", []any{t.VCardUID, t.VCardUID, t.UserID}
			},
			Reason: "edge-shaped (either endpoint): natural-key unique index, trap #7",
		},
		{
			Table: "household_members", NewModel: func() any { return &models.HouseholdMember{} },
			Mode: CascadeHard, Match: byMemberUID,
			Reason: "join row; the household container survives for its other members",
		},
		{
			Table: "circle_members", NewModel: func() any { return &models.CircleMember{} },
			Mode: CascadeHard, Match: byMemberUID,
			Reason: "join row; the circle container survives for its other members",
		},
		{
			Table: "contact_tags", NewModel: func() any { return &models.ContactTag{} },
			Mode: CascadeHard, Match: byContactUID,
			Reason: "join row; the tag container survives for its other contacts",
		},
		{
			Table: "life_events", NewModel: func() any { return &models.LifeEvent{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "user-authored content: soft delete",
		},
		{
			Table: "life_event_suggestion_resolutions", NewModel: func() any { return &models.LifeEventSuggestionResolution{} },
			Mode: CascadeHard, Match: byEntity,
			Reason: "ADR 0023: system-generated, join-shaped resolution memory; nothing to remember a decision about once the contact is gone",
		},
		{
			Table: "preferences", NewModel: func() any { return &models.Preference{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "user-authored content: soft delete",
		},
		{
			Table: "field_values", NewModel: func() any { return &models.FieldValue{} },
			Mode: CascadeHard, Match: byEntity,
			Reason: "join-shaped (field definition x entity): trap #7",
		},
		{
			Table: "reach_out_suggestions", NewModel: func() any { return &models.ReachOutSuggestion{} },
			Mode: CascadeHard, Match: byContactUID,
			Reason: "issue #177: system-generated pending suggestion; the companion reminder is removed by the reminders step",
		},
		{
			Table: "conversation_agenda", NewModel: func() any { return &models.ConversationAgenda{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "T21: user-authored content: soft delete",
		},
		{
			Table: "gifts", NewModel: func() any { return &models.Gift{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "T20b: user-authored content: soft delete",
		},
		{
			Table: "cadence_policies", NewModel: func() any { return &models.CadencePolicy{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "T19: user-authored content: soft delete",
		},
		{
			Table: "data_decay_policies", NewModel: func() any { return &models.DataDecayPolicy{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "issue #352, ADR 0027: user-authored content: soft delete",
		},
		{
			Table: "occasion_obligations", NewModel: func() any { return &models.OccasionObligation{} },
			Mode: CascadeSoft, Match: byEntity,
			Reason: "ADR 0024, issue #1222: user-authored content: soft delete",
		},
		{
			Table: "occasion_event_attendees", NewModel: func() any { return &models.OccasionEventAttendee{} },
			Mode: CascadeHard, Match: byEntity,
			Reason: "ADR 0026, issue #1228: join row; the event survives because another attendee may still be invited",
		},
		{
			Table: "contact_sync_links", NewModel: func() any { return &models.ContactSyncLink{} },
			Mode: CascadeHard, Match: byID,
			Reason: "genuine Contact.ID FK; join-shaped sync bookkeeping, hard delete per ContactSyncLink's doc",
		},
		{
			Table: "contact_sync_conflicts", NewModel: func() any { return &models.ContactSyncConflict{} },
			Mode: CascadeHard, Match: byID,
			Reason: "issue #395: system-generated; nothing left to review once the contact is gone",
		},
		{
			Table: "external_identities", NewModel: func() any { return &models.ExternalIdentity{} },
			Mode: CascadeHard, Match: byEntity,
			Reason: "T14: integration link keyed by VCardUID; hard delete",
		},
		{
			Table: "external_activities", NewModel: func() any { return &models.ExternalActivity{} },
			Mode: CascadeHard, Match: byEntity,
			Reason: "T14: enrichment event keyed by VCardUID; hard delete",
		},
		{
			Table: "attachments", NewModel: func() any { return &models.Attachment{} },
			Mode: CascadeSoft, Match: byContactUID,
			Reason: "N7: user-authored content: soft delete; on-disk files are removed by the caller after the transaction commits",
		},
		{
			Table: "users", Mode: CascadeMutate,
			Apply: func(tx *gorm.DB, t ContactCascadeTarget, _ time.Time) error {
				return tx.Model(&models.User{}).Where("id = ? AND self_contact_vcard_uid = ?", t.UserID, t.VCardUID).
					Update("self_contact_vcard_uid", nil).Error
			},
			Reason: "T90: the user's \"Me\" pointer must not dangle on a soft-deleted row; a no-op in the merge path where the pointer was already re-pointed to the keeper",
		},
		{
			Table: "feeds", Mode: CascadeMutate,
			Apply: func(tx *gorm.DB, t ContactCascadeTarget, now time.Time) error {
				return tx.Model(&models.Feed{}).
					Where("user_id = ? AND kind = ? AND entity_id = ? AND revoked_at IS NULL", t.UserID, models.FeedKindContact, t.VCardUID).
					Update("revoked_at", now).Error
			},
			Reason: "issue #382, ADR 0030 decision 6: revoked, not deleted -- the row stays for the audit trail and undo must not silently re-arm an export; account-aggregate feeds untouched",
		},
		{
			Table: "dismissed_duplicate_pairs", NewModel: func() any { return &models.DismissedDuplicatePair{} },
			Mode: CascadeHard,
			Match: func(t ContactCascadeTarget) (string, []any) {
				return "(uid_low = ? OR uid_high = ?) AND user_id = ?", []any{t.VCardUID, t.VCardUID, t.UserID}
			},
			Reason: "T93: join-shaped dismissal naming this contact on either side of the ordered uid pair",
		},
	}
}

// DeleteContactAssociations removes every row that references contact -- the
// full cascade except the contact row itself -- by walking
// ContactCascadeRegistry in order. It must run inside an existing transaction
// (tx). now stamps the notes tombstone-touch and the feed revocation.
func DeleteContactAssociations(tx *gorm.DB, contact models.Contact, userID uint, now time.Time) error {
	target := ContactCascadeTarget{ID: contact.ID, VCardUID: contact.VCardUID, UserID: userID}
	for _, step := range ContactCascadeRegistry() {
		if err := runCascadeStep(tx, step, target, now); err != nil {
			return err
		}
	}
	return nil
}

// deleteCascadeRows is the default step body: delete the matched rows through
// the step's model (so soft delete and model hooks behave exactly as a direct
// tx.Delete would), or by raw SQL for a model-less join table.
func deleteCascadeRows(tx *gorm.DB, step CascadeStep, target ContactCascadeTarget) error {
	query, args := step.Match(target)
	if step.NewModel == nil {
		return tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s", step.Table, query), args...).Error
	}
	return tx.Where(query, args...).Delete(step.NewModel()).Error
}

func runCascadeStep(tx *gorm.DB, step CascadeStep, target ContactCascadeTarget, now time.Time) error {
	var err error
	if step.Apply != nil {
		err = step.Apply(tx, target, now)
	} else {
		err = deleteCascadeRows(tx, step, target)
	}
	if err != nil {
		return err
	}
	if step.After != nil {
		return step.After(tx, target, now)
	}
	return nil
}

// DeleteContactInTx runs the whole contact delete inside tx: the registry
// cascade, then the contact row itself (soft delete).
func DeleteContactInTx(tx *gorm.DB, contact models.Contact, userID uint, now time.Time) error {
	if err := DeleteContactAssociations(tx, contact, userID, now); err != nil {
		return err
	}
	return tx.Delete(&contact).Error
}

// DeleteContact deletes contact and everything that references it in a single
// transaction, so the cascade either completes or rolls back whole (N5 trap).
// Callers handle on-disk cleanup (photo, attachment files) after it returns.
func DeleteContact(db *gorm.DB, contact models.Contact, userID uint, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return DeleteContactInTx(tx, contact, userID, now)
	})
}
