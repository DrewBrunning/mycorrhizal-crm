package controllers

// Merge-repoint completeness guard (issue #1309), the merge counterpart of
// delete_cascade_coverage_test.go.
//
// CommitContactMerge re-points the loser's associations
// (services.RepointContactAssociations) and then runs services.DeleteContactAssociations
// as a defense-in-depth sweep. Any contact-keyed table the repoint forgets is
// therefore silently destroyed (or revoked) on every merge — which shipped for
// occasion_obligations, occasion_event_attendees and contact feeds, because each
// was added to the delete cascade and not to the repoint, and nothing compared
// the two.
//
// The guard is driven by the migrated schema, not by a hand-kept table list:
//
//  1. every table with a column that can reference a contact (contact_id,
//     *_vcard_uid, entity_id, entity_uid, uid_low/uid_high, source_id/target_id)
//     must be classified below as repointed, dropped-by-design (with a reason)
//     or retained-history; an unclassified one FAILS, as does a classification
//     naming a table or column that no longer exists;
//  2. every `repointed` table must have a behavioral seeder in mergeSeeders;
//     the sweep seeds one row on the loser, runs the real merge endpoint, and
//     asserts a live row now references the keeper and none references the
//     loser. Removing any repoint step fails exactly that table.
//
// To extend it (a new contact-keyed table): add the table to mergeCoverage as
// `repointed` (and write its mergeSeeder) or `droppedByDesign`/`retained` with
// a reason. Follow-ups that add merge behavior build on these two maps.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type mergeDisposition int

const (
	// mergeRepointed: the row moves to the keeper (possibly deduped on a
	// unique-key collision — the keeper's row wins).
	mergeRepointed mergeDisposition = iota
	// mergeDroppedByDesign: the row is deliberately discarded with the loser.
	mergeDroppedByDesign
	// mergeRetained: history that intentionally keeps naming the loser.
	mergeRetained
)

type mergeClass struct {
	disposition mergeDisposition
	// keyCol is the contact-referencing column the sweep asserts on. For
	// tables keyed by Contact.ID rather than VCardUID, byID is true.
	keyCol string
	byID   bool
	reason string // required for dropped/retained
}

