package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// contactShareRouterFor builds a router bound to db, acting as userID. Unlike
// setupRouter (which hardcodes acting-user to the single seeded "tester"),
// contact-share tests genuinely need to issue requests as different actors
// (sender, recipient, an uninvolved third party) against the SAME db to
// prove ownership scoping — so each test builds one router per actor it
// needs and shares the underlying db across them.
func contactShareRouterFor(db *gorm.DB, cfg *config.Config, userID uint) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.GET("/users/directory", ListUserDirectory)
	router.POST("/contact-shares", withValidated(func() any { return &models.ContactShareInput{} }), CreateContactShare)
	router.GET("/contact-shares/incoming", ListIncomingContactShares)
	router.GET("/contact-shares/outgoing", ListOutgoingContactShares)
	router.POST("/contact-shares/:id/accept", AcceptContactShare)
	router.POST("/contact-shares/:id/confirm", withValidated(func() any { return &models.ImportConfirmRequest{} }), func(c *gin.Context) {
		ConfirmContactShare(c, cfg)
	})
	router.POST("/contact-shares/:id/decline", DeclineContactShare)
	return router
}

// newContactShareFixtures returns a migrated db (reusing setupRouter's
// migration list, ignoring its default router/user) plus three fresh users:
// sender, recipient, and an uninvolved third party.
func newContactShareFixtures(t *testing.T) (*gorm.DB, *config.Config, models.User, models.User, models.User) {
	t.Helper()
	db, _ := setupRouter()
	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}

	sender := models.User{Username: "share_sender", Password: "password123", Email: "share_sender@example.com"}
	require.NoError(t, db.Create(&sender).Error)
	recipient := models.User{Username: "share_recipient", Password: "password123", Email: "share_recipient@example.com"}
	require.NoError(t, db.Create(&recipient).Error)
	third := models.User{Username: "share_third", Password: "password123", Email: "share_third@example.com"}
	require.NoError(t, db.Create(&third).Error)

	return db, cfg, sender, recipient, third
}

// createPendingShareFixture creates a contact owned by fromID and shares it
// with toID through the real CreateContactShare handler (not a hand-crafted
// DB row), so the resulting Payload is a genuine JSContact export the rest
// of the pipeline can parse.
func createPendingShareFixture(t *testing.T, db *gorm.DB, cfg *config.Config, fromID, toID uint, firstname string, sections []string, includeSensitive bool) models.ContactShare {
	t.Helper()
	contact := models.Contact{UserID: fromID, Firstname: firstname, Lastname: "Anderson", Email: "shared@example.com", Phone: "555-1111"}
	require.NoError(t, db.Create(&contact).Error)

	router := contactShareRouterFor(db, cfg, fromID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: toID, VCardUID: contact.VCardUID, Sections: sections, IncludeSensitive: includeSensitive,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", fromID, toID).Order("created_at DESC").First(&share).Error)
	return share
}

func decodeSharePayloadCard(t *testing.T, payload string) map[string]any {
	t.Helper()
	var cards []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &cards))
	require.Len(t, cards, 1)
	var card map[string]any
	require.NoError(t, json.Unmarshal(cards[0], &card))
	return card
}

