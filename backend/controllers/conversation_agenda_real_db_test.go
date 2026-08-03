package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConversationAgenda_RealMigratedSchema is the real-DB check for T21:
// every other controller test runs against AutoMigrate on :memory: sqlite,
// which derives its schema from the same Go struct tags the application code
// uses — it cannot catch a GORM column-tag mismatch against the real
// hand-written migration SQL (this fork's own recurring bug class, e.g.
// ContactSyncLink.ETag). This test runs the full ConversationAgenda surface
// against a database.InitDB-migrated real file database instead: create ->
// list by entity -> update (discussed state preserved) -> mark discussed
// (with an activity link) -> still visible in the list -> soft-delete ->
// recreate.
func TestConversationAgenda_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agenda-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "agenda-realdb", Password: "password123!A", Email: "agenda-realdb@example.com"}
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
	router.POST("/conversation-agenda", withValidated(func() any { return &models.ConversationAgendaInput{} }), CreateConversationAgenda)
	router.GET("/conversation-agenda", ListConversationAgenda)
	router.PUT("/conversation-agenda/:id", withValidated(func() any { return &models.ConversationAgendaInput{} }), UpdateConversationAgenda)
	router.PATCH("/conversation-agenda/:id/discuss", withValidated(func() any { return &models.ConversationAgendaDiscussInput{} }), DiscussConversationAgenda)
	router.DELETE("/conversation-agenda/:id", DeleteConversationAgenda)

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

	// Create — must round-trip content + optional reference_url through the
	// real schema.
	createResp := doJSON("POST", "/conversation-agenda", models.ConversationAgendaInput{
		EntityID: contact.VCardUID, Content: "Ask about her mother's surgery", ReferenceURL: "https://example.com/article",
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		ConversationAgenda models.ConversationAgenda `json:"conversation_agenda"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	itemID := created.ConversationAgenda.ID
	require.NotEmpty(t, itemID)
	assert.Equal(t, "https://example.com/article", created.ConversationAgenda.ReferenceURL)

	// List by entity: one open item, not discussed.
	listResp := doJSON("GET", "/conversation-agenda?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		Items []models.ConversationAgenda `json:"conversation_agenda"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.Items, 1)
	assert.Nil(t, listed.Items[0].DiscussedAt)

	// Mark discussed with an activity link — the real discussed_at/activity_id
	// columns must resolve.
	activity := models.Activity{UserID: user.ID, Title: "Coffee catch-up", Date: time.Now(), Contacts: []models.Contact{contact}}
	require.NoError(t, db.Create(&activity).Error)

	discussResp := doJSON("PATCH", "/conversation-agenda/"+itemID+"/discuss", models.ConversationAgendaDiscussInput{ActivityID: &activity.ID})
	require.Equal(t, http.StatusOK, discussResp.Code, discussResp.Body.String())

	// Still visible in the list, now in a resolved state.
	listResp2 := doJSON("GET", "/conversation-agenda?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp2.Code, listResp2.Body.String())
	var listed2 struct {
		Items []models.ConversationAgenda `json:"conversation_agenda"`
	}
	require.NoError(t, json.Unmarshal(listResp2.Body.Bytes(), &listed2))
	require.Len(t, listed2.Items, 1, "discussed items must stay visible, not vanish")
	require.NotNil(t, listed2.Items[0].DiscussedAt)
	require.NotNil(t, listed2.Items[0].ActivityID)
	assert.Equal(t, activity.ID, *listed2.Items[0].ActivityID)

	// Update content — discussed state preserved through the real schema.
	updateResp := doJSON("PUT", "/conversation-agenda/"+itemID, models.ConversationAgendaInput{
		EntityID: contact.VCardUID, Content: "Ask about the surgery recovery",
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	var reloaded models.ConversationAgenda
	require.NoError(t, db.First(&reloaded, "id = ?", itemID).Error)
	assert.Equal(t, "Ask about the surgery recovery", reloaded.Content)
	require.NotNil(t, reloaded.DiscussedAt, "updating content must not re-open a discussed item")

	// Soft-delete: gone from the browse list but still present unscoped (the
	// change-feed tombstone / retention job's row).
	deleteResp := doJSON("DELETE", "/conversation-agenda/"+itemID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	var gone int64
	require.NoError(t, db.Model(&models.ConversationAgenda{}).Where("id = ?", itemID).Count(&gone).Error)
	assert.Zero(t, gone)
	var unscoped int64
	require.NoError(t, db.Unscoped().Model(&models.ConversationAgenda{}).Where("id = ?", itemID).Count(&unscoped).Error)
	assert.EqualValues(t, 1, unscoped)

	// Recreate for the same contact must not be blocked (no natural-key
	// unique constraint).
	recreateResp := doJSON("POST", "/conversation-agenda", models.ConversationAgendaInput{
		EntityID: contact.VCardUID, Content: "Ask about something new",
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
}