// mergeCoverage classifies every contact-referencing table. Keys must exist in
// the migrated schema (stale entries fail).
var mergeCoverage = map[string]mergeClass{
	// --- repointed --------------------------------------------------------
	"activity_contacts":                 {disposition: mergeRepointed, keyCol: "contact_id", byID: true},
	"attachments":                       {disposition: mergeRepointed, keyCol: "contact_vcard_uid"},
	"cadence_policies":                  {disposition: mergeRepointed, keyCol: "entity_id"},
	"circle_members":                    {disposition: mergeRepointed, keyCol: "member_vcard_uid"},
	"contact_tags":                      {disposition: mergeRepointed, keyCol: "contact_vcard_uid"},
	"conversation_agenda":               {disposition: mergeRepointed, keyCol: "entity_id"},
	"data_decay_policies":               {disposition: mergeRepointed, keyCol: "entity_id"},
	"external_activities":               {disposition: mergeRepointed, keyCol: "entity_id"},
	"external_identities":               {disposition: mergeRepointed, keyCol: "entity_id"},
	"feeds":                             {disposition: mergeRepointed, keyCol: "entity_id"},
	"field_values":                      {disposition: mergeRepointed, keyCol: "entity_id"},
	"gifts":                             {disposition: mergeRepointed, keyCol: "entity_id"},
	"household_members":                 {disposition: mergeRepointed, keyCol: "member_vcard_uid"},
	"import_source_links":               {disposition: mergeRepointed, keyCol: "entity_uid"},
	"life_event_suggestion_resolutions": {disposition: mergeRepointed, keyCol: "entity_id"},
	"life_events":                       {disposition: mergeRepointed, keyCol: "entity_id"},
	"notes":                             {disposition: mergeRepointed, keyCol: "contact_id", byID: true},
	"occasion_event_attendees":          {disposition: mergeRepointed, keyCol: "entity_id"},
	"occasion_obligations":              {disposition: mergeRepointed, keyCol: "entity_id"},
	"preferences":                       {disposition: mergeRepointed, keyCol: "entity_id"},
	"reach_out_suggestions":             {disposition: mergeRepointed, keyCol: "contact_vcard_uid"},
	"relationship_edges":                {disposition: mergeRepointed, keyCol: "source_id"},
	"reminder_completions":              {disposition: mergeRepointed, keyCol: "contact_id", byID: true},
	"reminders":                         {disposition: mergeRepointed, keyCol: "contact_id", byID: true},
	"users":                             {disposition: mergeRepointed, keyCol: "self_contact_vcard_uid"},
	// --- dropped by design ------------------------------------------------
	"contact_sync_links": {disposition: mergeDroppedByDesign, keyCol: "contact_id", byID: true,
		reason: "CardDAV href binding of the loser's own remote card; the keeper keeps its own link (deleted by services.DeleteContactAssociations)"},
	"contact_sync_conflicts": {disposition: mergeDroppedByDesign, keyCol: "contact_id", byID: true,
		reason: "pending review rows about the loser's remote card; nothing to review once the loser is gone"},
	"dismissed_duplicate_pairs": {disposition: mergeDroppedByDesign, keyCol: "uid_low",
		reason: "a dismissal is a decision about the loser as a distinct contact; the merged contact is a new identity for duplicate detection"},
	// --- retained history -------------------------------------------------
	"audit_events": {disposition: mergeRetained, keyCol: "entity_id",
		reason: "append-only audit trail (ADR: audit is never rewritten); the merge itself is recorded on the keeper"},
}

// contactRefColumns reports whether a column can reference a contact.
func contactRefColumn(name string) bool {
	switch name {
	case "contact_id", "entity_id", "entity_uid", "uid_low", "uid_high", "source_id", "target_id":
		return true
	}
	return strings.HasSuffix(name, "_vcard_uid")
}

func tableColumns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var rows []struct{ Name string }
	require.NoError(t, db.Raw(fmt.Sprintf("PRAGMA table_info(%q)", table)).Scan(&rows).Error)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

func TestMergeRepointCoverage_EveryContactKeyedTableIsClassified(t *testing.T) {
	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	schema := schemaTables(t, db)
	var unclassified []string
	for tb := range schema {
		if strings.HasSuffix(tb, "_fts") || strings.Contains(tb, "_fts_") {
			continue
		}
		var hits []string
		for _, col := range tableColumns(t, db, tb) {
			if contactRefColumn(col) {
				hits = append(hits, col)
			}
		}
		if len(hits) == 0 {
			continue
		}
		if _, ok := mergeCoverage[tb]; !ok {
			unclassified = append(unclassified, fmt.Sprintf("%s%v", tb, hits))
		}
	}
	require.Empty(t, unclassified,
		"tables with a contact-referencing column but no merge disposition — decide repointed / dropped-by-design / retained "+
			"in mergeCoverage (and update services.RepointContactAssociations + mergeSeeders):\n  %v", unclassified)

	for tb, cls := range mergeCoverage {
		require.True(t, schema[tb], "mergeCoverage names %s, which is not in the migrated schema", tb)
		require.Contains(t, tableColumns(t, db, tb), cls.keyCol, "mergeCoverage key column %s.%s does not exist", tb, cls.keyCol)
		if cls.disposition != mergeRepointed {
			require.NotEmpty(t, cls.reason, "%s is not repointed and needs a written reason", tb)
		}
		if cls.disposition == mergeRepointed {
			_, ok := mergeSeeders[tb]
			require.True(t, ok, "%s is declared repointed but has no mergeSeeder — the behavioral sweep cannot prove the repoint", tb)
		}
	}
	for tb := range mergeSeeders {
		cls, ok := mergeCoverage[tb]
		require.True(t, ok && cls.disposition == mergeRepointed, "mergeSeeders has %s, which is not declared repointed", tb)
	}
}

