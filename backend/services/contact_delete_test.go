package services

// Contact delete cascade registry guards (issue #1495, ADR 0035). The registry
// replaces the hand-written checklist in controllers/contact_controller.go; the
// completeness test below is the mechanical "a new dependent table fails CI
// until classified" gate, driven by the real migrated schema (trap #1).

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// contactRefColumns are the column names through which a table can reference a
// contact (by Contact.ID or Contact.VCardUID). A table carrying one of them
// that is not registered must be listed in contactCascadeNotRegistered.
var contactRefColumns = []string{
	"contact_id", "contact_vcard_uid", "member_vcard_uid", "entity_id",
	"source_id", "target_id", "uid_low", "uid_high", "self_contact_vcard_uid",
}

// contactCascadeNotRegistered lists tables that carry a contact-reference
// column (or an FK to contacts) yet are deliberately NOT part of the contact
// delete cascade. Every entry needs a reason.
var contactCascadeNotRegistered = map[string]string{
	"audit_events": "append-only audit trail keyed polymorphically by entity_id; a contact delete must leave its own history intact",
}

// registryIndirectTables are registered tables that reference a contact only
// through a parent row (so the column scan cannot see them).
var registryIndirectTables = map[string]string{
	"notification_deliveries": "keyed by reminder_id; reaches the contact through its reminder",
}

func contactReferencingTables(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	var rows []struct {
		T string
		C string
	}
	// GLOB, not LIKE: LIKE's "_" is a wildcard and would drop the "gifts" table
	// as an FTS shadow table.
	require.NoError(t, db.Raw(`SELECT m.name AS t, p.name AS c
		FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT GLOB 'sqlite_*' AND m.name NOT GLOB '*_fts' AND m.name NOT GLOB '*_fts_*'
		  AND p.name IN ?
		ORDER BY m.name, p.name`, contactRefColumns).Scan(&rows).Error)
	for _, r := range rows {
		out[r.T] = "column " + r.C
	}
	var fks []struct{ T string }
	require.NoError(t, db.Raw(`SELECT DISTINCT m.name AS t FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table' AND f."table" = 'contacts'`).Scan(&fks).Error)
	for _, r := range fks {
		out[r.T] = "foreign key to contacts"
	}
	return out
}

