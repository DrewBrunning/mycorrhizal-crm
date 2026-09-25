package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerOccasionEventRoutes wires the OccasionEvent surface (docs/adrs/
// 0026-occasions-events.md, issue #1228) directly, mirroring
// registerOccasionObligationRoutes.
func registerOccasionEventRoutes(t *testing.T, router *gin.Engine) {
	router.POST("/occasion-events", withValidated(func() any { return &models.OccasionEventInput{} }), CreateOccasionEvent)
	router.GET("/occasion-events", ListOccasionEvents)
	router.GET("/occasion-events/invitee-suggestions", GetInviteeSuggestions)
	router.GET("/occasion-events/:id", GetOccasionEvent)
	router.PUT("/occasion-events/:id", withValidated(func() any { return &models.OccasionEventInput{} }), UpdateOccasionEvent)
	router.DELETE("/occasion-events/:id", DeleteOccasionEvent)
	router.POST("/occasion-events/:id/attendees", withValidated(func() any { return &models.OccasionEventAttendeeInput{} }), AddOccasionEventAttendee)
	router.PUT("/occasion-events/:id/attendees/:vcard_uid", withValidated(func() any { return &models.OccasionEventAttendeeUpdateInput{} }), UpdateOccasionEventAttendee)
	router.DELETE("/occasion-events/:id/attendees/:vcard_uid", RemoveOccasionEventAttendee)
}

// ---- create ---------------------------------------------------------------

func TestCreateOccasionEvent(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	start := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Hour)
	w := doOccasionJSON(router, "POST", "/occasion-events", models.OccasionEventInput{
		Title:    "Summer BBQ",
		StartsAt: &start,
		EndsAt:   &end,
		Location: "The park",
		Notes:    "Bring the folding chairs",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var stored models.OccasionEvent
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, "Summer BBQ", stored.Title)
	assert.Equal(t, start, stored.StartsAt.UTC())
	require.NotNil(t, stored.EndsAt)
	assert.Equal(t, models.RelationshipSensitivityNormal, stored.Sensitivity, "omitted sensitivity must default to normal")
	assert.Equal(t, "Bring the folding chairs", stored.Notes, "encrypted notes must persist")
	_ = contact
}

func TestCreateOccasionEventRejectsEndBeforeStart(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	start := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour)
	w := doOccasionJSON(router, "POST", "/occasion-events", models.OccasionEventInput{
		Title:    "Backwards",
		StartsAt: &start,
		EndsAt:   &end,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionEvent{}).Count(&count)
	assert.Zero(t, count)
}

// ---- read / list ----------------------------------------------------------

func TestGetOccasionEventIncludesAttendees(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	start := time.Now().UTC().Add(24 * time.Hour)
	event := models.OccasionEvent{UserID: user.ID, Title: "Dinner", StartsAt: start}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: event.ID, EntityID: contact.VCardUID, RSVP: models.OccasionEventRSVPAccepted}).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events/"+event.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		OccasionEvent models.OccasionEvent               `json:"occasion_event"`
		Attendees     []models.OccasionEventAttendeeView `json:"attendees"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, event.ID, body.OccasionEvent.ID)
	require.Len(t, body.Attendees, 1)
	assert.Equal(t, contact.VCardUID, body.Attendees[0].EntityID)
	assert.Equal(t, models.OccasionEventRSVPAccepted, body.Attendees[0].RSVP)
	assert.Equal(t, contact.ID, body.Attendees[0].ContactID)
	assert.Equal(t, "Alice", body.Attendees[0].ContactName)
}

func TestGetOccasionEventEmptyAttendeesSerializesAsArray(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	event := models.OccasionEvent{UserID: user.ID, Title: "Solo", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events/"+event.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// CLAUDE.md frontend trap #8: the key must be present as [], not absent.
	assert.Contains(t, w.Body.String(), `"attendees":[]`)
}

func TestListOccasionEventsWindow(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i, offset := range []int{5, 40, 100} {
		require.NoError(t, db.Create(&models.OccasionEvent{
			UserID: user.ID, Title: "event", StartsAt: now.AddDate(0, 0, offset), CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}).Error)
	}

	w := doOccasionJSON(router, "GET", "/occasion-events?from=2026-07-01T00:00:00Z&to=2026-09-01T00:00:00Z", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		OccasionEvents []models.OccasionEvent `json:"occasion_events"`
		Total          int64                  `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body.Total, "only the 5-day and 40-day events fall in the window")
	assert.Len(t, body.OccasionEvents, 2)
}

