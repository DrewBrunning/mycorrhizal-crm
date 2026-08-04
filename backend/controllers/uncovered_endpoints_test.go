package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Endpoints that were reachable from the UI but had no test at all. Each is
// small, but the shared risk is the same: an ownership-scoping regression on
// any of them is an IDOR, and there was nothing to catch one.

// --- ArchiveContact / UnarchiveContact ---------------------------------------

func TestArchiveContact_SetsFlagAndRetiresReminders(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/archive", ArchiveContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Archie"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "ping", RemindAt: time.Now().AddDate(0, 0, 5),
	}).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(contact.ID)+"/archive", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.True(t, reloaded.Archived)

	// Archiving retires the contact's reminders — otherwise an archived
	// contact keeps generating reminder email.
	var reminderCount int64
	require.NoError(t, db.Model(&models.Reminder{}).Where("contact_id = ?", contact.ID).Count(&reminderCount).Error)
	assert.Zero(t, reminderCount, "archiving must delete the contact's reminders")
}

func TestUnarchiveContact_ClearsFlag(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/unarchive", UnarchiveContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Archie", Archived: true}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(contact.ID)+"/unarchive", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.False(t, reloaded.Archived)
}

func TestArchiveContact_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/archive", ArchiveContact)

	other := models.User{Username: "other-archive", Email: "other-archive@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&theirs).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(theirs.ID)+"/archive", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, theirs.ID).Error)
	assert.False(t, reloaded.Archived, "another user's contact must not be archivable")
}

func TestUnarchiveContact_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/unarchive", UnarchiveContact)

	other := models.User{Username: "other-unarchive", Email: "other-unarchive@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Not Yours", Archived: true}
	require.NoError(t, db.Create(&theirs).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(theirs.ID)+"/unarchive", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- ListUserDirectory --------------------------------------------------------

// The directory backs P1's share-recipient picker and is the only endpoint any
// authenticated (non-admin) user can call that returns other users. It must
// expose id + username ONLY — never email, password hash, or admin status.
func TestListUserDirectory_ReturnsOtherUsersWithoutSensitiveFields(t *testing.T) {
	db, router := setupRouter()
	router.GET("/users/directory", ListUserDirectory)

	var me models.User
	require.NoError(t, db.First(&me).Error)
	require.NoError(t, db.Create(&models.User{
		Username: "zoe", Email: "zoe@example.com", Password: "secret-hash", IsAdmin: true,
	}).Error)
	require.NoError(t, db.Create(&models.User{
		Username: "adam", Email: "adam@example.com", Password: "secret-hash",
	}).Error)

	req, _ := http.NewRequest("GET", "/users/directory", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.NotContains(t, body, "secret-hash", "password hashes must never reach the directory")
	assert.NotContains(t, body, "zoe@example.com", "emails must not be exposed to non-admins")
	assert.NotContains(t, body, "is_admin", "admin status must not be exposed")

	var resp struct {
		Users []UserDirectoryEntry `json:"users"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Users, 2, "every user except the caller")

	// Ordered by username.
	assert.Equal(t, "adam", resp.Users[0].Username)
	assert.Equal(t, "zoe", resp.Users[1].Username)

	for _, entry := range resp.Users {
		assert.NotEqual(t, me.ID, entry.ID, "the caller must not be listed as a share recipient")
	}
}

func TestListUserDirectory_ExcludesOnlyTheCaller(t *testing.T) {
	db, router := setupRouter()
	router.GET("/users/directory", ListUserDirectory)

	var me models.User
	require.NoError(t, db.First(&me).Error)

	req, _ := http.NewRequest("GET", "/users/directory", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Users []UserDirectoryEntry `json:"users"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Users, "a lone user sees an empty directory, not themselves")
}

// --- UpdateHouseholdMember ----------------------------------------------------

func TestUpdateHouseholdMember_ChangesRole(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/households/:id/members/:vcard_uid", UpdateHouseholdMember)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Member"}
	require.NoError(t, db.Create(&contact).Error)
	household := models.Household{UserID: user.ID, Name: "The Smiths"}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{
		HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: contact.VCardUID, Role: "child",
	}).Error)

	body, _ := json.Marshal(map[string]string{"role": "parent"})
	req, _ := http.NewRequest("PATCH",
		"/households/"+household.ID+"/members/"+contact.VCardUID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var member models.HouseholdMember
	require.NoError(t, db.Where("household_id = ? AND member_vcard_uid = ?",
		household.ID, contact.VCardUID).First(&member).Error)
	assert.Equal(t, "parent", member.Role)
}

