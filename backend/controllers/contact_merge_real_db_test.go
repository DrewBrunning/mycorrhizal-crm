package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactMerge_RealMigratedSchema is the real-DB check for ticket N1
// (N1): every other test in this
// package uses AutoMigrate against:memory: sqlite, which cannot catch a
// GORM column-tag mismatch against the real migration SQL. This test seeds a
// keeper (Alice) and loser (Bob) plus a third contact (Carol) covering every
// merge case in one pass, then exercises preview -> commit against a
// database.InitDB-migrated real file database, and finally the missing-
// resolution rejection path.
func TestContactMerge_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)

	user := models.User{Username: "mergetester", Password: "password123!A", Email: "merge@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := models.Contact{
		UserID: user.ID, Firstname: "Alice",
		Emails: []models.ContactEmail{{Type: "home", Value: "alice@home.example"}},
	}
	bob := models.Contact{
		UserID: user.ID, Firstname: "Robert", Lastname: "Smith",
		Emails: []models.ContactEmail{
			{Type: "home", Value: "alice@home.example"}, // duplicate of Alice's -- must dedup
			{Type: "work", Value: "bob@work.example"},   // unique -- must survive the union
		},
	}
	carol := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	require.NoError(t, db.Create(&carol).Error)

	// Notes / reminders / reminder completions on Bob.
	note := models.Note{UserID: user.ID, ContactID: &bob.ID, Content: "a note about bob"}
	require.NoError(t, db.Create(&note).Error)
	reminder := models.Reminder{UserID: user.ID, ContactID: &bob.ID, Message: "follow up", Recurrence: "once", RemindAt: time.Now().Add(24 * time.Hour)}
	require.NoError(t, db.Create(&reminder).Error)
	completion := models.ReminderCompletion{UserID: user.ID, ContactID: bob.ID, ReminderID: &reminder.ID, Message: "done", CompletedAt: time.Now()}
	require.NoError(t, db.Create(&completion).Error)

	// Activities: one shared (dedup case), one Bob-only (plain repoint).
	sharedActivity := models.Activity{UserID: user.ID, Title: "Dinner", Contacts: []models.Contact{alice, bob}}
	require.NoError(t, db.Create(&sharedActivity).Error)
	bobOnlyActivity := models.Activity{UserID: user.ID, Title: "Coffee", Contacts: []models.Contact{bob}}
	require.NoError(t, db.Create(&bobOnlyActivity).Error)

	// Relationship edges: a direct Alice<->Bob edge (becomes a self-loop),
	// and an inverse pair through Carol (exactly one must survive).
	directEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: bob.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&directEdge).Error)
	bobParentEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: bob.VCardUID, TargetID: carol.VCardUID, Type: "parent_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&bobParentEdge).Error)
	// The same fact as bobParentEdge ("bob parent_of carol", which becomes
	// "alice parent_of carol" after repoint), recorded from carol's side
	// instead: "carol child_of alice" -- source/target swapped, type
	// inverted. This is the genuine inverse-pair collision the dedup logic
	// must catch, distinct from two edges that merely disagree.
	carolChildEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: carol.VCardUID, TargetID: alice.VCardUID, Type: "child_of",
		Source: models.RelationshipSourceHouseholdInferred, Confidence: 0.5, Status: models.RelationshipStatusSuggested,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&carolChildEdge).Error)

	// Household: Alice and Bob both already members (duplicate case).
	household := models.Household{UserID: user.ID, Name: "The House", Type: models.HouseholdTypeRoommates}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: alice.VCardUID}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: bob.VCardUID}).Error)

	// Circle / Tag: Bob-only (plain repoint).
	circle := models.Circle{UserID: user.ID, Name: "College Friends"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: bob.VCardUID}).Error)
	tag := models.Tag{UserID: user.ID, Name: "vip"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: user.ID, ContactVCardUID: bob.VCardUID}).Error)

	// Custom field values: one Bob-only (plain repoint), one on both
	// (collision -- must appear as a conflict).
	bobOnlyDef := models.FieldDefinition{UserID: user.ID, Label: "Favorite Color", Key: "favorite_color", Target: "contact", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, db.Create(&bobOnlyDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: bobOnlyDef.ID, UserID: user.ID, EntityID: bob.VCardUID, Value: json.RawMessage(`"blue"`)}).Error)

	sharedDef := models.FieldDefinition{UserID: user.ID, Label: "T-Shirt Size", Key: "tshirt_size", Target: "contact", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, db.Create(&sharedDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: sharedDef.ID, UserID: user.ID, EntityID: alice.VCardUID, Value: json.RawMessage(`"M"`)}).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: sharedDef.ID, UserID: user.ID, EntityID: bob.VCardUID, Value: json.RawMessage(`"L"`)}).Error)

	// Life events: Bob's own (repoint), and Carol's referencing Bob as a
	// secondary participant (the RelatedEntityIDs JSON-array rewrite case).
	bobLifeEvent := models.LifeEvent{UserID: user.ID, EntityID: bob.VCardUID, Type: "moved"}
	require.NoError(t, db.Create(&bobLifeEvent).Error)
	carolLifeEvent := models.LifeEvent{UserID: user.ID, EntityID: carol.VCardUID, Type: "married", RelatedEntityIDs: []string{bob.VCardUID}}
	require.NoError(t, db.Create(&carolLifeEvent).Error)

	// A conversation agenda item keyed to Bob (must be repointed, not
	// deleted, on merge).
	bobAgendaItem := models.ConversationAgenda{UserID: user.ID, EntityID: bob.VCardUID, Content: "Ask about the move"}
	require.NoError(t, db.Create(&bobAgendaItem).Error)

	// A gift record keyed to Bob (must be repointed, not deleted, on merge).
	bobGift := models.Gift{UserID: user.ID, EntityID: bob.VCardUID, Description: "A gift idea for Bob"}
	require.NoError(t, db.Create(&bobGift).Error)

	// ContactSyncLink on Bob (must be discarded, not repointed).
	subscription := models.ContactSubscription{UserID: user.ID, Name: "Test CardDAV", URL: "https://example.com/dav"}
	require.NoError(t, db.Create(&subscription).Error)
	syncLink := models.ContactSyncLink{SubscriptionID: subscription.ID, UserID: user.ID, Href: "/dav/bob.vcf", ContactID: bob.ID, ContentHash: "abc123"}
	require.NoError(t, db.Create(&syncLink).Error)

	// T107: the five association types that previously fell through
	// RepointContactAssociations straight into deleteContactAssociations'
	// delete calls, silently destroyed on every merge. All Bob-only (no
	// conflict) here -- the conflict/dedupe cases get their own dedicated
	// tests below.
	bobAttachment := models.Attachment{UserID: user.ID, ContactVCardUID: bob.VCardUID, StoredName: "stored-bob-1", OriginalName: "resume.pdf", ContentType: "application/pdf", SizeBytes: 1024}
	require.NoError(t, db.Create(&bobAttachment).Error)
	bobPreference := models.Preference{UserID: user.ID, EntityID: bob.VCardUID, Category: "food", Value: "sushi"}
	require.NoError(t, db.Create(&bobPreference).Error)
	bobCadence := models.CadencePolicy{UserID: user.ID, EntityID: bob.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&bobCadence).Error)
	bobIdentity := models.ExternalIdentity{UserID: user.ID, EntityID: bob.VCardUID, System: "immich", ExternalID: "person-bob"}
	require.NoError(t, db.Create(&bobIdentity).Error)
	bobActivity := models.ExternalActivity{UserID: user.ID, EntityID: bob.VCardUID, SourceSystem: "immich", ExternalID: "asset-bob", Type: "photo-appearance", OccurredAt: time.Now()}
	require.NoError(t, db.Create(&bobActivity).Error)

	// NotificationDelivery is keyed only by ReminderID, not by contact -- it
	// must survive untouched once its owning Reminder is repointed (T107
	// closes out every table deleteContactAssociations touches; this is the
	// one that's neither repointed nor dropped by name, since it rides along
	// with the Reminder).
	notificationDelivery := models.NotificationDelivery{ReminderID: reminder.ID, Channel: "email", Status: "pending"}
	require.NoError(t, db.Create(&notificationDelivery).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// --- Preview ---
	previewResp := doJSON("POST", "/contacts/merge/preview", models.ContactMergeRequest{KeepID: alice.ID, MergeID: bob.ID})
	require.Equal(t, http.StatusOK, previewResp.Code, previewResp.Body.String())
	var preview models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewResp.Body.Bytes(), &preview))

	require.Len(t, preview.Resolution.Conflicts, 1, "firstname must be the only scalar conflict")
	assert.Equal(t, "firstname", preview.Resolution.Conflicts[0].Field)
	assert.Equal(t, "Alice", preview.Resolution.Conflicts[0].KeeperValue)
	assert.Equal(t, "Robert", preview.Resolution.Conflicts[0].LoserValue)
	assert.Equal(t, "Smith", preview.Resolution.ResolvedScalars["lastname"], "lastname only set on the loser must be auto-resolved")

	require.Len(t, preview.Resolution.Emails, 2, "duplicate email deduped, unique email unioned")

	require.Len(t, preview.Resolution.FieldValueConflicts, 1, "the shared custom field must be a conflict")
	assert.Equal(t, sharedDef.ID, preview.Resolution.FieldValueConflicts[0].Field)
	assert.Equal(t, `"M"`, preview.Resolution.FieldValueConflicts[0].KeeperValue)
	assert.Equal(t, `"L"`, preview.Resolution.FieldValueConflicts[0].LoserValue)

	counts := preview.AssociationCounts
	assert.EqualValues(t, 1, counts.Notes)
	assert.EqualValues(t, 2, counts.Activities)
	assert.EqualValues(t, 1, counts.Reminders)
	assert.EqualValues(t, 1, counts.ReminderCompletions)
	assert.EqualValues(t, 2, counts.RelationshipEdges)
	assert.EqualValues(t, 1, counts.HouseholdMemberships)
	assert.EqualValues(t, 1, counts.CircleMemberships)
	assert.EqualValues(t, 1, counts.Tags)
	assert.EqualValues(t, 1, counts.LifeEvents)
	assert.EqualValues(t, 1, counts.ConversationAgendaItems)
	assert.EqualValues(t, 1, counts.GiftItems)
	assert.EqualValues(t, 1, counts.LifeEventReferences)
	assert.EqualValues(t, 2, counts.FieldValues)
	assert.EqualValues(t, 1, counts.ContactSyncLinks)
	assert.EqualValues(t, 1, counts.Attachments)
	assert.EqualValues(t, 1, counts.Preferences)
	assert.EqualValues(t, 1, counts.ExternalIdentities)
	assert.EqualValues(t, 1, counts.ExternalActivities)
	assert.EqualValues(t, 1, counts.CadencePolicies)

	// --- Commit without a resolution for the known conflicts: rejected, no partial merge ---
	rejectResp := doJSON("POST", "/contacts/merge", models.ContactMergeRequest{KeepID: alice.ID, MergeID: bob.ID})
	require.Equal(t, http.StatusBadRequest, rejectResp.Code, rejectResp.Body.String())
	assert.Contains(t, rejectResp.Body.String(), "firstname")

	var untouchedAlice models.Contact
	require.NoError(t, db.First(&untouchedAlice, alice.ID).Error)
	assert.Equal(t, "Alice", untouchedAlice.Firstname, "a rejected commit must not have modified the keeper")
	var untouchedBobCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", bob.ID).Count(&untouchedBobCount).Error)
	assert.EqualValues(t, 1, untouchedBobCount, "a rejected commit must not have soft-deleted the loser")

	// --- Commit with both conflicts resolved ---
	commitResp := doJSON("POST", "/contacts/merge", models.ContactMergeRequest{
		KeepID: alice.ID, MergeID: bob.ID,
		Resolutions: map[string]string{"firstname": "Alice", sharedDef.ID: `"L"`},
	})
	require.Equal(t, http.StatusOK, commitResp.Code, commitResp.Body.String())

	// Keeper's fields.
	var keeper models.Contact
	require.NoError(t, db.First(&keeper, alice.ID).Error)
	assert.Equal(t, "Alice", keeper.Firstname)
	assert.Equal(t, "Smith", keeper.Lastname)
	assert.Len(t, keeper.Emails, 2)

	// Loser soft-deleted, not hard-deleted.
	var scopedCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", bob.ID).Count(&scopedCount).Error)
	assert.EqualValues(t, 0, scopedCount, "plain query must not see the soft-deleted loser")
	var unscopedCount int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", bob.ID).Count(&unscopedCount).Error)
	assert.EqualValues(t, 1, unscopedCount, "Unscoped query must still find the soft-deleted loser")

	// Notes / reminders / reminder completions re-pointed, zero orphans.
	assertZero := func(model any, cond string, args ...any) {
		t.Helper()
		var count int64
		require.NoError(t, db.Model(model).Where(cond, args...).Count(&count).Error)
		assert.EqualValues(t, 0, count, "orphan rows still reference the loser: %v", cond)
	}
	assertZero(&models.Note{}, "contact_id = ?", bob.ID)
	assertZero(&models.Reminder{}, "contact_id = ?", bob.ID)
	assertZero(&models.ReminderCompletion{}, "contact_id = ?", bob.ID)
	assertZero(&models.HouseholdMember{}, "member_vcard_uid = ?", bob.VCardUID)
	assertZero(&models.CircleMember{}, "member_vcard_uid = ?", bob.VCardUID)
	assertZero(&models.ContactTag{}, "contact_vcard_uid = ?", bob.VCardUID)
	assertZero(&models.FieldValue{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.LifeEvent{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.Gift{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.ContactSyncLink{}, "contact_id = ?", bob.ID)
	assertZero(&models.RelationshipEdge{}, "source_id = ? OR target_id = ?", bob.VCardUID, bob.VCardUID)
	assertZero(&models.Attachment{}, "contact_vcard_uid = ?", bob.VCardUID)
	assertZero(&models.Preference{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.CadencePolicy{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.ExternalIdentity{}, "entity_id = ?", bob.VCardUID)
	assertZero(&models.ExternalActivity{}, "entity_id = ?", bob.VCardUID)

	// T107: all five re-pointed onto Alice, not destroyed.
	var repointedAttachment models.Attachment
	require.NoError(t, db.Where("id = ?", bobAttachment.ID).First(&repointedAttachment).Error)
	assert.Equal(t, alice.VCardUID, repointedAttachment.ContactVCardUID)

	var repointedPreference models.Preference
	require.NoError(t, db.Where("id = ?", bobPreference.ID).First(&repointedPreference).Error)
	assert.Equal(t, alice.VCardUID, repointedPreference.EntityID)

	var adoptedCadence models.CadencePolicy
	require.NoError(t, db.Where("id = ?", bobCadence.ID).First(&adoptedCadence).Error)
	assert.Equal(t, alice.VCardUID, adoptedCadence.EntityID, "Alice had no cadence policy of her own -- Bob's must be adopted silently")
	assert.Equal(t, 30, adoptedCadence.TargetIntervalDays)

	var repointedIdentity models.ExternalIdentity
	require.NoError(t, db.Where("id = ?", bobIdentity.ID).First(&repointedIdentity).Error)
	assert.Equal(t, alice.VCardUID, repointedIdentity.EntityID)

	var repointedActivity models.ExternalActivity
	require.NoError(t, db.Where("id = ?", bobActivity.ID).First(&repointedActivity).Error)
	assert.Equal(t, alice.VCardUID, repointedActivity.EntityID)

	// NotificationDelivery rides along with its Reminder -- untouched by the
	// merge, not swept up by deleteContactAssociations' ContactSyncLink-only
	// cleanup pass.
	var survivingDelivery models.NotificationDelivery
	require.NoError(t, db.Where("id = ?", notificationDelivery.ID).First(&survivingDelivery).Error)
	assert.Equal(t, reminder.ID, survivingDelivery.ReminderID)

	var notesForKeeper, remindersForKeeper int64
	require.NoError(t, db.Model(&models.Note{}).Where("contact_id = ?", alice.ID).Count(&notesForKeeper).Error)
	require.NoError(t, db.Model(&models.Reminder{}).Where("contact_id = ?", alice.ID).Count(&remindersForKeeper).Error)
	assert.EqualValues(t, 2, notesForKeeper, "Bob's repointed note plus the merge audit note")
	assert.EqualValues(t, 1, remindersForKeeper)

	// activity_contacts: shared activity has exactly one row for Alice (no
	// PK violation, no duplicate); Bob-only activity now points to Alice.
	var sharedActivityRows, bobOnlyActivityRows int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM activity_contacts WHERE activity_id = ? AND contact_id = ?", sharedActivity.ID, alice.ID).Scan(&sharedActivityRows).Error)
	assert.EqualValues(t, 1, sharedActivityRows)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM activity_contacts WHERE activity_id = ? AND contact_id = ?", bobOnlyActivity.ID, alice.ID).Scan(&bobOnlyActivityRows).Error)
	assert.EqualValues(t, 1, bobOnlyActivityRows)
	var totalActivityRows int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM activity_contacts WHERE activity_id = ? AND contact_id = ?", sharedActivity.ID, bob.ID).Scan(&totalActivityRows).Error)
	assert.EqualValues(t, 0, totalActivityRows)

	// Household membership deduped: exactly one row for Alice, not two.
	var householdRows int64
	require.NoError(t, db.Model(&models.HouseholdMember{}).Where("household_id = ? AND member_vcard_uid = ?", household.ID, alice.VCardUID).Count(&householdRows).Error)
	assert.EqualValues(t, 1, householdRows)

	// Circle/Tag re-pointed to Alice.
	var circleRows, tagRows int64
	require.NoError(t, db.Model(&models.CircleMember{}).Where("circle_id = ? AND member_vcard_uid = ?", circle.ID, alice.VCardUID).Count(&circleRows).Error)
	assert.EqualValues(t, 1, circleRows)
	require.NoError(t, db.Model(&models.ContactTag{}).Where("tag_id = ? AND contact_vcard_uid = ?", tag.ID, alice.VCardUID).Count(&tagRows).Error)
	assert.EqualValues(t, 1, tagRows)

	// FieldValue: Bob-only value re-pointed; the colliding one resolved to
	// the chosen value ("L"), not silently kept as Alice's original ("M").
	var bobOnlyValue, sharedValue models.FieldValue
	require.NoError(t, db.Where("field_definition_id = ? AND entity_id = ?", bobOnlyDef.ID, alice.VCardUID).First(&bobOnlyValue).Error)
	assert.JSONEq(t, `"blue"`, string(bobOnlyValue.Value))
	require.NoError(t, db.Where("field_definition_id = ? AND entity_id = ?", sharedDef.ID, alice.VCardUID).First(&sharedValue).Error)
	assert.JSONEq(t, `"L"`, string(sharedValue.Value), "the resolved conflict value must win, not the keeper's original")
	var sharedValueCount int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("field_definition_id = ?", sharedDef.ID).Count(&sharedValueCount).Error)
	assert.EqualValues(t, 1, sharedValueCount, "the collision must leave exactly one row, not two")

	// LifeEvent: Bob's own subject re-pointed; Carol's RelatedEntityIDs
	// rewritten from Bob's VCardUID to Alice's.
	var repointedLifeEvent models.LifeEvent
	require.NoError(t, db.Where("id = ?", bobLifeEvent.ID).First(&repointedLifeEvent).Error)
	assert.Equal(t, alice.VCardUID, repointedLifeEvent.EntityID)

	var reloadedCarolEvent models.LifeEvent
	require.NoError(t, db.Where("id = ?", carolLifeEvent.ID).First(&reloadedCarolEvent).Error)
	assert.Equal(t, []string{alice.VCardUID}, reloadedCarolEvent.RelatedEntityIDs)

	// ConversationAgenda: Bob's item follows the surviving contact, not
	// deleted with the loser.
	var repointedAgenda models.ConversationAgenda
	require.NoError(t, db.Where("id = ?", bobAgendaItem.ID).First(&repointedAgenda).Error)
	assert.Equal(t, alice.VCardUID, repointedAgenda.EntityID)

	// Gift: Bob's record follows the surviving contact, not deleted with the
	// loser.
	var repointedGift models.Gift
	require.NoError(t, db.Where("id = ?", bobGift.ID).First(&repointedGift).Error)
	assert.Equal(t, alice.VCardUID, repointedGift.EntityID)

	// RelationshipEdge: self-loop dropped entirely; of the inverse pair,
	// exactly one survives -- and per the tie-break (higher Confidence /
	// confirmed status) it must be the one that started as Bob's parent_of.
	var selfLoopCount int64
	require.NoError(t, db.Model(&models.RelationshipEdge{}).
		Where("user_id = ? AND source_id = ? AND target_id = ?", user.ID, alice.VCardUID, alice.VCardUID).
		Count(&selfLoopCount).Error)
	assert.EqualValues(t, 0, selfLoopCount, "the direct Alice<->Bob edge must become a self-loop and be dropped")

	var aliceCarolEdges []models.RelationshipEdge
	require.NoError(t, db.Where(
		"user_id = ? AND ((source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?))",
		user.ID, alice.VCardUID, carol.VCardUID, carol.VCardUID, alice.VCardUID,
	).Find(&aliceCarolEdges).Error)
	require.Len(t, aliceCarolEdges, 1, "exactly one of the inverse pair must survive")
	assert.Equal(t, "parent_of", aliceCarolEdges[0].Type, "the higher-confidence, confirmed edge must be the survivor")
	assert.Equal(t, alice.VCardUID, aliceCarolEdges[0].SourceID)
	assert.Equal(t, models.RelationshipStatusConfirmed, aliceCarolEdges[0].Status)

	// Audit note exists on the keeper and names the merge.
	var mergeNote models.Note
	require.NoError(t, db.Where("contact_id = ? AND content LIKE ?", alice.ID, "%Merged contact%").First(&mergeNote).Error)
	assert.Contains(t, mergeNote.Content, "Robert")
}

// TestContactMerge_SharedCircleAndTag_Deduped covers the branch
// TestContactMerge_RealMigratedSchema does not: there, only the loser holds
// the circle/tag membership, so the plain UPDATE half of the repoint is
// exercised and the DELETE-the-loser's-duplicate half never runs. Here both
// contacts are in the same circle and carry the same tag, which is the case
// that produced the T95 beta report ("merge doesn't carry forward circles").
// The backend was never the bug -- but nothing pinned the dedup path, so a
// frontend refresh bug was indistinguishable from a backend data-loss bug.
//
// Both tables hard-delete (/CLAUDE.md trap #7) and both are counted with a
// plain Count() below, which is safe precisely because there is no soft-delete
// to hide behind.
func TestContactMerge_SharedCircleAndTag_Deduped(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-shared-membership.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)

	user := models.User{Username: "dedupetester", Password: "password123!A", Email: "dedupe@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	// shared: both are members -> the loser's row must be dropped, not moved.
	shared := models.Circle{UserID: user.ID, Name: "Book Club"}
	require.NoError(t, db.Create(&shared).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: shared.ID, UserID: user.ID, MemberVCardUID: alice.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: shared.ID, UserID: user.ID, MemberVCardUID: bob.VCardUID}).Error)

	// loserOnly: only Bob is a member -> the row must move to Alice. Asserted
	// alongside the shared case so a fix that deletes instead of repointing
	// cannot pass this test.
	loserOnly := models.Circle{UserID: user.ID, Name: "Climbing Gym"}
	require.NoError(t, db.Create(&loserOnly).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: loserOnly.ID, UserID: user.ID, MemberVCardUID: bob.VCardUID}).Error)

	sharedTag := models.Tag{UserID: user.ID, Name: "neighbor"}
	require.NoError(t, db.Create(&sharedTag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: sharedTag.ID, UserID: user.ID, ContactVCardUID: alice.VCardUID}).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: sharedTag.ID, UserID: user.ID, ContactVCardUID: bob.VCardUID}).Error)

	loserOnlyTag := models.Tag{UserID: user.ID, Name: "climbing"}
	require.NoError(t, db.Create(&loserOnlyTag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: loserOnlyTag.ID, UserID: user.ID, ContactVCardUID: bob.VCardUID}).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(models.ContactMergeRequest{
		KeepID: alice.ID, MergeID: bob.ID,
		// Both have a firstname, so it is a scalar conflict the commit path
		// refuses to guess at -- unrelated to what this test is about.
		Resolutions: map[string]string{"firstname": "Alice"},
	}))
	req, _ := http.NewRequest("POST", "/contacts/merge", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	countRows := func(model any, where string, args ...any) int64 {
		var n int64
		require.NoError(t, db.Model(model).Where(where, args...).Count(&n).Error)
		return n
	}

	// The shared membership collapses to exactly one row for the keeper --
	// not two (no dedup) and not zero (dedup deleted the survivor too).
	assert.EqualValues(t, 1, countRows(&models.CircleMember{}, "circle_id = ? AND member_vcard_uid = ?", shared.ID, alice.VCardUID))
	assert.EqualValues(t, 1, countRows(&models.CircleMember{}, "circle_id = ? AND member_vcard_uid = ?", loserOnly.ID, alice.VCardUID))
	assert.EqualValues(t, 1, countRows(&models.ContactTag{}, "tag_id = ? AND contact_vcard_uid = ?", sharedTag.ID, alice.VCardUID))
	assert.EqualValues(t, 1, countRows(&models.ContactTag{}, "tag_id = ? AND contact_vcard_uid = ?", loserOnlyTag.ID, alice.VCardUID))

	// Nothing is left pointing at the deleted loser.
	assert.EqualValues(t, 0, countRows(&models.CircleMember{}, "member_vcard_uid = ?", bob.VCardUID))
	assert.EqualValues(t, 0, countRows(&models.ContactTag{}, "contact_vcard_uid = ?", bob.VCardUID))

	// The keeper ends up in both circles and carries both tags -- the claim
	// the T95 report disputed, stated as the user would experience it.
	assert.EqualValues(t, 2, countRows(&models.CircleMember{}, "member_vcard_uid = ?", alice.VCardUID))
	assert.EqualValues(t, 2, countRows(&models.ContactTag{}, "contact_vcard_uid = ?", alice.VCardUID))
}

