package services

import (
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

// purgeMinInterval is slightly less than the 24h cron cadence so a natural
// clock-skew or overlap doesn't cause a skipped run.
var purgeMinInterval = JobCatchupWindow(24 * time.Hour)

// purgedSoftDeleteModels is the list of soft-deleted child models the purge
// hard-deletes past retention (contacts, the parent, are purged last and
// separately). The #1310 completeness guard (purge_completeness_test.go) checks the
// real list, not a hand-kept copy.
var purgedSoftDeleteModels = []any{
	&models.Note{},
	&models.Activity{},
	&models.Reminder{},
	&models.LifeEvent{},
	&models.Preference{},
	&models.CadencePolicy{},
	&models.DataDecayPolicy{},
	&models.ConversationAgenda{},
	&models.Gift{},
	&models.ImmichConfig{},
	&models.PaperlessConfig{},
	&models.GeoPulseConfig{},
	&models.SeafileConfig{},
	&models.WebDAVConfig{},
	&models.LinkFieldType{},
	&models.CalendarSubscription{},
	&models.ContactSubscription{},
	// N7: attachment files are removed at delete time by the
	// controllers/cascade, so only the metadata row needs purging here.
	&models.Attachment{},
	// Occasions (ADR 0024/0026): user-authored, soft-deleted. Issue #1310:
	// these were missing, so a deleted contact's obligations outlived the
	// contact as orphans and soft-deleted events were kept forever.
	&models.OccasionObligation{},
	&models.OccasionEvent{},
	// Soft-deleted by their DELETE endpoints (webhook_controller.go,
	// reminder_controller.go) and, for completions, by DeleteContact — the
	// #1310 completeness guard found both unpurged. A webhook row carries
	// its signing secret; its deliveries go with it via the FK cascade.
	&models.Webhook{},
	&models.ReminderCompletion{},
}

// contactUIDCleanup names a table whose rows point at a contact by
// Contact.VCardUID (not an FK) in one or more columns, and every such column.
type contactUIDCleanup struct {
	Table string
	Cols  []string
}

// purgeContactUIDCleanups is the single list of VCardUID-keyed cleanups the
// purge runs for contacts past retention (issue #1351). The #1351 scoping
// guard (purge_user_scope_test.go) iterates THIS slice, and separately scans
// the schema for tables that carry a VCardUID-shaped column, so a new
// cleanup cannot skip the owner-scoping test.
//
// vcard_uid is unique only per (user_id) and only among LIVE contacts (the
// partial idx_contacts_vcard_uid_user), so a bare `IN (SELECT vcard_uid ...)`
// also matched another user's same-UID contact and the same user's
// re-created one — see colPredicate for the predicate that prevents that.
var purgeContactUIDCleanups = []contactUIDCleanup{
	{"occasion_event_attendees", []string{"entity_id"}},
	{"circle_members", []string{"member_vcard_uid"}},
	{"contact_tags", []string{"contact_vcard_uid"}},
	{"household_members", []string{"member_vcard_uid"}},
	{"field_values", []string{"entity_id"}},
	{"preferences", []string{"entity_id"}},
	{"conversation_agenda", []string{"entity_id"}},
	{"gifts", []string{"entity_id"}},
	{"cadence_policies", []string{"entity_id"}},
	{"data_decay_policies", []string{"entity_id"}},
	{"relationship_edges", []string{"source_id", "target_id"}},
	{"external_identities", []string{"entity_id"}},
	{"external_activities", []string{"entity_id"}},
	{"occasion_obligations", []string{"entity_id"}},
}

// colPredicate is true for a row whose <col> names a contact that is
// purge-eligible for THE ROW'S OWN USER and that has no other claim on the
// UID: no live contact of that user, and no still-in-retention soft-deleted
// one (its undo window is still open, so its rows are still someone's undo).
// Both subqueries correlate on <table>.user_id, so another user's same-UID
// contact can neither trigger nor block the delete.
func (c contactUIDCleanup) colPredicate(col string) string {
	return fmt.Sprintf(
		"(EXISTS (SELECT 1 FROM contacts d WHERE d.user_id = %[1]s.user_id AND d.vcard_uid = %[1]s.%[2]s AND d.deleted_at IS NOT NULL AND d.deleted_at < ?)"+
			" AND NOT EXISTS (SELECT 1 FROM contacts l WHERE l.user_id = %[1]s.user_id AND l.vcard_uid = %[1]s.%[2]s AND (l.deleted_at IS NULL OR l.deleted_at >= ?)))",
		c.Table, col)
}

func (c contactUIDCleanup) query() string {
	preds := make([]string, len(c.Cols))
	for i, col := range c.Cols {
		preds[i] = c.colPredicate(col)
	}
	return "DELETE FROM " + c.Table + " WHERE " + strings.Join(preds, " OR ")
}

func (c contactUIDCleanup) args(cutoff time.Time) []interface{} {
	out := make([]interface{}, 0, 2*len(c.Cols))
	for range c.Cols {
		out = append(out, cutoff, cutoff)
	}
	return out
}

// PurgeSoftDeletedRows hard-deletes soft-deleted rows older than the
// retention window. Called by both the scheduled cron job and the admin
// "purge now" trigger (T26).
//
// Purges in FK-respecting order: children before parents, so ON DELETE
// CASCADE declarations on the children can fire cleanly for the parents.
// Edge/join-shaped rows (activity_contacts, circle_members, contact_tags,
// etc.) that reference contacts are cleaned up explicitly before the
// contacts themselves are purged.
//
// A non-positive DeleteRetentionDays disables the purge (DELETED_RETENTION_DAYS=0
// is the documented "keep soft-deleted rows forever" value, .env.example). The
// guard is not merely a convenience: the cutoff is computed as now minus the
// window, so with 0 it equals now and with a negative value it lands in the
// future — either way `deleted_at < cutoff` matches the ENTIRE undo window and
// this job would hard-delete every soft-deleted row on the next run, including
// the boot-time Initial trigger in main.go. Every sibling purge (audit,
// contact-share, idempotency, job-run, system-event, webhook-delivery) carries
// the same guard for the same reason.
//
// It returns a non-nil error if any individual delete fails, joining every
// failure rather than stopping at the first (the cleanup is best-effort and
// must still attempt every table). The scheduled entry point passes that to
// releaseJobLock so a persistently failing purge is recorded as `failed` and
// job_stopped can fire, instead of every run advancing last_run_at as success.
func PurgeSoftDeletedRows(db *gorm.DB, cfg config.Config) error {
	if cfg.DeleteRetentionDays <= 0 {
		// Misconfigured to 0/negative, or deliberately disabled: never delete
		// the whole undo window.
		return nil
	}
	cutoff := Now().AddDate(0, 0, -cfg.DeleteRetentionDays)

	var errs []error

	// activity_contacts has no soft-delete. Clean up rows referencing
	// now-purged contacts as defense-in-depth.
	if err := db.Exec(
		"DELETE FROM activity_contacts WHERE contact_id IN (SELECT id FROM contacts WHERE deleted_at IS NOT NULL AND deleted_at < ?)",
		cutoff,
	).Error; err != nil {
		logger.Error().Err(err).Msg("purge: failed to delete orphaned activity_contacts")
		errs = append(errs, fmt.Errorf("purge orphaned activity_contacts: %w", err))
	}

	// occasion_event_attendees are hard-delete join rows (no deleted_at), so
	// they never age out on their own. Remove those hanging off an event
	// about to be purged BEFORE the event loop below (the FK is ON DELETE
	// CASCADE; this makes the order explicit rather than constraint-reliant).
	// Attendees naming a purged contact are in purgeContactUIDCleanups.
	for _, c := range []struct {
		query string
		desc  string
	}{
		{
			"DELETE FROM occasion_event_attendees WHERE event_id IN (SELECT id FROM occasion_events WHERE deleted_at IS NOT NULL AND deleted_at < ?)",
			"occasion_event_attendees (purged events)",
		},
	} {
		if err := db.Exec(c.query, cutoff).Error; err != nil {
			logger.Error().Err(err).Str("table", c.desc).Msg("purge: failed to clean up occasion attendees")
			errs = append(errs, fmt.Errorf("purge cleanup %s: %w", c.desc, err))
		}
	}

	// Soft-deleted children past retention. This is the durable half of
	// CLAUDE.md trap #7: the disconnect/delete handlers soft-delete
	// user-authored config (an undo window), and this job is the only thing
	// that ever hard-deletes it. A soft-deletable model that is missed here
	// lives forever — and for the integration configs that means an encrypted
	// API token/app-password row travelling into every backup. Every
	// `deleted_at`-bearing user-authored entity must be represented; the
	// integration configs (Immich/Paperless/Seafile/WebDAV), the subscriptions
	// whose URLs/credentials can embed tokens, and LinkFieldType are the
	// issue #978 omissions this list now covers.
	for _, model := range purgedSoftDeleteModels {
		if err := db.Unscoped().Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).Delete(model).Error; err != nil {
			logger.Error().Err(err).Msg("purge: failed to delete soft-deleted rows")
			errs = append(errs, fmt.Errorf("purge soft-deleted %T: %w", model, err))
		}
	}

	// Edge-shaped rows referencing contacts being purged — defense-in-depth.
	type cleanup struct {
		query string
		args  []interface{}
		desc  string
	}
	cleanups := []cleanup{
		{
			"DELETE FROM contact_sync_links WHERE contact_id IN (SELECT id FROM contacts WHERE deleted_at IS NOT NULL AND deleted_at < ?)",
			[]interface{}{cutoff}, "contact_sync_links",
		},
		{
			"DELETE FROM calendar_event_links WHERE activity_id IN (SELECT id FROM activities WHERE deleted_at IS NOT NULL AND deleted_at < ?)",
			[]interface{}{cutoff}, "calendar_event_links",
		},
	}
	// VCardUID-keyed rows: owner-scoped, see purgeContactUIDCleanups (#1351).
	for _, spec := range purgeContactUIDCleanups {
		cleanups = append(cleanups, cleanup{spec.query(), spec.args(cutoff), spec.Table})
	}
	for _, c := range cleanups {
		if err := db.Exec(c.query, c.args...).Error; err != nil {
			logger.Error().Err(err).Str("table", c.desc).Msg("purge: failed to clean up edge rows")
			errs = append(errs, fmt.Errorf("purge cleanup %s: %w", c.desc, err))
		}
	}

	// Parents: soft-deleted contacts past retention.
	result := db.Unscoped().Where(
		"deleted_at IS NOT NULL AND deleted_at < ?", cutoff,
	).Delete(&models.Contact{})
	if result.Error != nil {
		logger.Error().Err(result.Error).Msg("purge: failed to delete soft-deleted contacts")
		errs = append(errs, fmt.Errorf("purge soft-deleted contacts: %w", result.Error))
	} else if result.RowsAffected > 0 {
		logger.Info().
			Int64("rows", result.RowsAffected).
			Time("cutoff", cutoff).
			Msg("Purged soft-deleted rows")
	}

	return errors.Join(errs...)
}

