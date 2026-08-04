package controllers

import (
	"encoding/json"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idString renders a contact's numeric ID for URL building (GORM uint PKs).
func idString(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func TestGetContactBriefing_ComposesAllBlocks(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{
		UserID: user.ID, Firstname: "Alice", Lastname: "Wonder",
	}
	require.NoError(t, db.Create(&contact).Error)
	// Set CRMEnvelope.Kind + Birthday via the proper record path (CLAUDE.md
	// trap 2: BeforeSave derives the nested model from Card/CRM, so direct
	// field mutation is skipped). The Card must carry the name components and
	// the birthday anniversary or the derivation overwrites them from the
	// empty/partial Card.
	birthday := time.Now().AddDate(0, 0, 10)
	birthYear, birthMonth, birthDay := birthday.Year(), int(birthday.Month()), birthday.Day()
	models.ApplyRecordToContact(&contact, &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{
				Components: []contactmodel.NameComponent{
					{Kind: "given", Value: "Alice"},
					{Kind: "surname", Value: "Wonder"},
				},
			},
			Anniversaries: []contactmodel.Anniversary{
				{
					Kind: "birth",
					Date: contactmodel.AnniversaryDate{
						Partial: &contactmodel.PartialDate{
							Year:  &birthYear,
							Month: intPtr(birthMonth),
							Day:   intPtr(birthDay),
						},
					},
				},
			},
		},
		Envelope: contactmodel.CRMEnvelope{Kind: "human"},
	}, "")
	require.NoError(t, db.Save(&contact).Error)

	// Activity (must involve the contact via the join table).
	activity := models.Activity{UserID: user.ID, Title: "Coffee", Type: "visit", Date: time.Now().AddDate(0, -1, 0)}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Model(&activity).Association("Contacts").Replace(&[]models.Contact{contact}))

	// Note.
	note := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "Talks about her garden", Date: time.Now().AddDate(0, 0, -3)}
	require.NoError(t, db.Create(&note).Error)

	// Cadence policy + a qualifying activity (the one above is qualifying).
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)

	// Open agenda item.
	agenda := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the surgery"}
	require.NoError(t, db.Create(&agenda).Error)

	// Confirmed relationship with Bob.
	bob := models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Marley"}
	require.NoError(t, db.Create(&bob).Error)
	edge := models.RelationshipEdge{
		UserID: user.ID, SourceID: bob.VCardUID, TargetID: contact.VCardUID, Type: "spouse_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
	}
	require.NoError(t, db.Create(&edge).Error)

	// Life event.
	lifeEvent := models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "graduated", Description: "PhD"}
	require.NoError(t, db.Create(&lifeEvent).Error)

	// Upcoming reminder.
	reminder := models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "Send card",
		RemindAt: time.Now().AddDate(0, 0, 7), Recurrence: "once", Completed: false,
	}
	require.NoError(t, db.Create(&reminder).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var briefing models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &briefing))

	assert.Equal(t, contact.ID, briefing.ContactID)
	assert.Equal(t, "Alice Wonder", briefing.Name)
	assert.Equal(t, "human", briefing.Kind)

	// Last activity block.
	require.NotNil(t, briefing.LastActivity)
	assert.Equal(t, "Coffee", briefing.LastActivity.Title)

	// Recent notes.
	require.Len(t, briefing.RecentNotes, 1)
	assert.Equal(t, "Talks about her garden", briefing.RecentNotes[0].Content)

	// Cadence health block — one qualifying activity in the past, 30-day
	// interval: not yet overdue (last interaction was yesterday).
	require.NotNil(t, briefing.Cadence)
	assert.True(t, briefing.Cadence.Health.HasQualifyingInteraction)

	// Open agenda items.
	require.Len(t, briefing.OpenAgendaItems, 1)
	assert.Equal(t, "Ask about the surgery", briefing.OpenAgendaItems[0].Content)

	// Relationships: confirmed edge with Bob, displayed as spouse from
	// Alice's perspective (target) → type stays spouse_of.
	require.Len(t, briefing.Relationships, 1)
	assert.Equal(t, "Bob Marley", briefing.Relationships[0].OtherPartyName)
	assert.Equal(t, "spouse_of", briefing.Relationships[0].DisplayToken)

	// Life events.
	require.Len(t, briefing.LifeEvents, 1)
	assert.Equal(t, "graduated", briefing.LifeEvents[0].Type)

	// Upcoming reminders.
	require.Len(t, briefing.UpcomingReminders, 1)
	assert.Equal(t, "Send card", briefing.UpcomingReminders[0].Message)

	// Upcoming dates: the birthday we set is 10 days out.
	require.Len(t, briefing.UpcomingDates, 1)
	assert.Equal(t, "birthday", briefing.UpcomingDates[0].Label)
}