func TestListOccasionEventsSinceReturnsTombstones(t *testing.T) {
	db, router := setupRouterWithRetention(t, 30)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	first := models.OccasionEvent{UserID: user.ID, Title: "first", StartsAt: time.Now().UTC()}
	second := models.OccasionEvent{UserID: user.ID, Title: "gone", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)
	afterFirst := EncodeCursor(first.UpdatedAt, first.ID)
	require.NoError(t, db.Delete(&second).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events?since="+afterFirst, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		OccasionEvents []models.OccasionEvent `json:"occasion_events"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.OccasionEvents, 1)
	assert.True(t, body.OccasionEvents[0].Deleted, "a soft-deleted event must surface as a tombstone on the change feed")
}

func TestListOccasionEventsRejectsBadWindow(t *testing.T) {
	_, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	w := doOccasionJSON(router, "GET", "/occasion-events?from=not-a-date", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// ---- update / delete ------------------------------------------------------

func TestUpdateOccasionEventFullReplace(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	event := models.OccasionEvent{UserID: user.ID, Title: "Old", StartsAt: time.Now().UTC(), Location: "Old place", Notes: "old"}
	require.NoError(t, db.Create(&event).Error)

	newStart := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	w := doOccasionJSON(router, "PUT", "/occasion-events/"+event.ID, models.OccasionEventInput{
		Title:    "New",
		StartsAt: &newStart,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.OccasionEvent
	require.NoError(t, db.First(&reloaded, "id = ?", event.ID).Error)
	assert.Equal(t, "New", reloaded.Title)
	assert.Empty(t, reloaded.Location, "full replace clears an omitted field")
	assert.Empty(t, reloaded.Notes)
}

func TestUpdateOccasionEventForeignIsNotFound(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	other := models.User{Username: "other-event", Password: "x", Email: "other-event@example.com"}
	require.NoError(t, db.Create(&other).Error)
	event := models.OccasionEvent{UserID: other.ID, Title: "Theirs", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)

	start := time.Now().UTC()
	w := doOccasionJSON(router, "PUT", "/occasion-events/"+event.ID, models.OccasionEventInput{Title: "Hijack", StartsAt: &start})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestDeleteOccasionEventHardDeletesAttendees(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: event.ID, EntityID: contact.VCardUID}).Error)

	w := doOccasionJSON(router, "DELETE", "/occasion-events/"+event.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var eventCount int64
	require.NoError(t, db.Model(&models.OccasionEvent{}).Where("id = ?", event.ID).Count(&eventCount).Error)
	assert.Zero(t, eventCount, "event is soft-deleted (default scope hides it)")

	var attendeeCount int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("event_id = ?", event.ID).Count(&attendeeCount).Error)
	assert.Zero(t, attendeeCount, "attendees are hard-deleted with their event")
}

// ---- attendees ------------------------------------------------------------

func TestAddOccasionEventAttendee(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)

	w := doOccasionJSON(router, "POST", "/occasion-events/"+event.ID+"/attendees", models.OccasionEventAttendeeInput{EntityID: contact.VCardUID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.OccasionEventAttendee
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, models.OccasionEventRSVPPending, stored.RSVP, "omitted RSVP defaults to pending")

	// Duplicate add is a checked 409.
	dup := doOccasionJSON(router, "POST", "/occasion-events/"+event.ID+"/attendees", models.OccasionEventAttendeeInput{EntityID: contact.VCardUID})
	assert.Equal(t, http.StatusConflict, dup.Code, dup.Body.String())
}

func TestAddOccasionEventAttendeeForeignContactIsNotFound(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	other := models.User{Username: "other-attendee", Password: "x", Email: "other-attendee@example.com"}
	require.NoError(t, db.Create(&other).Error)
	othersContact := seedOccasionObligationContact(t, db, other.ID)
	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)

	w := doOccasionJSON(router, "POST", "/occasion-events/"+event.ID+"/attendees", models.OccasionEventAttendeeInput{EntityID: othersContact.VCardUID})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateOccasionEventAttendeeRSVP(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: event.ID, EntityID: contact.VCardUID}).Error)

	w := doOccasionJSON(router, "PUT", "/occasion-events/"+event.ID+"/attendees/"+contact.VCardUID,
		models.OccasionEventAttendeeUpdateInput{RSVP: models.OccasionEventRSVPDeclined})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.OccasionEventAttendee
	require.NoError(t, db.First(&reloaded, "event_id = ? AND entity_id = ?", event.ID, contact.VCardUID).Error)
	assert.Equal(t, models.OccasionEventRSVPDeclined, reloaded.RSVP)
}

func TestRemoveOccasionEventAttendee(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: event.ID, EntityID: contact.VCardUID}).Error)

	w := doOccasionJSON(router, "DELETE", "/occasion-events/"+event.ID+"/attendees/"+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("event_id = ?", event.ID).Count(&count).Error)
	assert.Zero(t, count)

	// Removing again is a 404.
	again := doOccasionJSON(router, "DELETE", "/occasion-events/"+event.ID+"/attendees/"+contact.VCardUID, nil)
	assert.Equal(t, http.StatusNotFound, again.Code, again.Body.String())
}