// mergeFixture is the shared world one behavioral sweep seeds into.
type mergeFixture struct {
	db     *gorm.DB
	userID uint
	other  models.Contact
	cont   cascadeContainers
	event  models.OccasionEvent
}

// mergeSeeders seeds exactly one row in the table referencing c (the loser).
var mergeSeeders = map[string]func(t *testing.T, f *mergeFixture, c models.Contact){
	"activity_contacts": func(t *testing.T, f *mergeFixture, c models.Contact) {
		a := models.Activity{UserID: f.userID, Title: "act", Date: time.Now()}
		require.NoError(t, f.db.Create(&a).Error)
		require.NoError(t, f.db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", a.ID, c.ID).Error)
	},
	"attachments": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Attachment{UserID: f.userID, ContactVCardUID: c.VCardUID, StoredName: "s", OriginalName: "o", ContentType: "text/plain", SizeBytes: 1}).Error)
	},
	"cadence_policies": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.CadencePolicy{UserID: f.userID, EntityID: c.VCardUID, TargetIntervalDays: 30}).Error)
	},
	"circle_members": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.CircleMember{CircleID: f.cont.circleID, UserID: f.userID, MemberVCardUID: c.VCardUID}).Error)
	},
	"contact_tags": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ContactTag{TagID: f.cont.tagID, UserID: f.userID, ContactVCardUID: c.VCardUID}).Error)
	},
	"conversation_agenda": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ConversationAgenda{UserID: f.userID, EntityID: c.VCardUID, Content: "agenda"}).Error)
	},
	"data_decay_policies": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.DataDecayPolicy{UserID: f.userID, EntityID: c.VCardUID, IntervalDays: 365}).Error)
	},
	"external_activities": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ExternalActivity{UserID: f.userID, EntityID: c.VCardUID, SourceSystem: "immich", ExternalID: "a-1", Type: "photo-appearance", OccurredAt: time.Now()}).Error)
	},
	"external_identities": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ExternalIdentity{UserID: f.userID, EntityID: c.VCardUID, System: "immich", ExternalID: "p-1"}).Error)
	},
	"feeds": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Feed{UserID: f.userID, Name: "loser feed", Kind: models.FeedKindContact, EntityID: c.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "hash-" + c.VCardUID}).Error)
	},
	"field_values": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.FieldValue{FieldDefinitionID: f.cont.fieldDef.ID, UserID: f.userID, EntityID: c.VCardUID, Value: json.RawMessage(`"v"`)}).Error)
	},
	"gifts": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Gift{UserID: f.userID, EntityID: c.VCardUID, Description: "gift"}).Error)
	},
	"household_members": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.HouseholdMember{HouseholdID: f.cont.householdID, UserID: f.userID, MemberVCardUID: c.VCardUID, Role: "adult"}).Error)
	},
	"import_source_links": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ImportSourceLink{UserID: f.userID, System: "monica", ExternalID: "contact/1", EntityKind: models.ImportSourceLinkKindContact, EntityUID: c.VCardUID}).Error)
	},
	"life_event_suggestion_resolutions": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.LifeEventSuggestionResolution{UserID: f.userID, EntityID: c.VCardUID, SourceKind: "address", SourceEntryID: "e1", EventType: "moved", Resolution: "dismissed"}).Error)
	},
	"life_events": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.LifeEvent{UserID: f.userID, EntityID: c.VCardUID, Type: "custom"}).Error)
	},
	"notes": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Note{UserID: f.userID, ContactID: &c.ID, Content: "n", Date: time.Now()}).Error)
	},
	"occasion_event_attendees": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.OccasionEventAttendee{UserID: f.userID, EventID: f.event.ID, EntityID: c.VCardUID, RSVP: models.OccasionEventRSVPPending}).Error)
	},
	"occasion_obligations": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.OccasionObligation{UserID: f.userID, EntityID: c.VCardUID, Kind: "card", Label: "occasion"}).Error)
	},
	"preferences": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Preference{UserID: f.userID, EntityID: c.VCardUID, Category: "food", Value: "pizza"}).Error)
	},
	"reach_out_suggestions": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ReachOutSuggestion{UserID: f.userID, ContactVCardUID: c.VCardUID, Kind: "organization", OldValue: "a", NewValue: "b", AuditEventID: 1, Status: "pending"}).Error)
	},
	"relationship_edges": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.RelationshipEdge{UserID: f.userID, SourceID: c.VCardUID, TargetID: f.other.VCardUID, Type: "friend_of",
			Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal}).Error)
	},
	"reminder_completions": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.ReminderCompletion{UserID: f.userID, ContactID: c.ID, Message: "done", CompletedAt: time.Now()}).Error)
	},
	"reminders": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Create(&models.Reminder{UserID: f.userID, ContactID: &c.ID, Message: "m", RemindAt: time.Now().Add(24 * time.Hour), Recurrence: "once"}).Error)
	},
	"users": func(t *testing.T, f *mergeFixture, c models.Contact) {
		require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.userID).Update("self_contact_vcard_uid", c.VCardUID).Error)
	},
}