func TestGetContactBriefing_GracefulDegradation(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	// A contact with nothing but a name — every block must be empty/absent.
	contact := models.Contact{UserID: user.ID, Firstname: "Empty"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var briefing models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &briefing))

	assert.Equal(t, "Empty", briefing.Name)
	assert.Nil(t, briefing.LastActivity)
	assert.Empty(t, briefing.RecentNotes)
	assert.Nil(t, briefing.Cadence)
	assert.Empty(t, briefing.OpenAgendaItems)
	assert.Empty(t, briefing.Relationships)
	assert.Empty(t, briefing.LifeEvents)
	assert.Empty(t, briefing.UpcomingReminders)
	assert.Empty(t, briefing.UpcomingDates)
}

// TestGetContactBriefing_EmptyBlocksSerializeAsArrays pins the *wire* shape of
// an empty briefing, which is a different assertion from
// TestGetContactBriefing_GracefulDegradation above: that test unmarshals into
// models.ContactBriefing, where an absent key and an empty array both decode to
// a nil slice, so it passes whether the server omits the block or emits `[]`.
//
// The server used to omit them (`omitempty` + GORM leaving nil slices on a
// no-rows Find), and the frontend's `briefing.open_agenda_items.length` crashed
// the whole prep view into its ErrorBoundary. Every freshly-created contact is
// in exactly this state, so the feature was broken on first use and no Go test
// could see it. Assert on the raw JSON — that is the contract the frontend
// actually consumes.
func TestGetContactBriefing_EmptyBlocksSerializeAsArrays(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	// Every collection block must be present and be `[]` — never missing, never
	// `null`. The frontend type declares these required and dereferences
	// `.length` on them without a guard.
	for _, key := range []string{
		"recent_notes",
		"open_agenda_items",
		"relationships",
		"life_events",
		"upcoming_reminders",
		"upcoming_dates",
	} {
		block, present := raw[key]
		require.Truef(t, present, "block %q must be present in the response even when empty", key)
		assert.JSONEqf(t, "[]", string(block), "block %q must serialize as an empty array, not null", key)
	}
}

func TestGetContactBriefing_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	otherUser := models.User{Username: "other-briefing", Password: "x", Email: "other-briefing@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(othersContact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetContactBriefing_ExcludesSensitiveAndSuggestedRelationships(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	carol := models.Contact{UserID: user.ID, Firstname: "Carol"}
	dan := models.Contact{UserID: user.ID, Firstname: "Dan"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	require.NoError(t, db.Create(&carol).Error)
	require.NoError(t, db.Create(&dan).Error)

	// Secret edge — must be excluded (91.13: secret must never surface on a
	// screen likely to be open in front of the person it concerns).
	secretEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: bob.VCardUID, Type: "partner_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&secretEdge).Error)

	// Private edge — stays (the briefing is the user's own screen; private
	// gates sharing/exposure, not the user's own briefing).
	privateEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: dan.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityPrivate,
	}
	require.NoError(t, db.Create(&privateEdge).Error)

	// Suggested edge — must be excluded (not fact).
	suggestedEdge := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: carol.VCardUID, Type: "sibling_of",
		Source: models.RelationshipSourceHouseholdInferred, Confidence: 0.7, Status: models.RelationshipStatusSuggested,
	}
	require.NoError(t, db.Create(&suggestedEdge).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(alice.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var briefing models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &briefing))
	// Only the private edge survives — secret and suggested are filtered out.
	require.Len(t, briefing.Relationships, 1)
	assert.Equal(t, "friend_of", briefing.Relationships[0].DisplayToken)
	assert.Equal(t, "Dan", briefing.Relationships[0].OtherPartyName)
}