// ---- invitee suggestions --------------------------------------------------

func TestGetInviteeSuggestionsExpandsCircles(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	a := seedOccasionObligationContact(t, db, user.ID)
	b := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&b).Error)
	circleA := models.Circle{UserID: user.ID, Name: "Family"}
	require.NoError(t, db.Create(&circleA).Error)
	circleB := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circleB).Error)

	require.NoError(t, db.Create(&models.CircleMember{CircleID: circleA.ID, UserID: user.ID, MemberVCardUID: a.VCardUID}).Error)
	// a is in both circles: the result must de-duplicate.
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circleB.ID, UserID: user.ID, MemberVCardUID: a.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circleB.ID, UserID: user.ID, MemberVCardUID: b.VCardUID}).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events/invitee-suggestions?circle_ids="+circleA.ID+","+circleB.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Suggestions []models.InviteeSuggestion `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Suggestions, 2, "cross-circle duplicate must collapse to one")
}

func TestGetInviteeSuggestionsExcludesExistingAttendees(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	var user models.User
	db.First(&user)
	a := seedOccasionObligationContact(t, db, user.ID)
	b := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&b).Error)
	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: a.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: b.VCardUID}).Error)

	event := models.OccasionEvent{UserID: user.ID, Title: "Party", StartsAt: time.Now().UTC()}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: event.ID, EntityID: a.VCardUID}).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events/invitee-suggestions?circle_ids="+circle.ID+"&event_id="+event.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Suggestions []models.InviteeSuggestion `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Suggestions, 1)
	assert.Equal(t, b.VCardUID, body.Suggestions[0].EntityID)
}

func TestGetInviteeSuggestionsValidation(t *testing.T) {
	_, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	w := doOccasionJSON(router, "GET", "/occasion-events/invitee-suggestions", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetInviteeSuggestionsForeignCircleIsNotFound(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionEventRoutes(t, router)

	other := models.User{Username: "other-suggest", Password: "x", Email: "other-suggest@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Circle{UserID: other.ID, Name: "Theirs"}
	require.NoError(t, db.Create(&foreign).Error)

	w := doOccasionJSON(router, "GET", "/occasion-events/invitee-suggestions?circle_ids="+foreign.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// TestOccasionEvent_RealMigratedSchema is the real-DB check (CLAUDE.md backend
// trap #1) for docs/adrs/0026-occasions-events.md: the full lifecycle against a
// database.InitDB-migrated real file database — encrypted notes column, the
// attendee composite unique index, and the soft-delete + hard-delete split that
// AutoMigrate alone cannot prove.
func TestOccasionEvent_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "event-realdb", Password: "password123!A", Email: "event-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerOccasionEventRoutes(t, router)

	start := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	createResp := doOccasionJSON(router, "POST", "/occasion-events", models.OccasionEventInput{
		Title:    "Real DB event",
		StartsAt: &start,
		Notes:    "Encrypted at rest",
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		OccasionEvent models.OccasionEvent `json:"occasion_event"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	eventID := created.OccasionEvent.ID
	require.NotEmpty(t, eventID)

	var stored models.OccasionEvent
	require.NoError(t, db.First(&stored, "id = ?", eventID).Error)
	assert.Equal(t, "Encrypted at rest", stored.Notes, "encrypted notes column must round-trip through the real migration")

	// Add attendee, then the composite unique index rejects a duplicate row
	// inserted directly (the handler's own 409 check is exercised elsewhere).
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: eventID, EntityID: contact.VCardUID}).Error)
	dup := db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: eventID, EntityID: contact.VCardUID})
	require.Error(t, dup.Error, "(event_id, entity_id) must be unique in the real schema")

	// A stale attendee row referencing a removed contact does not block
	// re-adding the same card once removed (hard delete, no lingering unique
	// key).
	require.NoError(t, db.Where("event_id = ? AND entity_id = ?", eventID, contact.VCardUID).Delete(&models.OccasionEventAttendee{}).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: eventID, EntityID: contact.VCardUID}).Error)

	deleteResp := doOccasionJSON(router, "DELETE", "/occasion-events/"+eventID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())

	var attendeeCount int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("event_id = ?", eventID).Count(&attendeeCount).Error)
	assert.Zero(t, attendeeCount)

	// The event row itself survives, soft-deleted.
	var unscoped models.OccasionEvent
	require.NoError(t, db.Unscoped().First(&unscoped, "id = ?", eventID).Error)
	assert.True(t, unscoped.DeletedAt.Valid)
}