func TestUpdateHouseholdMember_UnknownMemberIs404(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/households/:id/members/:vcard_uid", UpdateHouseholdMember)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	household := models.Household{UserID: user.ID, Name: "Empty House"}
	require.NoError(t, db.Create(&household).Error)

	body, _ := json.Marshal(map[string]string{"role": "parent"})
	req, _ := http.NewRequest("PATCH",
		"/households/"+household.ID+"/members/not-a-member", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateHouseholdMember_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/households/:id/members/:vcard_uid", UpdateHouseholdMember)

	other := models.User{Username: "other-household", Email: "other-household@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&theirContact).Error)
	theirHousehold := models.Household{UserID: other.ID, Name: "Their House"}
	require.NoError(t, db.Create(&theirHousehold).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{
		HouseholdID: theirHousehold.ID, UserID: other.ID,
		MemberVCardUID: theirContact.VCardUID, Role: "child",
	}).Error)

	body, _ := json.Marshal(map[string]string{"role": "parent"})
	req, _ := http.NewRequest("PATCH",
		"/households/"+theirHousehold.ID+"/members/"+theirContact.VCardUID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var member models.HouseholdMember
	require.NoError(t, db.Where("household_id = ?", theirHousehold.ID).First(&member).Error)
	assert.Equal(t, "child", member.Role, "another user's membership must be unchanged")
}

// --- GetExternalActivity ------------------------------------------------------

func TestGetExternalActivity_ReturnsOwnedActivity(t *testing.T) {
	db, router := setupRouter()
	router.GET("/external-activities/:id", GetExternalActivity)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Subject"}
	require.NoError(t, db.Create(&contact).Error)

	activity := models.ExternalActivity{
		UserID: user.ID, EntityID: contact.VCardUID, SourceSystem: "immich",
		ExternalID: "asset-1", Type: "photo-appearance", OccurredAt: time.Now(),
	}
	require.NoError(t, db.Create(&activity).Error)

	req, _ := http.NewRequest("GET", "/external-activities/"+activity.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got models.ExternalActivity
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "immich", got.SourceSystem)
	assert.Equal(t, "asset-1", got.ExternalID)
}

func TestGetExternalActivity_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/external-activities/:id", GetExternalActivity)

	other := models.User{Username: "other-extact", Email: "other-extact@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&theirContact).Error)
	theirs := models.ExternalActivity{
		UserID: other.ID, EntityID: theirContact.VCardUID, SourceSystem: "immich",
		ExternalID: "asset-2", Type: "photo-appearance", OccurredAt: time.Now(),
	}
	require.NoError(t, db.Create(&theirs).Error)

	req, _ := http.NewRequest("GET", "/external-activities/"+theirs.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetExternalActivity_UnknownIDIs404(t *testing.T) {
	_, router := setupRouter()
	router.GET("/external-activities/:id", GetExternalActivity)

	req, _ := http.NewRequest("GET", "/external-activities/does-not-exist", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Dashboard endpoints ------------------------------------------------------

func TestGetContactsRandom_ExcludesArchivedAndOtherUsers(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/random", GetContactsRandom)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	other := models.User{Username: "other-random", Email: "other-random@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Visible"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Archived", Archived: true}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Theirs"}).Error)

	req, _ := http.NewRequest("GET", "/contacts/random", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, "Visible")
	assert.NotContains(t, body, "Archived", "an archived contact must not surface in Stay in Touch")
	assert.NotContains(t, body, "Theirs", "another user's contact must never surface")
}

func TestGetUpcomingBirthdays_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts/birthdays", GetUpcomingBirthdays)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	other := models.User{Username: "other-birthday", Email: "other-birthday@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)

	// A birthday a few days out, in --MM-DD form so it recurs annually.
	soon := time.Now().AddDate(0, 0, 3).Format("--01-02")
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Mine", Birthday: soon}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Theirs", Birthday: soon}).Error)

	req, _ := http.NewRequest("GET", "/contacts/birthdays", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, "Mine")
	assert.NotContains(t, body, "Theirs", "another user's birthday must never surface")
}
