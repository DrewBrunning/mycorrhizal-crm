package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactShare_RealMigratedSchema is P1's real-DB check (CLAUDE.md trap
// #1): every other contact-share test runs against AutoMigrate on :memory:
// sqlite, which derives its schema from the same Go struct tags the app code
// uses and so cannot catch a GORM column-name mismatch against the real
// hand-written migration (000008_contact_shares) — this fork's own recurring
// bug class (ContactSyncLink.ETag shipped broken this way). Runs the full
// create -> accept -> confirm round trip against a database.InitDB-migrated
// real file database instead.
func TestContactShare_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-share-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	sender := models.User{Username: "share-realdb-sender", Password: "password123!A", Email: "share-realdb-sender@example.com"}
	require.NoError(t, db.Create(&sender).Error)
	recipient := models.User{Username: "share-realdb-recipient", Password: "password123!A", Email: "share-realdb-recipient@example.com"}
	require.NoError(t, db.Create(&recipient).Error)

	contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice-realdb@example.com", Phone: "555-9999"}
	require.NoError(t, db.Create(&contact).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	senderRouter := contactShareRouterFor(db, cfg, sender.ID)
	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)

	doJSONReal := func(router *gin.Engine, method, url string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, url, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Create: exercises from_user_id/to_user_id/contact_display_name/payload/
	// status against the real migrated columns.
	w := doJSONReal(senderRouter, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID, VCardUID: contact.VCardUID, Sections: []string{models.SectionEmails},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
	assert.Equal(t, "Alice Anderson", share.ContactDisplayName)
	assert.Equal(t, models.ContactShareStatusPending, share.Status)
	assert.Nil(t, share.RespondedAt)

	// List incoming/outgoing round-trip through the real schema too.
	w = doJSONReal(recipientRouter, "GET", "/contact-shares/incoming", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listResp struct {
		ContactShares []models.ContactShare `json:"contact_shares"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.ContactShares, 1)

	// Accept: parse the real persisted payload column back through the
	// import pipeline.
	w = doJSONReal(recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Len(t, preview.Rows, 1)

	// Confirm: real DeclineContactShare/ConfirmContactShare row updates
	// (status, responded_at) against the real schema.
	w = doJSONReal(recipientRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Created)

	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, models.ContactShareStatusAccepted, reloaded.Status)
	require.NotNil(t, reloaded.RespondedAt)

	var createdContact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", recipient.ID, "Alice").First(&createdContact).Error)
	assert.Equal(t, "alice-realdb@example.com", createdContact.Email)
}