// TestContactMerge_KeepEqualsMerge_Rejected is a lightweight guard against
// the trivial self-merge case, on AutoMigrate (no real-DB dependency needed
// for this check).
func TestContactMerge_KeepEqualsMerge_Rejected(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Solo"}
	require.NoError(t, db.Create(&contact).Error)

	router.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(models.ContactMergeRequest{KeepID: contact.ID, MergeID: contact.ID}))
	req, _ := http.NewRequest("POST", "/contacts/merge/preview", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestContactMerge_CadencePolicyConflict covers T107's new conflict type:
// unlike every other association CadencePolicy can't be unioned or plain
// re-pointed (migration 000002's partial unique index on (user_id,
// entity_id) allows only one per contact). Two independent pairs in one
// test: pair A shows the commit is rejected until resolved, same as any
// other conflict; pair B resolves it toward the loser's policy and confirms
// the keeper actually ends up with the loser's values, not silently kept.
func TestContactMerge_CadencePolicyConflict(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-cadence-conflict.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)

	user := models.User{Username: "cadencetester", Password: "password123!A", Email: "cadence@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	doJSON := func(path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
		req, _ := http.NewRequest("POST", path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// --- Pair A: rejected until resolved ---
	aliceA := models.Contact{UserID: user.ID, Firstname: "AliceA"}
	bobA := models.Contact{UserID: user.ID, Firstname: "AliceA"} // same firstname: no scalar conflict to muddy this test
	require.NoError(t, db.Create(&aliceA).Error)
	require.NoError(t, db.Create(&bobA).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: aliceA.VCardUID, TargetIntervalDays: 30}).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: bobA.VCardUID, TargetIntervalDays: 90}).Error)

	previewA := doJSON("/contacts/merge/preview", models.ContactMergeRequest{KeepID: aliceA.ID, MergeID: bobA.ID})
	require.Equal(t, http.StatusOK, previewA.Code, previewA.Body.String())
	var resolutionA models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewA.Body.Bytes(), &resolutionA))
	require.Len(t, resolutionA.Resolution.Conflicts, 1, "differing cadence policies on both sides must surface as exactly one conflict")
	assert.Equal(t, "cadence_policy", resolutionA.Resolution.Conflicts[0].Field)
	assert.Equal(t, "Every 30 days", resolutionA.Resolution.Conflicts[0].KeeperValue)
	assert.Equal(t, "Every 90 days", resolutionA.Resolution.Conflicts[0].LoserValue)

	rejectA := doJSON("/contacts/merge", models.ContactMergeRequest{KeepID: aliceA.ID, MergeID: bobA.ID})
	require.Equal(t, http.StatusBadRequest, rejectA.Code, rejectA.Body.String())
	assert.Contains(t, rejectA.Body.String(), "cadence_policy")

	// --- Pair B: resolved toward the loser's policy ---
	aliceB := models.Contact{UserID: user.ID, Firstname: "AliceB"}
	bobB := models.Contact{UserID: user.ID, Firstname: "AliceB"}
	require.NoError(t, db.Create(&aliceB).Error)
	require.NoError(t, db.Create(&bobB).Error)
	keeperPolicy := models.CadencePolicy{UserID: user.ID, EntityID: aliceB.VCardUID, TargetIntervalDays: 14}
	require.NoError(t, db.Create(&keeperPolicy).Error)
	loserPolicy := models.CadencePolicy{UserID: user.ID, EntityID: bobB.VCardUID, TargetIntervalDays: 60, QualifyingTypes: []string{"call", "visit"}}
	require.NoError(t, db.Create(&loserPolicy).Error)

	commitB := doJSON("/contacts/merge", models.ContactMergeRequest{
		KeepID: aliceB.ID, MergeID: bobB.ID,
		Resolutions: map[string]string{"cadence_policy": "Every 60 days (call, visit)"},
	})
	require.Equal(t, http.StatusOK, commitB.Code, commitB.Body.String())

	var survivingPolicy models.CadencePolicy
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", aliceB.VCardUID, user.ID).First(&survivingPolicy).Error)
	assert.Equal(t, 60, survivingPolicy.TargetIntervalDays, "the chosen (loser's) policy must win, not the keeper's original")
	assert.Equal(t, []string{"call", "visit"}, survivingPolicy.QualifyingTypes)
	assert.Equal(t, loserPolicy.ID, survivingPolicy.ID, "the surviving row must be the loser's re-pointed, not a new one")

	var keeperPolicyGone int64
	require.NoError(t, db.Model(&models.CadencePolicy{}).Where("id = ?", keeperPolicy.ID).Count(&keeperPolicyGone).Error)
	assert.EqualValues(t, 0, keeperPolicyGone, "the keeper's original (unchosen) policy must be gone, not left behind as a duplicate")

	// --- Pair C: a resolution value that matches neither side is rejected,
	// not silently treated as "keeper wins". This is the stale-preview case
	// -- e.g. the loser's policy was edited between preview and commit, so
	// the client's echoed-back resolution no longer matches either side's
	// freshly-recomputed summary. Picking wrong here silently discards
	// which whole row survives, unlike a scalar conflict where a bad value
	// just writes an odd string -- so this must be a hard rejection, the
	// same way an unresolved conflict already is.
	aliceC := models.Contact{UserID: user.ID, Firstname: "AliceC"}
	bobC := models.Contact{UserID: user.ID, Firstname: "AliceC"}
	require.NoError(t, db.Create(&aliceC).Error)
	require.NoError(t, db.Create(&bobC).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: aliceC.VCardUID, TargetIntervalDays: 7}).Error)
	loserPolicyC := models.CadencePolicy{UserID: user.ID, EntityID: bobC.VCardUID, TargetIntervalDays: 21}
	require.NoError(t, db.Create(&loserPolicyC).Error)

	commitC := doJSON("/contacts/merge", models.ContactMergeRequest{
		KeepID: aliceC.ID, MergeID: bobC.ID,
		// Stale: doesn't match "Every 7 days" (keeper) or "Every 21 days"
		// (loser) -- e.g. echoed back from a preview taken before the
		// loser's policy was edited to 21 days.
		Resolutions: map[string]string{"cadence_policy": "Every 14 days"},
	})
	require.Equal(t, http.StatusInternalServerError, commitC.Code, commitC.Body.String())

	var aliceCPolicy, bobCPolicy models.CadencePolicy
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", aliceC.VCardUID, user.ID).First(&aliceCPolicy).Error)
	assert.Equal(t, 7, aliceCPolicy.TargetIntervalDays, "a rejected commit must not have touched the keeper's policy")
	require.NoError(t, db.Where("id = ?", loserPolicyC.ID).First(&bobCPolicy).Error)
	assert.Equal(t, 21, bobCPolicy.TargetIntervalDays, "a rejected commit must not have dropped the loser's policy either")

	// --- Pair D: QualifyingTypes in a different order must not be treated
	// as a conflict -- it's a JSON array with no canonical order, so the
	// same set of types written in a different sequence is the same policy.
	aliceD := models.Contact{UserID: user.ID, Firstname: "AliceD"}
	bobD := models.Contact{UserID: user.ID, Firstname: "AliceD"}
	require.NoError(t, db.Create(&aliceD).Error)
	require.NoError(t, db.Create(&bobD).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: aliceD.VCardUID, TargetIntervalDays: 30, QualifyingTypes: []string{"call", "visit"}}).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: bobD.VCardUID, TargetIntervalDays: 30, QualifyingTypes: []string{"visit", "call"}}).Error)

	previewD := doJSON("/contacts/merge/preview", models.ContactMergeRequest{KeepID: aliceD.ID, MergeID: bobD.ID})
	require.Equal(t, http.StatusOK, previewD.Code, previewD.Body.String())
	var resolutionD models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewD.Body.Bytes(), &resolutionD))
	assert.Empty(t, resolutionD.Resolution.Conflicts, "same interval and same qualifying types in a different order must not surface as a conflict")

	commitD := doJSON("/contacts/merge", models.ContactMergeRequest{KeepID: aliceD.ID, MergeID: bobD.ID})
	require.Equal(t, http.StatusOK, commitD.Code, commitD.Body.String())
}

