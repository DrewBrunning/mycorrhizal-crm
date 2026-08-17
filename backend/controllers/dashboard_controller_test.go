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

// TestGetDashboard_EmptyBlocksSerializeAsArrays pins the M3 composite's most
// likely regression: every collection block must serialize as `[]`, never
// `null`/absent, on a brand-new account with no data (CLAUDE.md frontend
// trap 8 — the exact bug that broke the N2 prep view once already). Assert
// on the raw JSON, since decoding into the Go struct makes "absent" and `[]`
// indistinguishable.
func TestGetDashboard_EmptyBlocksSerializeAsArrays(t *testing.T) {
	db, router := setupRouter()
	router.GET("/dashboard", GetDashboard)
	_ = db

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	for _, key := range []string{"birthdays", "random_contacts", "upcoming_reminders", "overdue", "favorites"} {
		block, present := raw[key]
		require.Truef(t, present, "block %q must be present in the response even when empty", key)
		assert.JSONEqf(t, "[]", string(block), "block %q must serialize as an empty array, not null", key)
	}
}

// TestGetDashboard_PopulatedComposesAllBlocks seeds one of each block's
// source data and asserts the composite surfaces it, including a reminder's
// contact_name resolving without a second fetch (design decision 2).
func TestGetDashboard_PopulatedComposesAllBlocks(t *testing.T) {
	db, router := setupRouter()
	router.GET("/dashboard", GetDashboard)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	// A contact with a birthday 3 days out, so it lands in both the
	// birthdays block and (via its reminder) the reminders block.
	birthday := time.Now().AddDate(0, 0, 3)
	contact := models.Contact{
		UserID: user.ID, Firstname: "Nick", Lastname: "Name", Nickname: "Nicky",
		Birthday: birthday.Format("2006-01-02"),
	}
	require.NoError(t, db.Create(&contact).Error)

	reminder := models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "Call about the trip",
		RemindAt: time.Now().AddDate(0, 0, 1), Recurrence: "once",
	}
	require.NoError(t, db.Create(&reminder).Error)

	// Overdue cadence: last qualifying interaction ~40 days ago, 30-day
	// interval (mirrors TestGetOverdueCadences's setup in
	// cadence_controller_test.go).
	oldActivity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&oldActivity).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}).Error)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp models.DashboardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Birthdays, 1)
	assert.Equal(t, contact.ID, resp.Birthdays[0].ContactID)

	require.Len(t, resp.UpcomingReminders, 1)
	assert.Equal(t, "Nicky Name", resp.UpcomingReminders[0].ContactName)
	assert.Equal(t, "Call about the trip", resp.UpcomingReminders[0].Message)

	require.Len(t, resp.Overdue, 1)
	assert.Equal(t, contact.VCardUID, resp.Overdue[0].Policy.EntityID)
	assert.Equal(t, contact.ID, resp.Overdue[0].ContactID)
}

// TestGetDashboard_ScopedToUser confirms every block is scoped by the
// requesting user (CLAUDE.md trap 5) — a second user's data must never
// appear.
func TestGetDashboard_ScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/dashboard", GetDashboard)

	otherUser := models.User{Username: "other-dash", Password: "x", Email: "other-dash@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)

	birthday := time.Now().AddDate(0, 0, 3)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not", Lastname: "Yours", Birthday: birthday.Format("2006-01-02")}
	require.NoError(t, db.Create(&othersContact).Error)
	othersReminder := models.Reminder{
		UserID: otherUser.ID, ContactID: &othersContact.ID, Message: "Not yours either",
		RemindAt: time.Now().AddDate(0, 0, 1), Recurrence: "once",
	}
	require.NoError(t, db.Create(&othersReminder).Error)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp models.DashboardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Empty(t, resp.Birthdays)
	assert.Empty(t, resp.UpcomingReminders)
	assert.Empty(t, resp.RandomContacts)
	assert.Empty(t, resp.Favorites)
}

// TestGetDashboard_FavoritesBlock seeds favorites (live and archived) and
// plain contacts, and asserts the favorites block surfaces exactly the live
// favorites in name order — and that non-favorites and archived favorites
// never leak in (issue #173).
func TestGetDashboard_FavoritesBlock(t *testing.T) {
	db, router := setupRouter()
	router.GET("/dashboard", GetDashboard)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	zebra := models.Contact{UserID: user.ID, Firstname: "Zebra", IsFavorite: true}
	alpha := models.Contact{UserID: user.ID, Firstname: "Alpha", IsFavorite: true}
	plain := models.Contact{UserID: user.ID, Firstname: "Plain"}
	archivedFav := models.Contact{UserID: user.ID, Firstname: "Arch", IsFavorite: true, Archived: true}
	require.NoError(t, db.Create(&zebra).Error)
	require.NoError(t, db.Create(&alpha).Error)
	require.NoError(t, db.Create(&plain).Error)
	require.NoError(t, db.Create(&archivedFav).Error)

	// Another user's favorite must not leak in (CLAUDE.md trap 5).
	other := models.User{Username: "other-dash-fav", Password: "x", Email: "other-dash-fav@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Theirs", IsFavorite: true}).Error)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp models.DashboardResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Favorites, 2, "only the caller's live favorites")
	assert.Equal(t, "Alpha", resp.Favorites[0].Firstname, "favorites must be name-ordered")
	assert.Equal(t, "Zebra", resp.Favorites[1].Firstname)
	assert.True(t, resp.Favorites[0].IsFavorite, "the wire flag must be true for a favorite")
}