func TestContactCascadeRegistry_CoversEverySchemaTableReferencingAContact(t *testing.T) {
	db := dbtest.New(t)
	registered := map[string]bool{}
	for _, s := range ContactCascadeRegistry() {
		registered[s.Table] = true
	}

	refs := contactReferencingTables(t, db)
	var unclassified []string
	for table, why := range refs {
		_, exempt := contactCascadeNotRegistered[table]
		switch {
		case registered[table]:
			assert.False(t, exempt, "%s is registered AND listed in contactCascadeNotRegistered — drop the stale exemption", table)
		case exempt:
		default:
			unclassified = append(unclassified, table+" ("+why+")")
		}
	}
	sort.Strings(unclassified)
	require.Empty(t, unclassified,
		"tables referencing a contact with no ContactCascadeRegistry step (services/contact_delete.go) and no "+
			"contactCascadeNotRegistered entry — a contact delete would leave their rows behind (CLAUDE.md trap #6, ADR 0035):\n  %s",
		strings.Join(unclassified, "\n  "))

	// Both directions: nothing stale on either side.
	schema := map[string]bool{}
	var names []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='table'`).Scan(&names).Error)
	for _, n := range names {
		schema[n] = true
	}
	for table := range registered {
		assert.True(t, schema[table], "registry step names %s, which is not in the migrated schema (stale)", table)
		_, viaColumn := refs[table]
		_, indirect := registryIndirectTables[table]
		assert.True(t, viaColumn || indirect,
			"registry step %s has no contact-reference column or FK and is not in registryIndirectTables — "+
				"it is either misnamed or does not belong in the contact cascade", table)
	}
	for table, reason := range contactCascadeNotRegistered {
		assert.True(t, schema[table], "contactCascadeNotRegistered names %s, not in the schema (stale)", table)
		assert.NotEmpty(t, reason)
	}
	for table, reason := range registryIndirectTables {
		assert.True(t, registered[table], "registryIndirectTables names %s, which is not registered (stale)", table)
		assert.NotEmpty(t, reason)
	}
}

func TestContactCascadeRegistry_StepsAreWellFormed(t *testing.T) {
	db := dbtest.New(t)
	seen := map[string]bool{}
	for _, s := range ContactCascadeRegistry() {
		assert.False(t, seen[s.Table], "table %s appears twice in the registry", s.Table)
		seen[s.Table] = true
		assert.NotEmpty(t, s.Reason, "%s needs a recorded reason", s.Table)
		if s.Apply == nil {
			require.NotNil(t, s.Match, "%s needs a Match", s.Table)
			q, args := s.Match(ContactCascadeTarget{ID: 1, VCardUID: "u", UserID: 2})
			assert.Contains(t, q, "user_id", "%s: every Match must scope by user_id (trap #5)", s.Table)
			assert.NotEmpty(t, args)
		}

		switch s.Mode {
		case CascadeSoft, CascadeHard:
			assert.Nil(t, s.Apply, "%s: delete steps use the default delete", s.Table)
			if s.NewModel == nil {
				assert.Equal(t, CascadeHard, s.Mode, "%s: a raw join-table delete is a hard delete", s.Table)
				continue
			}
			stmt := &gorm.Statement{DB: db}
			require.NoError(t, stmt.Parse(s.NewModel()))
			assert.Equal(t, s.Table, stmt.Schema.Table, "%s: NewModel maps to a different table", s.Table)
			hasDeletedAt := stmt.Schema.LookUpField("DeletedAt") != nil
			assert.Equal(t, s.Mode == CascadeSoft, hasDeletedAt,
				"%s: mode %q disagrees with the model (soft <=> it carries gorm.DeletedAt) — trap #7", s.Table, s.Mode)
		case CascadeMutate:
			assert.NotNil(t, s.Apply, "%s: a mutate step needs Apply", s.Table)
		default:
			t.Errorf("%s has unknown mode %q", s.Table, s.Mode)
		}
	}
}

func TestContactCascadeRegistry_ReturnsFreshSlice(t *testing.T) {
	a := ContactCascadeRegistry()
	a[0].Table = "tampered"
	assert.NotEqual(t, "tampered", ContactCascadeRegistry()[0].Table)
}

func TestContactCascadeRegistry_RemindersPrecedeLifeEventsAndDeliveries(t *testing.T) {
	idx := map[string]int{}
	for i, s := range ContactCascadeRegistry() {
		idx[s.Table] = i
	}
	assert.Less(t, idx["notification_deliveries"], idx["reminders"], "deliveries must be cleared before their reminders are soft-deleted")
	assert.Less(t, idx["reminders"], idx["life_events"], "life-event-linked reminders must go before the life events they reference")
}

type cascadeFixture struct {
	db      *gorm.DB
	userID  uint
	contact models.Contact
	other   models.Contact
}

func newCascadeFixture(t *testing.T) cascadeFixture {
	t.Helper()
	db, userID := newPurgeDB(t)
	contact := models.Contact{UserID: userID, Firstname: "Del", Lastname: "Me"}
	require.NoError(t, db.Create(&contact).Error)
	other := models.Contact{UserID: userID, Firstname: "Keep", Lastname: "Me"}
	require.NoError(t, db.Create(&other).Error)
	return cascadeFixture{db: db, userID: userID, contact: contact, other: other}
}

func (f cascadeFixture) seed(t *testing.T) {
	t.Helper()
	db, uid := f.db, f.contact.VCardUID
	rem := models.Reminder{UserID: f.userID, ContactID: &f.contact.ID, Message: "m", RemindAt: time.Now().Add(time.Hour), Recurrence: "once"}
	require.NoError(t, db.Create(&rem).Error)
	require.NoError(t, db.Create(&models.NotificationDelivery{ReminderID: rem.ID, Channel: "email", Status: "pending"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: f.userID, ContactID: &f.contact.ID, Content: "n", Date: time.Now()}).Error)
	act := models.Activity{UserID: f.userID, Title: "a", Date: time.Now()}
	require.NoError(t, db.Create(&act).Error)
	require.NoError(t, db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", act.ID, f.contact.ID).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{UserID: f.userID, SourceID: uid, TargetID: f.other.VCardUID, Type: "related_to"}).Error)
	require.NoError(t, db.Create(&models.Gift{UserID: f.userID, EntityID: uid, Description: "g"}).Error)
	require.NoError(t, db.Create(&models.DismissedDuplicatePair{UserID: f.userID, UIDLow: uid, UIDHigh: f.other.VCardUID}).Error)
	require.NoError(t, db.Create(&models.Feed{UserID: f.userID, Name: "f", Kind: models.FeedKindContact, EntityID: uid, Detail: models.FeedDetailFull, TokenHash: "h1"}).Error)
	require.NoError(t, db.Create(&models.Feed{UserID: f.userID, Name: "agg", Kind: models.FeedKindAggregate, TokenHash: "h2"}).Error)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", f.userID).Update("self_contact_vcard_uid", uid).Error)
}

func count(t *testing.T, db *gorm.DB, unscoped bool, model any, where string, args ...any) int64 {
	t.Helper()
	q := db.Model(model)
	if unscoped {
		q = q.Unscoped()
	}
	var n int64
	require.NoError(t, q.Where(where, args...).Count(&n).Error)
	return n
}

func TestDeleteContact_RunsRegistryAndSoftDeletesContact(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	now := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, now))

	db := f.db
	assert.Zero(t, count(t, db, false, &models.Contact{}, "id = ?", f.contact.ID), "contact soft-deleted")
	assert.EqualValues(t, 1, count(t, db, true, &models.Contact{}, "id = ?", f.contact.ID), "tombstone remains (soft)")
	assert.EqualValues(t, 1, count(t, db, false, &models.Contact{}, "id = ?", f.other.ID), "unrelated contact untouched")

	assert.Zero(t, count(t, db, false, &models.Reminder{}, "contact_id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, true, &models.Reminder{}, "contact_id = ?", f.contact.ID), "reminder soft")
	assert.Zero(t, count(t, db, true, &models.NotificationDelivery{}, "1 = 1"), "delivery hard-deleted")
	assert.Zero(t, count(t, db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID))
	assert.EqualValues(t, 1, count(t, db, true, &models.Gift{}, "entity_id = ?", f.contact.VCardUID), "gift soft")
	assert.Zero(t, count(t, db, true, &models.RelationshipEdge{}, "user_id = ?", f.userID), "edge hard")
	assert.Zero(t, count(t, db, true, &models.DismissedDuplicatePair{}, "user_id = ?", f.userID), "pair hard")
	var links int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM activity_contacts").Scan(&links).Error)
	assert.Zero(t, links, "join row removed")

	// Notes tombstone is stamped with the injected time (no wall clock).
	var note models.Note
	require.NoError(t, db.Unscoped().Where("contact_id = ?", f.contact.ID).First(&note).Error)
	assert.True(t, note.DeletedAt.Valid)
	assert.True(t, note.UpdatedAt.Equal(now), "note updated_at = %v, want injected %v", note.UpdatedAt, now)

	// Feed: contact feed revoked at `now`, aggregate untouched.
	var feed models.Feed
	require.NoError(t, db.Where("token_hash = ?", "h1").First(&feed).Error)
	require.NotNil(t, feed.RevokedAt)
	assert.True(t, feed.RevokedAt.Equal(now))
	var agg models.Feed
	require.NoError(t, db.Where("token_hash = ?", "h2").First(&agg).Error)
	assert.Nil(t, agg.RevokedAt)

	// Self pointer cleared.
	var user models.User
	require.NoError(t, db.First(&user, f.userID).Error)
	assert.Nil(t, user.SelfContactVCardUID)
}

func TestDeleteContactAssociations_LeavesContactRowAndOtherUsersAlone(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	// Same-ID-shaped rows for another user must survive (user_id scoping).
	stranger := models.User{Username: "stranger", Email: "s@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&stranger).Error)

	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return DeleteContactAssociations(tx, f.contact, stranger.ID, time.Now())
	}))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID), "wrong user_id deletes nothing")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID))

	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return DeleteContactAssociations(tx, f.contact, f.userID, time.Now())
	}))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID), "associations only; contact row remains")
	assert.Zero(t, count(t, f.db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID))
}

// Every step's failure must propagate and roll the whole delete back: hide each
// step's table in turn and require DeleteContact to fail with the contact and
// its already-processed associations intact.
func TestDeleteContact_EveryStepFailurePropagatesAndRollsBack(t *testing.T) {
	for _, step := range ContactCascadeRegistry() {
		t.Run(step.Table, func(t *testing.T) {
			f := newCascadeFixture(t)
			f.seed(t)
			dbtest.HideTable(t, f.db, step.Table)

			err := DeleteContact(f.db, f.contact, f.userID, time.Now())
			require.Error(t, err, "hiding %s must fail the delete", step.Table)

			assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID), "contact must survive a failed cascade")
			if step.Table != "reminders" && step.Table != "notification_deliveries" {
				assert.EqualValues(t, 1, count(t, f.db, false, &models.Reminder{}, "contact_id = ?", f.contact.ID),
					"earlier steps must roll back with the failed one")
			}
		})
	}
}

func TestRunCascadeStep_AfterErrorPropagates(t *testing.T) {
	f := newCascadeFixture(t)
	boom := errors.New("after failed")
	step := CascadeStep{
		Table: "gifts", NewModel: func() any { return &models.Gift{} }, Mode: CascadeSoft,
		Match: func(t ContactCascadeTarget) (string, []any) {
			return "entity_id = ? AND user_id = ?", []any{t.VCardUID, t.UserID}
		},
		After: func(*gorm.DB, ContactCascadeTarget, time.Time) error { return boom },
	}
	err := runCascadeStep(f.db, step, ContactCascadeTarget{ID: f.contact.ID, VCardUID: f.contact.VCardUID, UserID: f.userID}, time.Now())
	assert.ErrorIs(t, err, boom)
}

func TestRunCascadeStep_ApplyErrorPropagatesAndSkipsAfter(t *testing.T) {
	f := newCascadeFixture(t)
	boom := errors.New("apply failed")
	afterRan := false
	step := CascadeStep{
		Table: "x", Mode: CascadeMutate,
		Apply: func(*gorm.DB, ContactCascadeTarget, time.Time) error { return boom },
		After: func(*gorm.DB, ContactCascadeTarget, time.Time) error { afterRan = true; return nil },
	}
	err := runCascadeStep(f.db, step, ContactCascadeTarget{}, time.Now())
	assert.ErrorIs(t, err, boom)
	assert.False(t, afterRan)
}