// TestContactMerge_ExternalIdentityAndActivityPlainRepoint covers T107's
// external_identities/external_activities repoint. Their unique index --
// (system, external_id, user_id) / (source_system, external_id, user_id) --
// does not include entity_id, which unique-constrains across the whole user
// rather than per-contact: two contacts belonging to the same user can never
// separately hold the same (system, external_id) pair to begin with (proven
// while writing this test -- attempting to seed exactly that as a fixture
// fails at Create with a UNIQUE constraint error, before a merge is even
// involved), so there is no dedupe case to cover here, only a plain repoint.
func TestContactMerge_ExternalIdentityAndActivityPlainRepoint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-external-repoint.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)

	user := models.User{Username: "externalrepointtester", Password: "password123!A", Email: "extrepoint@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	require.NoError(t, db.Create(&models.ExternalIdentity{UserID: user.ID, EntityID: bob.VCardUID, System: "paperless", ExternalID: "doc-bob-only"}).Error)
	require.NoError(t, db.Create(&models.ExternalActivity{UserID: user.ID, EntityID: bob.VCardUID, SourceSystem: "immich", ExternalID: "asset-bob-only", Type: "photo-appearance", OccurredAt: time.Now()}).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(models.ContactMergeRequest{KeepID: alice.ID, MergeID: bob.ID}))
	req, _ := http.NewRequest("POST", "/contacts/merge", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	countRows := func(model any, where string, args ...any) int64 {
		var n int64
		require.NoError(t, db.Model(model).Where(where, args...).Count(&n).Error)
		return n
	}

	assert.EqualValues(t, 1, countRows(&models.ExternalIdentity{}, "entity_id = ? AND system = ? AND external_id = ?", alice.VCardUID, "paperless", "doc-bob-only"))
	assert.EqualValues(t, 1, countRows(&models.ExternalActivity{}, "entity_id = ? AND source_system = ? AND external_id = ?", alice.VCardUID, "immich", "asset-bob-only"))
	assert.EqualValues(t, 0, countRows(&models.ExternalIdentity{}, "entity_id = ?", bob.VCardUID))
	assert.EqualValues(t, 0, countRows(&models.ExternalActivity{}, "entity_id = ?", bob.VCardUID))
}

// TestContactMerge_PhotoAdoptionAndDiscard covers T107's photo rule: adopt
// the loser's photo when the keeper has none of its own, discard it
// (existing, correct behaviour) when the keeper already has one. Real files
// on disk via cfg.ProfilePhotoDir, since the assertion that matters is the
// file surviving (or not) on disk, not just the Contact.Photo string.
func TestContactMerge_PhotoAdoptionAndDiscard(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-photo.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)
	photoDir := t.TempDir()

	user := models.User{Username: "phototester", Password: "password123!A", Email: "photo@example.com"}
	require.NoError(t, db.Create(&user).Error)

	writePhoto := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(photoDir, name), []byte("fake-jpeg-bytes"), 0o644))
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(photoDir, name))
		return err == nil
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: photoDir})
		c.Next()
	})
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	doMerge := func(keepID, mergeID uint) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(models.ContactMergeRequest{KeepID: keepID, MergeID: mergeID}))
		req, _ := http.NewRequest("POST", "/contacts/merge", &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// --- Adoption: keeper has no photo, loser does ---
	writePhoto("loser-adopt.jpg")
	aliceNoPhoto := models.Contact{UserID: user.ID, Firstname: "AliceNoPhoto"}
	bobWithPhoto := models.Contact{UserID: user.ID, Firstname: "AliceNoPhoto", Photo: "loser-adopt.jpg", PhotoThumbnail: "data:image/jpeg;base64,Zm9v"}
	require.NoError(t, db.Create(&aliceNoPhoto).Error)
	require.NoError(t, db.Create(&bobWithPhoto).Error)

	w := doMerge(aliceNoPhoto.ID, bobWithPhoto.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var adopted models.Contact
	require.NoError(t, db.First(&adopted, aliceNoPhoto.ID).Error)
	assert.Equal(t, "loser-adopt.jpg", adopted.Photo, "the keeper must adopt the loser's photo when it had none")
	assert.Equal(t, "data:image/jpeg;base64,Zm9v", adopted.PhotoThumbnail)
	assert.True(t, exists("loser-adopt.jpg"), "the adopted photo file must survive on disk, not be deleted out from under the keeper")

	// --- Discard: both have a photo, keeper's own must survive untouched ---
	writePhoto("keeper-own.jpg")
	writePhoto("loser-discarded.jpg")
	aliceWithPhoto := models.Contact{UserID: user.ID, Firstname: "AliceWithPhoto", Photo: "keeper-own.jpg"}
	bobAlsoPhoto := models.Contact{UserID: user.ID, Firstname: "AliceWithPhoto", Photo: "loser-discarded.jpg"}
	require.NoError(t, db.Create(&aliceWithPhoto).Error)
	require.NoError(t, db.Create(&bobAlsoPhoto).Error)

	w2 := doMerge(aliceWithPhoto.ID, bobAlsoPhoto.ID)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var discarded models.Contact
	require.NoError(t, db.First(&discarded, aliceWithPhoto.ID).Error)
	assert.Equal(t, "keeper-own.jpg", discarded.Photo, "a keeper that already has a photo must keep its own, not the loser's")
	assert.True(t, exists("keeper-own.jpg"), "the keeper's own photo file must survive")
	assert.False(t, exists("loser-discarded.jpg"), "the loser's discarded photo file must actually be removed from disk")
}
