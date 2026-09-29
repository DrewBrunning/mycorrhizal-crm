package services

// Purge completeness guard (issue #1310), in the style of the merge-repoint
// guard (controllers/contact_merge_repoint_coverage_test.go).
//
// PurgeSoftDeletedRows hard-deletes soft-deleted rows for a hand-maintained
// model list, and nothing compared that list to the schema — so occasion
// obligations/events, webhooks and reminder completions were soft-deleted by
// their handlers and then lived forever (a deleted contact's obligations even
// outlived the contact as orphans).
//
// The guard is driven by the migrated schema: every table with a `deleted_at`
// column must be either purged (its model is in purgedSoftDeleteModels, or it
// is `contacts`, purged last as the parent) or listed in purgeExcluded with a
// reason. An unclassified table FAILS, as does a stale exclusion.

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// purgeExcluded lists tables with a deleted_at column that the soft-delete
// purge deliberately does NOT hard-delete. Every entry needs a reason.
var purgeExcluded = map[string]string{
	"users":                "DeleteUser/DeleteOwnAccount hard-delete via Unscoped (trap #7's one deliberate exception); the row is never left soft-deleted",
	"api_tokens":           "revoked with revoked_at (api_token_service.go), never soft-deleted by a handler; DeleteUser hard-deletes them",
	"device_grants":        "revoked with revoked_at (device_grant_service.go), never soft-deleted by a handler; DeleteUser hard-deletes them",
	"notification_configs": "no handler soft-deletes it (health bookkeeping updates only); DeleteUser hard-deletes it",
	"job_executions":       "scheduler lock rows; deleted_at is never set, the row is the lock and is never deleted",
	"webhook_deliveries":   "own retention job (PurgeExpiredWebhookDeliveries, WEBHOOK_DELIVERY_RETENTION_DAYS, issue #622) and FK-cascaded with the webhook",
}

// purgeParentTables are purged by PurgeSoftDeletedRows itself, after the
// edge cleanups, not through purgedSoftDeleteModels.
var purgeParentTables = map[string]bool{"contacts": true}

func softDeleteTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var tables []string
	require.NoError(t, db.Raw(
		`SELECT m.name FROM sqlite_master m
		 WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'
		   AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'deleted_at')
		 ORDER BY m.name`).Scan(&tables).Error)
	return tables
}

func purgedTableNames(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, m := range purgedSoftDeleteModels {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		out[stmt.Schema.Table] = true
	}
	return out
}

func TestPurgeCompleteness_EverySoftDeleteTableIsPurgedOrExcluded(t *testing.T) {
	db := dbtest.New(t)
	purged := purgedTableNames(t, db)

	present := map[string]bool{}
	for _, table := range softDeleteTables(t, db) {
		present[table] = true
		_, excluded := purgeExcluded[table]
		switch {
		case purged[table] || purgeParentTables[table]:
			assert.False(t, excluded, "%s is purged AND excluded — drop the stale exclusion", table)
		case excluded:
			// deliberate, reason recorded
		default:
			t.Errorf("table %s has deleted_at but is neither in purgedSoftDeleteModels nor purgeExcluded "+
				"(services/purge_service.go): soft-deleted rows would live forever (issue #1310)", table)
		}
	}
	for table, reason := range purgeExcluded {
		assert.True(t, present[table], "purgeExcluded names %s, which has no deleted_at column (stale)", table)
		assert.NotEmpty(t, reason, "purgeExcluded[%s] needs a reason", table)
	}
	for table := range purged {
		assert.True(t, present[table], "purgedSoftDeleteModels names %s, which has no deleted_at column", table)
	}
}