// mergeRouter builds a real-schema router with the merge endpoints and one
// seeded user (setupRouter), returning a JSON poster.
func mergeRouter(t *testing.T) (*gorm.DB, uint, func(body any) *httptest.ResponseRecorder) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)
	return db, user.ID, func(body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
		req, _ := http.NewRequest("POST", "/contacts/merge", &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
}

func newMergePair(t *testing.T, db *gorm.DB, userID uint) (keeper, loser, other models.Contact) {
	t.Helper()
	keeper = models.Contact{UserID: userID, Firstname: "Kay", Lastname: "Keeper"}
	loser = models.Contact{UserID: userID, Firstname: "Kay", Lastname: "Keeper"}
	other = models.Contact{UserID: userID, Firstname: "Oscar"}
	require.NoError(t, db.Create(&keeper).Error)
	require.NoError(t, db.Create(&loser).Error)
	require.NoError(t, db.Create(&other).Error)
	return
}

// liveRefCount counts rows in table whose keyCol equals key, excluding
// soft-deleted rows when the table has a deleted_at column (trap #6: a
// soft-deleted row is "moved" only in the sense of being destroyed, so it must
// not count as surviving).
func liveRefCount(t *testing.T, db *gorm.DB, table, keyCol string, key any) int64 {
	t.Helper()
	q := db.Table(table).Where(fmt.Sprintf("%s = ?", keyCol), key)
	if tableHasColumn(t, db, table, "deleted_at") {
		q = q.Where("deleted_at IS NULL")
	}
	var n int64
	require.NoError(t, q.Count(&n).Error)
	return n
}

func TestMergeRepointCoverage_EveryRepointedTableFollowsTheKeeper(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, other := newMergePair(t, db, userID)

	f := &mergeFixture{db: db, userID: userID, other: other, cont: seedContactContainers(t, db, userID, loser)}
	f.event = models.OccasionEvent{UserID: userID, Title: "party", StartsAt: time.Now()}
	require.NoError(t, db.Create(&f.event).Error)

	for tb, seed := range mergeSeeders {
		seed(t, f, loser)
		cls := mergeCoverage[tb]
		key := any(loser.VCardUID)
		if cls.byID {
			key = loser.ID
		}
		require.EqualValues(t, 1, liveRefCount(t, db, tb, cls.keyCol, key), "%s: seeder did not put a live row on the loser", tb)
	}

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	for tb := range mergeSeeders {
		cls := mergeCoverage[tb]
		keeperKey, loserKey := any(keeper.VCardUID), any(loser.VCardUID)
		if cls.byID {
			keeperKey, loserKey = keeper.ID, loser.ID
		}
		assert.EqualValues(t, 0, liveRefCount(t, db, tb, cls.keyCol, loserKey), "%s: a live row still references the merged-away contact", tb)
		assert.GreaterOrEqual(t, liveRefCount(t, db, tb, cls.keyCol, keeperKey), int64(1), "%s: the loser's row did not move to the keeper (destroyed by the merge)", tb)
	}
}

// TestMergeRepoint_Issue1309Reproduction is the reproduction from the issue: an
// obligation, event attendance and a contact feed on the loser end up on the
// keeper, alive, with nothing soft-deleted or revoked.
func TestMergeRepoint_Issue1309Reproduction(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, _ := newMergePair(t, db, userID)

	ob := models.OccasionObligation{UserID: userID, EntityID: loser.VCardUID, Kind: "card", Label: "Christmas card"}
	require.NoError(t, db.Create(&ob).Error)
	ev := models.OccasionEvent{UserID: userID, Title: "wedding", StartsAt: time.Now()}
	require.NoError(t, db.Create(&ev).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: ev.ID, EntityID: loser.VCardUID, RSVP: models.OccasionEventRSVPAccepted}).Error)
	feed := models.Feed{UserID: userID, Name: "L timeline", Kind: models.FeedKindContact, EntityID: loser.VCardUID, Detail: models.FeedDetailFull, TokenHash: "h1"}
	require.NoError(t, db.Create(&feed).Error)
	revoked := time.Now().Add(-time.Hour)
	oldFeed := models.Feed{UserID: userID, Name: "old", Kind: models.FeedKindContact, EntityID: loser.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "h2", RevokedAt: &revoked}
	require.NoError(t, db.Create(&oldFeed).Error)
	agg := models.Feed{UserID: userID, Name: "all", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "h3"}
	require.NoError(t, db.Create(&agg).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Obligation: same row, owned by the keeper, NOT soft-deleted (Unscoped).
	var gotOb models.OccasionObligation
	require.NoError(t, db.Unscoped().First(&gotOb, "id = ?", ob.ID).Error)
	assert.Equal(t, keeper.VCardUID, gotOb.EntityID)
	assert.False(t, gotOb.DeletedAt.Valid, "obligation must not be soft-deleted by the merge")
	var obTotal int64
	require.NoError(t, db.Unscoped().Model(&models.OccasionObligation{}).Where("user_id = ?", userID).Count(&obTotal).Error)
	assert.EqualValues(t, 1, obTotal)

	// Attendance: the keeper attends, with the loser's RSVP; loser has no row.
	var att models.OccasionEventAttendee
	require.NoError(t, db.Where("event_id = ? AND entity_id = ?", ev.ID, keeper.VCardUID).First(&att).Error)
	assert.Equal(t, models.OccasionEventRSVPAccepted, att.RSVP)
	var loserAtt int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("entity_id = ?", loser.VCardUID).Count(&loserAtt).Error)
	assert.EqualValues(t, 0, loserAtt)

	// Feeds: both contact feeds follow the keeper; the active one stays active,
	// the historically revoked one stays revoked, the aggregate is untouched.
	var f1, f2, fa models.Feed
	require.NoError(t, db.First(&f1, "id = ?", feed.ID).Error)
	require.NoError(t, db.First(&f2, "id = ?", oldFeed.ID).Error)
	require.NoError(t, db.First(&fa, "id = ?", agg.ID).Error)
	assert.Equal(t, keeper.VCardUID, f1.EntityID)
	assert.Nil(t, f1.RevokedAt, "the loser's active feed must survive the merge, not be revoked by the sweep")
	assert.Equal(t, keeper.VCardUID, f2.EntityID)
	assert.NotNil(t, f2.RevokedAt)
	assert.Equal(t, "", fa.EntityID)
	assert.Nil(t, fa.RevokedAt)
}

