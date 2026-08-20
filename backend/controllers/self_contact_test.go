package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// T90: every test in
// this file runs against a database.InitDB real migrated schema, not
// AutoMigrate — the self-contact column exists in migration 000018, and the
// whole point of the lazy-backfill half of the ticket is that it is a real
// migrated column. /CLAUDE.md backend trap #1.

// selfContactTestRouter wires a gin engine with the same context middleware
// the AuthMiddleware sets (db, userID, cfg) so the real handlers run as if
// authenticated. All other controller tests share setupRouter(); this file
// needs the real migrated schema, so it builds its own.
func selfContactTestRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", config.Config{ProfilePhotoDir: ""})
		c.Next()
	})
	return router
}

func selfContactTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "self-contact.db"))
	require.NoError(t, err)
	closeTestDBAtTeardown(t, db)
	return db
}

func patchSelfContact(t *testing.T, router *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest("PATCH", "/users/me/self-contact", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// The lazy backfill half of the ticket: an account created before migration
// 000018 has a NULL self_contact_vcard_uid forever (no endpoint ever wrote
// it), and GetCurrentUser must now fix that on the account's next /users/me.
func TestGetCurrentUser_LazyBackfillsSelfContact(t *testing.T) {
	db := selfContactTestDB(t)

	// Create the user directly (not via a registration path), so no
	// EnsureSelfContact call happens — this is the pre-000018 shape.
	user := models.User{Username: "legacy-user", Password: "password123!A", Email: "legacy@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.Nil(t, user.SelfContactVCardUID)

	router := selfContactTestRouter(t, db, user.ID)
	router.GET("/users/me", GetCurrentUser)

	req, _ := http.NewRequest("GET", "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.CurrentUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.SelfContactVCardUID, "a pre-000018 account must gain a self contact on /users/me")
	require.NotEmpty(t, *resp.SelfContactVCardUID)

	// The pointer must reference a real contact owned by the user.
	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, *resp.SelfContactVCardUID).First(&contact).Error)
	assert.Equal(t, "legacy-user", contact.Firstname, "the backfilled self contact is named after the username, matching EnsureSelfContact")
}

// GetCurrentUser must not clobber an already-set pointer — EnsureSelfContact
// is idempotent, and the backfill is unconditional by design.
func TestGetCurrentUser_LazyBackfillLeavesExistingSelfContact(t *testing.T) {
	db := selfContactTestDB(t)

	user := models.User{Username: "has-self", Password: "password123!A", Email: "has@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Give the user a real self contact pointing at a differently-named contact.
	existing := models.Contact{UserID: user.ID, Firstname: "My Real Self"}
	require.NoError(t, db.Create(&existing).Error)
	uid := existing.VCardUID
	user.SelfContactVCardUID = &uid
	require.NoError(t, db.Save(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.GET("/users/me", GetCurrentUser)

	req, _ := http.NewRequest("GET", "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.CurrentUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.SelfContactVCardUID)
	assert.Equal(t, uid, *resp.SelfContactVCardUID, "the existing pointer must survive the unconditional backfill call")

	// No duplicate contact may be created by the idempotent call.
	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestUpdateSelfContact_SetsOwnContact(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "picker", Password: "password123!A", Email: "picker@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "This Is Me"}
	require.NoError(t, db.Create(&contact).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	body, _ := json.Marshal(models.SelfContactInput{VCardUID: contact.VCardUID})
	w := patchSelfContact(t, router, string(body))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	require.NotNil(t, dbUser.SelfContactVCardUID)
	assert.Equal(t, contact.VCardUID, *dbUser.SelfContactVCardUID)
}

func TestUpdateSelfContact_ClearsWithNull(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "clearer", Password: "password123!A", Email: "clear@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Me"}
	require.NoError(t, db.Create(&contact).Error)
	uid := contact.VCardUID
	user.SelfContactVCardUID = &uid
	require.NoError(t, db.Save(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	w := patchSelfContact(t, router, `{"vcard_uid": null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	assert.Nil(t, dbUser.SelfContactVCardUID, "an explicit null must clear the link")
}

func TestUpdateSelfContact_ClearsWithEmptyString(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "clearer2", Password: "password123!A", Email: "clear2@example.com"}
	require.NoError(t, db.Create(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	w := patchSelfContact(t, router, `{"vcard_uid": ""}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	assert.Nil(t, dbUser.SelfContactVCardUID)
}

// Ownership scoping: a uid the caller doesn't own must be 404, never a 200
// (/CLAUDE.md backend trap #5).
func TestUpdateSelfContact_RejectsForeignUID(t *testing.T) {
	db := selfContactTestDB(t)
	caller := models.User{Username: "caller", Password: "password123!A", Email: "caller@example.com"}
	other := models.User{Username: "other", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&caller).Error)
	require.NoError(t, db.Create(&other).Error)

	othersContact := models.Contact{UserID: other.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	router := selfContactTestRouter(t, db, caller.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	body, _ := json.Marshal(models.SelfContactInput{VCardUID: othersContact.VCardUID})
	w := patchSelfContact(t, router, string(body))

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, caller.ID).Error)
	assert.Nil(t, dbUser.SelfContactVCardUID, "a rejected foreign uid must not change the pointer")
}

func TestUpdateSelfContact_RejectsUnknownUID(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "phantom", Password: "password123!A", Email: "phantom@example.com"}
	require.NoError(t, db.Create(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	missing := "11111111-2222-4333-8444-555555555555"
	body, _ := json.Marshal(models.SelfContactInput{VCardUID: missing})
	w := patchSelfContact(t, router, string(body))

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// A uid that isn't even shaped like a VCardUID (which is always uuid v4) is
// rejected by the DTO's uuid4 tag before the ownership lookup — 400, not 404.
func TestUpdateSelfContact_RejectsMalformedUID(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "malformed", Password: "password123!A", Email: "malformed@example.com"}
	require.NoError(t, db.Create(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)

	w := patchSelfContact(t, router, `{"vcard_uid": "definitely-not-a-uuid"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	assert.Nil(t, dbUser.SelfContactVCardUID, "a rejected uid must not change the pointer")
}

// The badge logic's backend half: deleting the self contact must clear the
// pointer rather than leave it dangling on a soft-deleted row.
func TestDeleteContact_NullsSelfContactPointer(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "deleter", Password: "password123!A", Email: "delete@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "About To Die"}
	require.NoError(t, db.Create(&contact).Error)
	uid := contact.VCardUID
	user.SelfContactVCardUID = &uid
	require.NoError(t, db.Save(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.DELETE("/contacts/:id", DeleteContact)

	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.FormatUint(uint64(contact.ID), 10), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	assert.Nil(t, dbUser.SelfContactVCardUID, "deleting the self contact must null the pointer")
}

// The ticket's second dangling-pointer hazard, end-to-end through the real
// merge commit path: when "Me" is the merge loser, RepointContactAssociations
// moves the pointer onto the keeper, and the deleteContactAssociations sweep
// that follows must not clobber it back to NULL. The service-level repoint
// test pins the function; this pins the controller's ordering of the two.
func TestContactMerge_RepointsSelfContactPointerToKeeper(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "merge-self", Password: "password123!A", Email: "merge-self@example.com"}
	require.NoError(t, db.Create(&user).Error)

	keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
	loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
	require.NoError(t, db.Create(&keeper).Error)
	require.NoError(t, db.Create(&loser).Error)

	// "Me" is the loser.
	uid := loser.VCardUID
	user.SelfContactVCardUID = &uid
	require.NoError(t, db.Save(&user).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	body, _ := json.Marshal(models.ContactMergeRequest{
		KeepID:      keeper.ID,
		MergeID:     loser.ID,
		Resolutions: map[string]string{"firstname": "Keeper"},
	})
	req, err := http.NewRequest("POST", "/contacts/merge", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dbUser models.User
	require.NoError(t, db.First(&dbUser, user.ID).Error)
	require.NotNil(t, dbUser.SelfContactVCardUID)
	assert.Equal(t, keeper.VCardUID, *dbUser.SelfContactVCardUID,
		"merging the self contact away must repoint the pointer to the keeper, not clear it")

	var loserRow models.Contact
	require.NoError(t, db.Unscoped().First(&loserRow, loser.ID).Error)
	assert.NotNil(t, loserRow.DeletedAt, "the loser must be soft-deleted by the merge")
	assert.NotEqual(t, loserRow.VCardUID, *dbUser.SelfContactVCardUID,
		"the pointer must never reference the soft-deleted loser")
}
