package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGift_RealMigratedSchema is the real-DB check for T20b: every other
// controller test runs against AutoMigrate on:memory: sqlite, which derives
// its schema from the same Go struct tags the application code uses — it
// cannot catch a GORM column-tag mismatch against the real hand-written
// migration SQL (this fork's own recurring bug class, e.g. ContactSyncLink.
// ETag and ConversationAgenda's table-name pluralization). This test runs the
// full Gift surface against a database.InitDB-migrated real file database
// instead: create (idea, then a given gift with value/currency) -> list by
// entity -> update -> link a life event and activity -> soft-delete ->
// recreate for the same contact.
func TestGift_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gift-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "gift-realdb", Password: "password123!A", Email: "gift-realdb@example.com"}
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
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)
	router.GET("/gifts", ListGifts)
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)
	router.DELETE("/gifts/:id", DeleteGift)

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

	// Create an idea — status must default to idea through the real schema.
	createResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "She liked the ceramics shop",
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		Gift models.Gift `json:"gift"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	giftID := created.Gift.ID
	require.NotEmpty(t, giftID)
	assert.Equal(t, models.GiftStatusIdea, created.Gift.Status)

	// List by entity: one open idea.
	listResp := doJSON("GET", "/gifts?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		Items []models.Gift `json:"gifts"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.Items, 1)
	assert.Equal(t, models.GiftStatusIdea, listed.Items[0].Status)

	// A given gift with value + explicit currency + occasion + references must
	// round-trip through the real value_cents/currency/life_event_id/activity_id
	// columns.
	lifeEvent := models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "married"}
	require.NoError(t, db.Create(&lifeEvent).Error)
	activity := models.Activity{UserID: user.ID, Title: "Wedding", Date: time.Now(), Contacts: []models.Contact{contact}}
	require.NoError(t, db.Create(&activity).Error)

	date := time.Now().Truncate(time.Second)
	updateResp := doJSON("PUT", "/gifts/"+giftID, models.GiftInput{
		EntityID: contact.VCardUID, Status: models.GiftStatusGiven,
		Occasion: "their wedding", Description: "The espresso machine",
		Date: &date, ValueCents: 12000, Currency: "EUR",
		LifeEventID: lifeEvent.ID, ActivityID: &activity.ID,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())

	var reloaded models.Gift
	require.NoError(t, db.First(&reloaded, "id = ?", giftID).Error)
	assert.Equal(t, models.GiftStatusGiven, reloaded.Status)
	assert.Equal(t, "their wedding", reloaded.Occasion)
	assert.EqualValues(t, 12000, reloaded.ValueCents)
	assert.Equal(t, "EUR", reloaded.Currency)
	assert.Equal(t, lifeEvent.ID, reloaded.LifeEventID)
	require.NotNil(t, reloaded.ActivityID)
	assert.Equal(t, activity.ID, *reloaded.ActivityID)
	require.NotNil(t, reloaded.Date)
	assert.Equal(t, date.Unix(), reloaded.Date.Unix())

	// Soft-delete: gone from the browse list but still present unscoped (the
	// change-feed tombstone / retention job's row).
	deleteResp := doJSON("DELETE", "/gifts/"+giftID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	var gone int64
	require.NoError(t, db.Model(&models.Gift{}).Where("id = ?", giftID).Count(&gone).Error)
	assert.Zero(t, gone)
	var unscoped int64
	require.NoError(t, db.Unscoped().Model(&models.Gift{}).Where("id = ?", giftID).Count(&unscoped).Error)
	assert.EqualValues(t, 1, unscoped)

	// Recreate for the same contact must not be blocked (no natural-key unique
	// constraint).
	recreateResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "A new idea",
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
}

// TestGift_URLAndNotes_RealMigratedSchema is T35's real-DB round trip for the
// two columns added by migration 000012. It runs against database.InitDB
// rather than AutoMigrate for the usual reason (CLAUDE.md trap 1): `URL` is
// exactly the acronym-cased field whose GORM-derived column name can silently
// disagree with the hand-written SQL — AutoMigrate would happily create
// whatever GORM derived and the mismatch would never surface.
//
// It also uses the real ValidateJSONMiddleware, not the withValidated test
// shim, so the `safeurl` validator genuinely runs on the unsafe-scheme case.
func TestGift_URLAndNotes_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gift-url-notes.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "gift-urlnotes", Password: "password123!A", Email: "gift-urlnotes@example.com"}
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
	router.POST("/gifts", middleware.ValidateJSONMiddleware(&models.GiftInput{}), CreateGift)
	router.GET("/gifts", ListGifts)
	router.PUT("/gifts/:id", middleware.ValidateJSONMiddleware(&models.GiftInput{}), UpdateGift)

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

	const productURL = "https://shop.example.com/handmade-ceramic-mug"
	const notes = "Size medium. Saw it at the Saturday market — check she hasn't bought it herself."

	createResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "The ceramic mug she liked",
		URL: productURL, Notes: notes,
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		Gift models.Gift `json:"gift"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	giftID := created.Gift.ID
	assert.Equal(t, productURL, created.Gift.URL)
	assert.Equal(t, notes, created.Gift.Notes)

	// Read back from the real columns, not from the create response — a GORM
	// column-name mismatch would leave these empty here while the response
	// echoed the in-memory struct just fine.
	var persisted models.Gift
	require.NoError(t, db.First(&persisted, "id = ?", giftID).Error)
	assert.Equal(t, productURL, persisted.URL)
	assert.Equal(t, notes, persisted.Notes)

	// Pin the physical column names the migration created: `url`/`notes`, not
	// GORM's derived spelling for an acronym field.
	var raw struct {
		URL   string
		Notes string
	}
	require.NoError(t, db.Raw("SELECT url, notes FROM gifts WHERE id = ?", giftID).Scan(&raw).Error)
	assert.Equal(t, productURL, raw.URL)
	assert.Equal(t, notes, raw.Notes)

	// The list surface must carry both fields too — it is what the contact
	// page renders from.
	listResp := doJSON("GET", "/gifts?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		Items []models.Gift `json:"gifts"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.Items, 1)
	assert.Equal(t, productURL, listed.Items[0].URL)
	assert.Equal(t, notes, listed.Items[0].Notes)

	// Update must be able to *set* both fields, not just carry them from
	// create — assigning them in UpdateGift is its own line of code, and
	// without this the clearing assertion below passes even if update hard-
	// codes them empty.
	const editedURL = "https://shop.example.com/handmade-ceramic-mug-blue"
	const editedNotes = "She picked the blue one in the end."
	editResp := doJSON("PUT", "/gifts/"+giftID, models.GiftInput{
		EntityID: contact.VCardUID, Description: "The ceramic mug she liked",
		URL: editedURL, Notes: editedNotes,
	})
	require.Equal(t, http.StatusOK, editResp.Code, editResp.Body.String())
	require.NoError(t, db.First(&persisted, "id = ?", giftID).Error)
	assert.Equal(t, editedURL, persisted.URL, "update must persist a changed url")
	assert.Equal(t, editedNotes, persisted.Notes, "update must persist changed notes")

	// Update is full-replace (UpdateGift's documented semantics): omitting
	// url/notes clears them rather than preserving the old values.
	updateResp := doJSON("PUT", "/gifts/"+giftID, models.GiftInput{
		EntityID: contact.VCardUID, Description: "The ceramic mug she liked",
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	require.NoError(t, db.First(&persisted, "id = ?", giftID).Error)
	assert.Empty(t, persisted.URL, "full-replace update must clear an omitted url")
	assert.Empty(t, persisted.Notes, "full-replace update must clear omitted notes")

	// A javascript: URL is rejected by the httpurl validator at the binding
	// layer, so it never reaches the database.
	unsafeResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "Malicious", URL: "javascript:alert(1)",
	})
	require.Equal(t, http.StatusBadRequest, unsafeResp.Code, unsafeResp.Body.String())

	// …and so is an over-long URL/notes value.
	tooLongResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "Too long",
		URL: "https://example.com/" + strings.Repeat("a", 2000),
	})
	require.Equal(t, http.StatusBadRequest, tooLongResp.Code, tooLongResp.Body.String())

	tooLongNotesResp := doJSON("POST", "/gifts", models.GiftInput{
		EntityID: contact.VCardUID, Description: "Too long", Notes: strings.Repeat("n", 2001),
	})
	require.Equal(t, http.StatusBadRequest, tooLongNotesResp.Code, tooLongNotesResp.Body.String())

	var total int64
	require.NoError(t, db.Model(&models.Gift{}).Count(&total).Error)
	assert.EqualValues(t, 1, total, "no rejected payload may have been persisted")
}