// PurgeDeletedRows is the scheduled cron entry point. It acquires a job lock
// to prevent concurrent purges across multiple instances, then hard-deletes
// both soft-deleted rows past retention (PurgeSoftDeletedRows, T26) and
// expired ContactShare snapshots (PurgeExpiredContactShares, issue #574) —
// both under the same lock so the two hard-delete passes share one cadence.
//
// It reports the outcome through releaseJobLock (issue #975): a successful
// pass advances last_run_at, a failed one is recorded as `failed` and leaves
// last_run_at untouched so job_stopped can detect a purge that keeps failing.
func PurgeDeletedRows(db *gorm.DB, cfg config.Config) (err error) {
	acquired, lockErr := acquireJobLock(db, models.JobNamePurgeDeleted, purgeMinInterval)
	if lockErr != nil { // # pragma: no cover — acquireJobLock never returns an error (job_lock.go returns err == nil, nil)
		logger.Error().Err(lockErr).Msg("purge: failed to check job lock") // # pragma: no cover — see the comment above
		return lockErr                                                     // # pragma: no cover — see the comment above
	}
	if !acquired {
		return nil
	}
	defer func() {
		if relErr := releaseJobLock(db, models.JobNamePurgeDeleted, err == nil); relErr != nil {
			logger.Error().Err(relErr).Msg("purge: failed to release job lock")
			err = errors.Join(err, relErr)
		}
	}()

	return errors.Join(
		PurgeSoftDeletedRows(db, cfg),
		PurgeExpiredContactShares(db, cfg),
	)
}
