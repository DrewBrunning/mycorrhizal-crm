package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- FavoriteContact / UnfavoriteContact (issue #173) ------------------------
//
// Mirrors the ArchiveContact/UnarchiveContact tests in
// uncovered_endpoints_test.go: flag toggling, 404 on unknown id, ownership
// scoping (IDOR guard), the change-feed/ETag propagation, and a real-schema
// persistence check.

func TestFavoriteContact_SetsFlag(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/favorite", FavoriteContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Star"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(contact.ID)+"/favorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.True(t, reloaded.IsFavorite)
}

func TestUnfavoriteContact_ClearsFlag(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/unfavorite", UnfavoriteContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Star", IsFavorite: true}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(contact.ID)+"/unfavorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.False(t, reloaded.IsFavorite)
}

func TestFavoriteContact_UnknownID_404(t *testing.T) {
	_, router := setupRouter()
	router.POST("/contacts/:id/favorite", FavoriteContact)

	req, _ := http.NewRequest("POST", "/contacts/999999/favorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUnfavoriteContact_UnknownID_404(t *testing.T) {
	_, router := setupRouter()
	router.POST("/contacts/:id/unfavorite", UnfavoriteContact)

	req, _ := http.NewRequest("POST", "/contacts/999999/unfavorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFavoriteContact_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/favorite", FavoriteContact)

	other := models.User{Username: "other-favorite", Email: "other-favorite@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&theirs).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(theirs.ID)+"/favorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, theirs.ID).Error)
	assert.False(t, reloaded.IsFavorite, "another user's contact must not be favoritable")
}

func TestUnfavoriteContact_ScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts/:id/unfavorite", UnfavoriteContact)

	other := models.User{Username: "other-unfavorite", Email: "other-unfavorite@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Not Yours", IsFavorite: true}
	require.NoError(t, db.Create(&theirs).Error)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(theirs.ID)+"/unfavorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestFavoriteContact_BumpsETagAndChangeFeed pins the hook-firing choice: a
// favorite flip must bump the ETag (so CardDAV sync sees a change) and
// surface in the T17 ?since= change feed — a favorite is a change of the
// contact, and a replica needs to see it. This is exactly why the handlers
// use `Update` rather than `UpdateColumn` (which skips hooks).
func TestFavoriteContact_BumpsETagAndChangeFeed(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.POST("/contacts/:id/favorite", FavoriteContact)
	router.POST("/contacts/:id/unfavorite", UnfavoriteContact)
	router.GET("/contacts", GetContacts)

	contact := models.Contact{UserID: user.ID, Firstname: "Feed"}
	require.NoError(t, db.Create(&contact).Error)
	// The ETag is second-precision (e-<id>-<updated_at unix>), so a favorite
	// update landing in the SAME wall-clock second as the create computes the
	// same ETag and AfterSave would skip rewriting it. Overwrite the stored
	// etag with a sentinel (UpdateColumn skips hooks, so this is inert) —
	// the favorite's hook-firing Update must then recompute it to the real
	// value, which is exactly the mechanism this test pins.
	require.NoError(t, db.Model(&contact).UpdateColumn("etag", "e-stale-sentinel").Error)
	require.NoError(t, db.First(&contact, contact.ID).Error)
	etagBefore := contact.ETag
	require.Equal(t, "e-stale-sentinel", etagBefore)

	// Cursor positioned at the contact's creation.
	cursor := EncodeCursor(contact.UpdatedAt, contact.ID)

	req, _ := http.NewRequest("POST", "/contacts/"+idString(contact.ID)+"/favorite", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.True(t, reloaded.IsFavorite)
	assert.NotEqual(t, etagBefore, reloaded.ETag, "favoriting must bump the ETag (hooks fire)")

	feed := getContactsPage(t, router, "since="+cursor+"&limit=10")
	require.Len(t, feed.Contacts, 1, "the favorite flip must appear in the change feed")
	assert.Equal(t, "Feed", feed.Contacts[0]["firstname"])
	assert.Equal(t, true, feed.Contacts[0]["is_favorite"], "the feed row must carry the updated flag")
}

// TestFavoriteFlag_RealMigratedSchema is the real-DB check for the recurring
// GORM-vs-migration column-name drift trap (CLAUDE.md backend trap 1): the
// `is_favorite` column must exist with exactly that name in the hand-written
// migration SQL, and the flag must round-trip through a real
// database.InitDB-migrated file DB.
func TestFavoriteFlag_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "favorite-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "realdb-fav", Password: "password123!A", Email: "realdb-fav@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Real", IsFavorite: true}
	require.NoError(t, db.Create(&contact).Error)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.True(t, reloaded.IsFavorite, "is_favorite must persist through the real migrated schema")

	// Unfavorite back and confirm it round-trips too.
	require.NoError(t, db.Model(&reloaded).Update("is_favorite", false).Error)
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.False(t, reloaded.IsFavorite)
}

// --- GET /contacts?favorites=true ---------------------------------------------

func TestGetContacts_FavoritesFilter(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts", GetContacts)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	fav := models.Contact{UserID: user.ID, Firstname: "Fav", IsFavorite: true}
	plain := models.Contact{UserID: user.ID, Firstname: "Plain"}
	archivedFav := models.Contact{UserID: user.ID, Firstname: "ArchFav", IsFavorite: true, Archived: true}
	require.NoError(t, db.Create(&fav).Error)
	require.NoError(t, db.Create(&plain).Error)
	require.NoError(t, db.Create(&archivedFav).Error)

	req, _ := http.NewRequest("GET", "/contacts?favorites=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Contacts []models.ContactSummary `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Contacts, 1, "only the live favorite should match")
	assert.Equal(t, "Fav", resp.Contacts[0].Firstname)
	assert.True(t, resp.Contacts[0].IsFavorite)
}

// TestGetContactsSummary_AlwaysSerializesIsFavorite asserts the raw JSON: a
// non-favorite contact must still carry `"is_favorite":false` on the wire (no
// omitempty) — decoding into the Go struct makes absent and false
// indistinguishable (CLAUDE.md frontend trap 8), so this reads the raw bytes.
func TestGetContactsSummary_AlwaysSerializesIsFavorite(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts", GetContacts)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Plain"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"is_favorite":false`, "a non-favorite must serialize is_favorite:false, not omit it")
	assert.False(t, strings.Contains(body, `"is_favorite":null`))
}

// TestGetContacts_FavoritesFilterComposesWithArchived pins that favorites
// composes with the other predicates like every other filter — a user asking
// for favorites while browsing archived contacts gets their archived
// favorites, not an empty list.
func TestGetContacts_FavoritesFilterComposesWithArchived(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts", GetContacts)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	liveFav := models.Contact{UserID: user.ID, Firstname: "Live", IsFavorite: true}
	archivedFav := models.Contact{UserID: user.ID, Firstname: "Arch", IsFavorite: true, Archived: true}
	require.NoError(t, db.Create(&liveFav).Error)
	require.NoError(t, db.Create(&archivedFav).Error)

	req, _ := http.NewRequest("GET", "/contacts?favorites=true&archived=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Contacts []models.ContactSummary `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, "Arch", resp.Contacts[0].Firstname)
}
