package controllers

import (
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetContactDetail_EmptyBlocksSerializeAsArrays pins the M4 composite's
// most likely regression (per the ticket's own traps section): every
// collection block must serialize as `[]`, never `null`/absent, for a
// brand-new contact with no data. Assert on raw JSON — decoding into the Go
// struct makes "absent" and `[]` indistinguishable.
func TestGetContactDetail_EmptyBlocksSerializeAsArrays(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/detail", GetContactDetail)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	for _, key := range []string{
		"notes", "activities", "completions", "reminders",
		"relationship_edges", "life_events", "agenda", "gifts",
		"field_values", "external_identities", "external_activities",
		"circles", "tags",
	} {
		block, present := raw[key]
		require.Truef(t, present, "block %q must be present in the response even when empty", key)
		assert.JSONEqf(t, "[]", string(block), "block %q must serialize as an empty array, not null", key)
	}

	// Immich is the one legitimately-absent block when unconfigured.
	_, immichPresent := raw["immich"]
	assert.False(t, immichPresent, "immich block must be absent (not just null) when the user has no Immich config")

	// The contact block reuses NewContactRecordResponse verbatim.
	_, contactPresent := raw["contact"]
	assert.True(t, contactPresent)
	_, userPresent := raw["user"]
	assert.True(t, userPresent)
}

// TestGetContactDetail_ComposesAllBlocks seeds one row per block and asserts
// the composite surfaces it, including the two batch name-resolution
// enrichments (relationship edge other-party name, life event related-entity
// names).
func TestGetContactDetail_ComposesAllBlocks(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/detail", GetContactDetail)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Main", Lastname: "Subject"}
	require.NoError(t, db.Create(&contact).Error)
	other := models.Contact{UserID: user.ID, Firstname: "Other", Lastname: "Party"}
	require.NoError(t, db.Create(&other).Error)

	note := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "A note", Date: time.Now()}
	require.NoError(t, db.Create(&note).Error)

	activity := models.Activity{
		UserID: user.ID, Title: "Coffee", Date: time.Now(), Type: models.InteractionTypeVisit,
		Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)

	completion := models.ReminderCompletion{UserID: user.ID, ContactID: contact.ID, Message: "Done", CompletedAt: time.Now()}
	require.NoError(t, db.Create(&completion).Error)

	reminder := models.Reminder{UserID: user.ID, ContactID: &contact.ID, Message: "Follow up", RemindAt: time.Now().AddDate(0, 0, 1), Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)

	edge := models.RelationshipEdge{
		UserID: user.ID, SourceID: contact.VCardUID, TargetID: other.VCardUID,
		Type: "friend_of", Status: models.RelationshipStatusConfirmed,
	}
	require.NoError(t, db.Create(&edge).Error)

	lifeEvent := models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: "moved",
		RelatedEntityIDs: []string{other.VCardUID},
	}
	require.NoError(t, db.Create(&lifeEvent).Error)

	agenda := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about trip"}
	require.NoError(t, db.Create(&agenda).Error)

	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "Book", Status: "idea"}
	require.NoError(t, db.Create(&gift).Error)

	fieldValue := models.FieldValue{FieldDefinitionID: "some-def-id", UserID: user.ID, EntityID: contact.VCardUID, Value: json.RawMessage(`"blue"`)}
	require.NoError(t, db.Create(&fieldValue).Error)

	identity := models.ExternalIdentity{UserID: user.ID, EntityID: contact.VCardUID, System: "paperless", ExternalID: "ext-1"}
	require.NoError(t, db.Create(&identity).Error)

	extActivity := models.ExternalActivity{
		UserID: user.ID, EntityID: contact.VCardUID, SourceSystem: "paperless",
		ExternalID: "ext-evt-1", Type: "document-added", OccurredAt: time.Now(),
	}
	require.NoError(t, db.Create(&extActivity).Error)

	circle := models.Circle{UserID: user.ID, Name: "Book Club"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{UserID: user.ID, CircleID: circle.ID, MemberVCardUID: contact.VCardUID}).Error)

	tag := models.Tag{UserID: user.ID, Name: "VIP"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{UserID: user.ID, TagID: tag.ID, ContactVCardUID: contact.VCardUID}).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp models.ContactDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Notes, 1)
	assert.Equal(t, "A note", resp.Notes[0].Content)

	require.Len(t, resp.Activities, 1)
	assert.Equal(t, "Coffee", resp.Activities[0].Title)

	require.Len(t, resp.Completions, 1)
	require.Len(t, resp.Reminders, 1)

	require.Len(t, resp.RelationshipEdges, 1)
	assert.Equal(t, "Other Party", resp.RelationshipEdges[0].OtherPartyName)
	assert.Equal(t, other.ID, resp.RelationshipEdges[0].OtherPartyContactID)

	require.Len(t, resp.LifeEvents, 1)
	assert.Equal(t, "Other Party", resp.LifeEvents[0].RelatedEntityNames[other.VCardUID])

	require.Len(t, resp.Agenda, 1)
	require.Len(t, resp.Gifts, 1)
	require.Len(t, resp.FieldValues, 1)
	require.Len(t, resp.ExternalIdentities, 1)
	require.Len(t, resp.ExternalActivities, 1)

	require.Len(t, resp.Circles, 1)
	assert.Equal(t, "Book Club", resp.Circles[0].Name)
	require.Len(t, resp.Tags, 1)
	assert.Equal(t, "VIP", resp.Tags[0].Name)

	assert.Equal(t, contact.ID, resp.Contact.ID)
}

// TestGetContactDetail_ExcludesSecretRelationships mirrors the briefing
// composite's own coverage (TestGetContactBriefing_ExcludesSensitiveAndSuggestedRelationships)
// -- resolveConfirmedRelationships is shared code, but the composite's own
// wiring of it deserves its own pin.
func TestGetContactDetail_ExcludesSecretRelationships(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/detail", GetContactDetail)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Main"}
	require.NoError(t, db.Create(&contact).Error)
	secretParty := models.Contact{UserID: user.ID, Firstname: "Secret"}
	require.NoError(t, db.Create(&secretParty).Error)

	secretEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: contact.VCardUID, TargetID: secretParty.VCardUID,
		Type: "friend_of", Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&secretEdge).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp models.ContactDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.RelationshipEdges)
}

func TestGetContactDetail_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/detail", GetContactDetail)

	otherUser := models.User{Username: "other-detail", Password: "x", Email: "other-detail@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(othersContact.ID)+"/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetContactDetail_Immich covers the one legitimately-absent block:
// absent when the user has no Immich config, present-with-nil-summary when
// configured but this contact has no link, present-with-summary when linked.
func TestGetContactDetail_Immich(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/detail", GetContactDetail)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Immich Subject"}
	require.NoError(t, db.Create(&contact).Error)

	// No Immich config at all: block absent.
	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	_, present := raw["immich"]
	assert.False(t, present, "immich must be absent with no config")

	// Configured, but this contact has no Immich link: block present, summary nil.
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: user.ID, BaseURL: "https://immich.example.com"}).Error)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.ContactDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Immich, "immich must be present once the user has a config")
	assert.Nil(t, resp.Immich.Summary, "summary must be nil when this contact has no Immich link")
}