// TestPurgeCompleteness_OccasionsWebhooksCompletionsPurged is the behavioral
// half: for each model #1310 added to the purge, one soft-deleted row past
// retention is hard-deleted and one inside retention survives, and the
// attendee join rows follow their event.
func TestPurgeCompleteness_OccasionsWebhooksCompletionsPurged(t *testing.T) {
	db, userID := newPurgeDB(t)
	old := time.Now().AddDate(0, 0, -400)
	recent := time.Now().AddDate(0, 0, -1)

	contact := models.Contact{UserID: userID, Firstname: "Purge", Lastname: "Me"}
	require.NoError(t, db.Create(&contact).Error)

	type seeded struct {
		table string
		model any
		id    any
	}
	var oldRows, keepRows []seeded

	mk := func(kind string, when time.Time) []seeded {
		var out []seeded
		ob := models.OccasionObligation{UserID: userID, EntityID: contact.VCardUID, Kind: "card", Label: "ob-" + kind}
		require.NoError(t, db.Create(&ob).Error)
		out = append(out, seeded{"occasion_obligations", &models.OccasionObligation{}, ob.ID})
		ev := models.OccasionEvent{UserID: userID, Title: "ev-" + kind, StartsAt: time.Now()}
		require.NoError(t, db.Create(&ev).Error)
		out = append(out, seeded{"occasion_events", &models.OccasionEvent{}, ev.ID})
		wh := models.Webhook{UserID: userID, Name: kind, URL: "https://example.com/" + kind, Events: []string{"contact.created"}, Secret: "s"}
		require.NoError(t, db.Create(&wh).Error)
		out = append(out, seeded{"webhooks", &models.Webhook{}, wh.ID})
		rc := models.ReminderCompletion{UserID: userID, ContactID: contact.ID, Message: kind, CompletedAt: time.Now()}
		require.NoError(t, db.Create(&rc).Error)
		out = append(out, seeded{"reminder_completions", &models.ReminderCompletion{}, rc.ID})
		for _, s := range out {
			softDeleteAt(t, db, s.model, s.id, when)
		}
		return out
	}
	oldRows = mk("old", old)
	keepRows = mk("keep", recent)

	// An attendee on the old (purged) event, and one on the kept event.
	oldEventID := oldRows[1].id.(string)
	keepEventID := keepRows[1].id.(string)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: oldEventID, EntityID: contact.VCardUID, RSVP: models.OccasionEventRSVPPending}).Error)
	other := models.Contact{UserID: userID, Firstname: "Other", Lastname: "Person"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: keepEventID, EntityID: other.VCardUID, RSVP: models.OccasionEventRSVPPending}).Error)

	require.NoError(t, PurgeSoftDeletedRows(db, purgeConfig()))

	count := func(s seeded) int64 {
		var n int64
		require.NoError(t, db.Unscoped().Model(s.model).Where("id = ?", s.id).Count(&n).Error)
		return n
	}
	for _, s := range oldRows {
		assert.Zero(t, count(s), "%s: soft-deleted past retention must be hard-deleted", s.table)
	}
	for _, s := range keepRows {
		assert.EqualValues(t, 1, count(s), "%s: soft-deleted inside retention must survive", s.table)
	}

	var attendees int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("event_id = ?", oldEventID).Count(&attendees).Error)
	assert.Zero(t, attendees, "attendees of a purged event must go with it")
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("event_id = ?", keepEventID).Count(&attendees).Error)
	assert.EqualValues(t, 1, attendees, "attendees of a surviving event are untouched")
}

// The issue's reproduction: contact + obligation, delete the contact the way
// DeleteContact does (obligation soft-deleted with it), backdate, purge — the
// obligation must not outlive the contact as an orphan.
func TestPurgeCompleteness_ContactPurgeLeavesNoOrphanOccasionRows(t *testing.T) {
	db, userID := newPurgeDB(t)
	old := time.Now().AddDate(0, 0, -400)

	contact := models.Contact{UserID: userID, Firstname: "Gone", Lastname: "Contact"}
	require.NoError(t, db.Create(&contact).Error)
	// One soft-deleted (as DeleteContact leaves it) and one LIVE obligation /
	// attendee still naming the purged contact (defense-in-depth cleanup).
	softOb := models.OccasionObligation{UserID: userID, EntityID: contact.VCardUID, Kind: "card", Label: "soft"}
	liveOb := models.OccasionObligation{UserID: userID, EntityID: contact.VCardUID, Kind: "card", Label: "live"}
	require.NoError(t, db.Create(&softOb).Error)
	require.NoError(t, db.Create(&liveOb).Error)
	softDeleteAt(t, db, &models.OccasionObligation{}, softOb.ID, old)

	ev := models.OccasionEvent{UserID: userID, Title: "live event", StartsAt: time.Now()}
	require.NoError(t, db.Create(&ev).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: ev.ID, EntityID: contact.VCardUID, RSVP: models.OccasionEventRSVPPending}).Error)

	require.NoError(t, db.Delete(&contact).Error)
	softDeleteAt(t, db, &models.Contact{}, contact.ID, old)

	require.NoError(t, PurgeSoftDeletedRows(db, purgeConfig()))

	var n int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", contact.ID).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Unscoped().Model(&models.OccasionObligation{}).Where("entity_id = ?", contact.VCardUID).Count(&n).Error)
	assert.Zero(t, n, "no occasion_obligations may outlive the purged contact")
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("entity_id = ?", contact.VCardUID).Count(&n).Error)
	assert.Zero(t, n, "no attendee row may name the purged contact")
	require.NoError(t, db.Model(&models.OccasionEvent{}).Where("id = ?", ev.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "the live event itself is untouched")
}