func TestGetContactBriefing_DisplayTokenInvertsWhenViewedIsSource(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	// Alice is the parent (source); Bob is the child (target). Stored edge:
	// source=Alice, type=parent_of → from Alice's perspective Bob is her
	// child, so the display token must invert to child_of.
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	edge := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: bob.VCardUID, Type: "parent_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
	}
	require.NoError(t, db.Create(&edge).Error)

	// From Alice's page.
	req, _ := http.NewRequest("GET", "/contacts/"+idString(alice.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var b1 models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b1))
	require.Len(t, b1.Relationships, 1)
	assert.Equal(t, "child_of", b1.Relationships[0].DisplayToken)
	assert.Equal(t, bob.VCardUID, b1.Relationships[0].OtherPartyUID)
	assert.Equal(t, bob.ID, b1.Relationships[0].OtherPartyContactID)

	// From Bob's page: the edge reads as parent_of (Alice is his parent).
	req, _ = http.NewRequest("GET", "/contacts/"+idString(bob.ID)+"/briefing", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var b2 models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b2))
	require.Len(t, b2.Relationships, 1)
	assert.Equal(t, "parent_of", b2.Relationships[0].DisplayToken)
	assert.Equal(t, alice.VCardUID, b2.Relationships[0].OtherPartyUID)
}

func TestGetContactBriefing_CadenceWithNoQualifyingInteraction(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Inactive"}
	require.NoError(t, db.Create(&contact).Error)
	// Cadence policy exists, but no activities at all — the "no
	// qualifying interaction ever" sub-state within the cadence card.
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var b models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	require.NotNil(t, b.Cadence)
	assert.False(t, b.Cadence.Health.HasQualifyingInteraction)
	assert.Equal(t, 0, b.Cadence.Health.OverdueBy)
}

func TestGetContactBriefing_BirthdayAndAnniversary(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Celebrant"}
	require.NoError(t, db.Create(&contact).Error)
	// Build both birth and wedding anniversaries so the flat Birthday/
	// Anniversary columns are derived correctly (CLAUDE.md trap 2).
	bday := time.Now().AddDate(0, 0, 5)
	anniv := time.Now().AddDate(0, 0, 20)
	bYear, bMonth, bDay := bday.Year(), int(bday.Month()), bday.Day()
	aYear, aMonth, aDay := anniv.Year(), int(anniv.Month()), anniv.Day()
	models.ApplyRecordToContact(&contact, &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{
				Components: []contactmodel.NameComponent{{Kind: "given", Value: "Celebrant"}},
			},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: &bYear, Month: intPtr(bMonth), Day: intPtr(bDay)}}},
				{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: &aYear, Month: intPtr(aMonth), Day: intPtr(aDay)}}},
			},
		},
		Envelope: contactmodel.CRMEnvelope{},
	}, "")
	require.NoError(t, db.Save(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var b models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	require.Len(t, b.UpcomingDates, 2)
	assert.Equal(t, "birthday", b.UpcomingDates[0].Label)
	assert.Greater(t, b.UpcomingDates[0].DaysUntil, 0)
	assert.Equal(t, "anniversary", b.UpcomingDates[1].Label)
	assert.Greater(t, b.UpcomingDates[1].DaysUntil, 0)
}

func TestGetContactBriefing_ExcludesCompletedReminders(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Reminded"}
	require.NoError(t, db.Create(&contact).Error)

	// Completed reminder — must be excluded.
	completed := models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "Already done",
		RemindAt: time.Now().AddDate(0, 0, 3), Recurrence: "once", Completed: true,
	}
	require.NoError(t, db.Create(&completed).Error)

	// Incomplete reminder — must be included.
	upcoming := models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "Still needed",
		RemindAt: time.Now().AddDate(0, 0, 3), Recurrence: "once", Completed: false,
	}
	require.NoError(t, db.Create(&upcoming).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var b models.ContactBriefing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	require.Len(t, b.UpcomingReminders, 1)
	assert.Equal(t, "Still needed", b.UpcomingReminders[0].Message)
}

func TestGetContactBriefing_NotFound(t *testing.T) {
	_, router := setupRouter()
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	req, _ := http.NewRequest("GET", "/contacts/999999/briefing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