func doJSON(t *testing.T, router *gin.Engine, method, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
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

func TestCreateContactShare_OnlySelectedSectionsPresent(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com", Phone: "555-0100"}
	require.NoError(t, db.Create(&contact).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID,
		VCardUID: contact.VCardUID,
		Sections: []string{models.SectionEmails},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
	assert.Equal(t, "Alice Anderson", share.ContactDisplayName)
	assert.Equal(t, models.ContactShareStatusPending, share.Status)

	card := decodeSharePayloadCard(t, share.Payload)
	assert.Contains(t, card, "emails")
	assert.NotContains(t, card, "phones")
}

func TestCreateContactShare_SensitiveExcludedByDefaultIncludedWhenOptedIn(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson"}
	require.NoError(t, db.Create(&contact).Error)
	other := models.Contact{UserID: sender.ID, Firstname: "Bob", Lastname: "Brown"}
	require.NoError(t, db.Create(&other).Error)

	edge := models.RelationshipEdge{
		UserID: sender.ID, SourceID: contact.VCardUID, TargetID: other.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityPrivate,
	}
	require.NoError(t, db.Create(&edge).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)

	// Default: no include_sensitive -> the private edge must not appear.
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID,
		VCardUID: contact.VCardUID,
		Sections: []string{models.SectionRelatedTo},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var shares []models.ContactShare
	require.NoError(t, db.Where("from_user_id = ?", sender.ID).Find(&shares).Error)
	require.Len(t, shares, 1)
	card := decodeSharePayloadCard(t, shares[0].Payload)
	assert.NotContains(t, card, "relatedTo", "private edge must be excluded without explicit opt-in")

	// Explicit opt-in -> the private edge must appear.
	w = doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID:         recipient.ID,
		VCardUID:         contact.VCardUID,
		Sections:         []string{models.SectionRelatedTo},
		IncludeSensitive: true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.Where("from_user_id = ?", sender.ID).Find(&shares).Error)
	require.Len(t, shares, 2)
	card = decodeSharePayloadCard(t, shares[1].Payload)
	assert.Contains(t, card, "relatedTo", "private edge must appear with explicit opt-in")
}

func TestCreateContactShare_RejectsForeignContact(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)

	othersContact := models.Contact{UserID: third.ID, Firstname: "Not", Lastname: "Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID,
		VCardUID: othersContact.VCardUID,
		Sections: []string{models.SectionEmails},
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateContactShare_RejectsSelfShare(t *testing.T) {
	db, cfg, sender, _, _ := newContactShareFixtures(t)
	contact := models.Contact{UserID: sender.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: sender.ID,
		VCardUID: contact.VCardUID,
		Sections: []string{models.SectionEmails},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateContactShare_RejectsUnknownRecipient(t *testing.T) {
	db, cfg, sender, _, _ := newContactShareFixtures(t)
	contact := models.Contact{UserID: sender.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: 999999,
		VCardUID: contact.VCardUID,
		Sections: []string{models.SectionEmails},
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListContactShares_ScopingAndThirdUserSeesNeither(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)
	_ = share

	// Recipient sees it incoming.
	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "GET", "/contact-shares/incoming", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		ContactShares []models.ContactShare `json:"contact_shares"`
		Usernames     map[string]string     `json:"usernames"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.ContactShares, 1)
	assert.Equal(t, sender.Username, resp.Usernames[strconv.Itoa(int(sender.ID))])

	// Recipient's outgoing is empty.
	w = doJSON(t, recipientRouter, "GET", "/contact-shares/outgoing", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.ContactShares)

	// Sender sees it outgoing.
	senderRouter := contactShareRouterFor(db, cfg, sender.ID)
	w = doJSON(t, senderRouter, "GET", "/contact-shares/outgoing", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.ContactShares, 1)

	// A third, uninvolved user sees neither side.
	thirdRouter := contactShareRouterFor(db, cfg, third.ID)
	w = doJSON(t, thirdRouter, "GET", "/contact-shares/incoming", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.ContactShares)

	w = doJSON(t, thirdRouter, "GET", "/contact-shares/outgoing", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.ContactShares)
}

func TestAcceptContactShare_WrongUserNotFound(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	thirdRouter := contactShareRouterFor(db, cfg, third.ID)
	w := doJSON(t, thirdRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAcceptContactShare_AlreadyRespondedConflict(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)
	share.Status = models.ContactShareStatusDeclined
	require.NoError(t, db.Save(&share).Error)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAcceptContactShare_DetectsDuplicate(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	// Recipient already has a contact matching by email.
	existing := models.Contact{UserID: recipient.ID, Firstname: "Ally", Lastname: "Existing", Email: "shared@example.com"}
	require.NoError(t, db.Create(&existing).Error)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Len(t, preview.Rows, 1)
	require.NotNil(t, preview.Rows[0].DuplicateMatch)
	assert.Equal(t, existing.ID, preview.Rows[0].DuplicateMatch.ExistingContactID)
	assert.Equal(t, "update", preview.Rows[0].SuggestedAction)

	// Status must still be pending -- accept is preview-only.
	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, models.ContactShareStatusPending, reloaded.Status)
}

// TestConfirmContactShare_RejectsSessionFromAnotherShare is the regression
// test for a real gap found in review: without SessionBelongsToShare,
// ConfirmContactShare would accept any session belonging to the recipient
// -- including the preview session from a DIFFERENT pending share -- and
// still flip the requested share to accepted, decoupling its status from
// the data that actually landed. Not cross-user (both shares belong to the
// same recipient here), but a real same-account integrity gap.
func TestConfirmContactShare_RejectsSessionFromAnotherShare(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share1 := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)
	share2 := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Bob", []string{models.SectionEmails}, false)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)

	// Accept share2 to mint a session bound to share2, not share1.
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share2.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var preview2 models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview2))

	// Attempt to confirm share1 using share2's session.
	w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share1.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: preview2.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// share1 must still be pending -- nothing should have been imported or
	// flipped using the wrong session.
	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share1.ID).Error)
	assert.Equal(t, models.ContactShareStatusPending, reloaded.Status)

	var aliceCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ? AND firstname = ?", recipient.ID, "Alice").Count(&aliceCount).Error)
	assert.Zero(t, aliceCount, "share1's contact must not have been created via share2's session")
}

func TestConfirmContactShare_AddCreatesContactWithSelectedFieldsOnly(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Len(t, preview.Rows, 1)

	w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Created)

	var created models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", recipient.ID, "Alice").First(&created).Error)
	assert.Equal(t, "shared@example.com", created.Email)
	assert.Empty(t, created.Phone, "phone was not a selected section and must be absent")

	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, models.ContactShareStatusAccepted, reloaded.Status)
	assert.NotNil(t, reloaded.RespondedAt)
}

func TestConfirmContactShare_UpdateMergesIntoExistingMatch(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	existing := models.Contact{UserID: recipient.ID, Firstname: "Ally", Lastname: "Existing", Email: "shared@example.com"}
	require.NoError(t, db.Create(&existing).Error)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))

	w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, 0, result.Created, "update must merge into the existing row, not create a second one")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ? AND email = ?", recipient.ID, "shared@example.com").Count(&count).Error)
	assert.Equal(t, int64(1), count, "no duplicate contact should exist after an update action")
}

func TestDeclineContactShare_FlipsStatusAndPreservesSender(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/decline", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, models.ContactShareStatusDeclined, reloaded.Status)
	assert.NotNil(t, reloaded.RespondedAt)
	// The sender's copy (the share row itself, and their original contact)
	// survives untouched -- declining must not delete anything.
	assert.NotEmpty(t, reloaded.Payload)

	var senderContactCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", sender.ID).Count(&senderContactCount).Error)
	assert.Equal(t, int64(1), senderContactCount)

	// Declining again must 409, not silently succeed.
	w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/decline", nil)
	assert.Equal(t, http.StatusConflict, w.Code)
}