func TestMergeRepoint_AttendeeCollisionKeepsExactlyOneRow(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, _ := newMergePair(t, db, userID)

	shared := models.OccasionEvent{UserID: userID, Title: "shared", StartsAt: time.Now()}
	loserOnly := models.OccasionEvent{UserID: userID, Title: "loser only", StartsAt: time.Now()}
	require.NoError(t, db.Create(&shared).Error)
	require.NoError(t, db.Create(&loserOnly).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: shared.ID, EntityID: keeper.VCardUID, RSVP: models.OccasionEventRSVPAccepted}).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: shared.ID, EntityID: loser.VCardUID, RSVP: models.OccasionEventRSVPDeclined}).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: userID, EventID: loserOnly.ID, EntityID: loser.VCardUID, RSVP: models.OccasionEventRSVPMaybe}).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var rows []models.OccasionEventAttendee
	require.NoError(t, db.Where("user_id = ?", userID).Order("event_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	byEvent := map[string]models.OccasionEventAttendee{}
	for _, r := range rows {
		assert.Equal(t, keeper.VCardUID, r.EntityID)
		byEvent[r.EventID] = r
	}
	assert.Equal(t, models.OccasionEventRSVPAccepted, byEvent[shared.ID].RSVP, "on a collision the keeper's RSVP wins")
	assert.Equal(t, models.OccasionEventRSVPMaybe, byEvent[loserOnly.ID].RSVP)
}

func TestMergeRepoint_ObligationsAccumulateAndKeepFieldsAndStaySoftDeleted(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, _ := newMergePair(t, db, userID)

	kOb := models.OccasionObligation{UserID: userID, EntityID: keeper.VCardUID, Kind: "card", Label: "same label"}
	lOb := models.OccasionObligation{UserID: userID, EntityID: loser.VCardUID, Kind: "card", Label: "same label"}
	require.NoError(t, db.Create(&kOb).Error)
	require.NoError(t, db.Create(&lOb).Error)
	// A previously (user-)deleted loser obligation must not be resurrected.
	dead := models.OccasionObligation{UserID: userID, EntityID: loser.VCardUID, Kind: "card", Label: "gone"}
	require.NoError(t, db.Create(&dead).Error)
	require.NoError(t, db.Delete(&dead).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var live int64
	require.NoError(t, db.Model(&models.OccasionObligation{}).Where("entity_id = ?", keeper.VCardUID).Count(&live).Error)
	assert.EqualValues(t, 2, live, "same-kind/label duplicates are legitimate and both survive")
	var resurrected int64
	require.NoError(t, db.Model(&models.OccasionObligation{}).Where("id = ?", dead.ID).Count(&resurrected).Error)
	assert.EqualValues(t, 0, resurrected, "an already soft-deleted obligation stays deleted")
}

func TestMergeRepoint_FeedsDoNotDangleAndUnrelatedFeedsUntouched(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, other := newMergePair(t, db, userID)

	kFeed := models.Feed{UserID: userID, Name: "K", Kind: models.FeedKindContact, EntityID: keeper.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "k"}
	lFeed := models.Feed{UserID: userID, Name: "L", Kind: models.FeedKindContact, EntityID: loser.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "l"}
	oFeed := models.Feed{UserID: userID, Name: "O", Kind: models.FeedKindContact, EntityID: other.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "o"}
	require.NoError(t, db.Create(&kFeed).Error)
	require.NoError(t, db.Create(&lFeed).Error)
	require.NoError(t, db.Create(&oFeed).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var active int64
	require.NoError(t, db.Model(&models.Feed{}).Where("user_id = ? AND entity_id = ? AND revoked_at IS NULL", userID, keeper.VCardUID).Count(&active).Error)
	assert.EqualValues(t, 2, active, "both credentials survive on the keeper (each independently revocable)")
	var stillLoser int64
	require.NoError(t, db.Model(&models.Feed{}).Where("entity_id = ?", loser.VCardUID).Count(&stillLoser).Error)
	assert.EqualValues(t, 0, stillLoser)
	var o models.Feed
	require.NoError(t, db.First(&o, "id = ?", oFeed.ID).Error)
	assert.Equal(t, other.VCardUID, o.EntityID)
	assert.Nil(t, o.RevokedAt)
	// The total feed count is unchanged, so MaxActiveFeedsPerUser accounting is too.
	var total int64
	require.NoError(t, db.Model(&models.Feed{}).Where("user_id = ?", userID).Count(&total).Error)
	assert.EqualValues(t, 3, total)
}

func TestMergeRepoint_SuggestionResolutionCollisionAndImportLinks(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, _ := newMergePair(t, db, userID)

	res := func(uid, entry, resolution string) {
		require.NoError(t, db.Create(&models.LifeEventSuggestionResolution{UserID: userID, EntityID: uid, SourceKind: "address", SourceEntryID: entry, EventType: "moved", Resolution: resolution}).Error)
	}
	res(keeper.VCardUID, "same", "accepted")
	res(loser.VCardUID, "same", "dismissed") // collides: keeper wins
	res(loser.VCardUID, "only-loser", "dismissed")
	require.NoError(t, db.Create(&models.ImportSourceLink{UserID: userID, System: "monica", ExternalID: "contact/9", EntityKind: models.ImportSourceLinkKindContact, EntityUID: loser.VCardUID}).Error)
	// A non-contact link whose uid happens to equal nothing of ours is untouched.
	require.NoError(t, db.Create(&models.ImportSourceLink{UserID: userID, System: "monica", ExternalID: "note/9", EntityKind: models.ImportSourceLinkKindNote, EntityUID: loser.VCardUID}).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var rs []models.LifeEventSuggestionResolution
	require.NoError(t, db.Where("user_id = ?", userID).Find(&rs).Error)
	require.Len(t, rs, 2)
	for _, r := range rs {
		assert.Equal(t, keeper.VCardUID, r.EntityID)
		if r.SourceEntryID == "same" {
			assert.Equal(t, "accepted", r.Resolution, "keeper's decision wins a collision")
		}
	}
	var link models.ImportSourceLink
	require.NoError(t, db.First(&link, "external_id = ?", "contact/9").Error)
	assert.Equal(t, keeper.VCardUID, link.EntityUID)
	var noteLink models.ImportSourceLink
	require.NoError(t, db.First(&noteLink, "external_id = ?", "note/9").Error)
	assert.Equal(t, loser.VCardUID, noteLink.EntityUID, "only entity_kind=contact links are contact-keyed")
}

func TestMergeRepoint_CrossUserRowsAreNotTouched(t *testing.T) {
	db, userID, post := mergeRouter(t)
	keeper, loser, _ := newMergePair(t, db, userID)

	// A second user's rows that (impossibly, but defensively) carry the same uid.
	u2 := models.User{Username: "other", Password: "password123", Email: "o@example.com"}
	require.NoError(t, db.Create(&u2).Error)
	require.NoError(t, db.Create(&models.OccasionObligation{UserID: u2.ID, EntityID: loser.VCardUID, Kind: "k", Label: "l"}).Error)
	require.NoError(t, db.Create(&models.Feed{UserID: u2.ID, Name: "x", Kind: models.FeedKindContact, EntityID: loser.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "x"}).Error)

	w := post(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var ob models.OccasionObligation
	require.NoError(t, db.First(&ob, "user_id = ?", u2.ID).Error)
	assert.Equal(t, loser.VCardUID, ob.EntityID)
	var fd models.Feed
	require.NoError(t, db.First(&fd, "user_id = ?", u2.ID).Error)
	assert.Equal(t, loser.VCardUID, fd.EntityID)
}
