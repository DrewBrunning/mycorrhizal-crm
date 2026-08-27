package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T96:
// Android's device-contacts import must get the same server-side
// merge/keep-both/discard machinery as every other import path. These tests
// cover POST /contacts/import/records — the endpoint that feeds a batch of
// neutral Card/CRM records through the standard preview pipeline — against
// the real migrated schema (CLAUDE.md trap 1).

// recordsFixture builds a minimal neutral Card/CRM record payload (the same
// nested shape POST /contacts accepts; Android's DeviceContactMapper produces
// this for a device contact).
func recordsFixture(records ...contactmodel.Card) []byte {
	payload := models.ImportRecordsRequest{}
	for _, card := range records {
		payload.Records = append(payload.Records, models.ContactRecordInput{Card: card})
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func cardFixture(given, surname, email, phone string) contactmodel.Card {
	card := contactmodel.Card{
		Name: &contactmodel.Name{
			Full: given + " " + surname,
			Components: []contactmodel.NameComponent{
				{Kind: "given", Value: given},
				{Kind: "surname", Value: surname},
			},
		},
	}
	if email != "" {
		card.Emails = []contactmodel.Email{{Address: email}}
	}
	if phone != "" {
		card.Phones = []contactmodel.Phone{{Number: phone}}
	}
	return card
}

func TestUploadImportRecords_PreviewDetectsDuplicateAndComputesDiff(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96records", Password: "password123!A", Email: "t96records@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
	}
	require.NoError(t, db.Create(existing).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	// The device contact matches by email and carries a phone the existing
	// contact does not have.
	body := recordsFixture(cardFixture("Jane", "Smith", "jane@example.com", "+15559998888"))
	req := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Rows, 1)
	require.NotNil(t, resp.Rows[0].DuplicateMatch)
	assert.Equal(t, existing.ID, resp.Rows[0].DuplicateMatch.ExistingContactID)
	assert.Equal(t, "email", resp.Rows[0].DuplicateMatch.MatchReason)
	assert.Equal(t, "update", resp.Rows[0].SuggestedAction)

	require.NotNil(t, resp.Rows[0].MergeDiff, "a DB duplicate row must carry its merge diff")
	addedPhone := ""
	for _, a := range resp.Rows[0].MergeDiff.Added {
		if a.Kind == "phone" {
			addedPhone = a.Value
		}
	}
	assert.Equal(t, "+15559998888", addedPhone, "the device contact's new phone must appear in the diff")
}

// TestUploadImportRecords_MergeDiffArraysSerializeAsEmptyArrays pins CLAUDE.md
// frontend trap #8 on the NEW diff fields specifically: a diff with no scalar
// updates (only an added phone) must serialize `updated` as `[]`, never `null`
// — the client renders `diff.updated.length` directly. Decoding into the Go
// struct can't see the difference (absent and `[]` both become a nil slice),
// so this reads the raw JSON body.
func TestUploadImportRecords_MergeDiffArraysSerializeAsEmptyArrays(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96recordsn", Password: "password123!A", Email: "t96recordsn@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane-null@example.com",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane-null@example.com"}},
	}
	require.NoError(t, db.Create(existing).Error)

	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, &config.Config{})

	// Same email as the existing contact (matched) plus a brand-new phone — so
	// the diff has an addition but no scalar updates.
	body := recordsFixture(cardFixture("Jane", "Smith", "jane-null@example.com", "+15559998888"))
	req := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw struct {
		Rows []struct {
			MergeDiff *struct {
				Updated []json.RawMessage `json:"updated"`
				Added   []json.RawMessage `json:"added"`
			} `json:"merge_diff"`
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Len(t, raw.Rows, 1)
	require.NotNil(t, raw.Rows[0].MergeDiff)
	require.NotNil(t, raw.Rows[0].MergeDiff.Updated, "updated must be present as [] on the wire, never absent")
	require.NotNil(t, raw.Rows[0].MergeDiff.Added, "added must be present as [] on the wire, never absent")
	require.Len(t, raw.Rows[0].MergeDiff.Added, 1, "the new phone must be the one addition")
}

func TestUploadImportRecords_ConfirmViaVCFRouteMergesIntoExisting(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96recordsm", Password: "password123!A", Email: "t96recordsm@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-0000"}},
	}
	require.NoError(t, db.Create(existing).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	body := recordsFixture(cardFixture("Jane", "Smith", "jane@example.com", "+15559998888"))
	uploadReq := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))

	// Confirm through the shared VCF confirm route, exactly as the client does.
	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)

	// The merge must have appended the new phone while keeping the old one
	// (additive T49 union), and must not have created a second contact.
	var persisted models.Contact
	require.NoError(t, db.First(&persisted, existing.ID).Error)
	phoneValues := make([]string, 0, len(persisted.Phones))
	for _, p := range persisted.Phones {
		phoneValues = append(phoneValues, p.Value)
	}
	assert.Contains(t, phoneValues, "+15559998888", "the incoming phone must be appended")
	assert.Contains(t, phoneValues, "555-0000", "the existing phone must survive")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "no second contact may be created by a merge")
}

func TestUploadImportRecords_ConfirmAddsNewContact(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96recordsa", Password: "password123!A", Email: "t96recordsa@example.com"}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	body := recordsFixture(cardFixture("Bob", "Jones", "bob@example.com", "+15557778888"))
	uploadReq := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))
	require.Equal(t, "add", uploadResp.Rows[0].SuggestedAction)

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

	var persisted models.Contact
	require.NoError(t, db.Where("email = ?", "bob@example.com").First(&persisted).Error)
	assert.Equal(t, "Bob", persisted.Firstname)
	assert.Equal(t, user.ID, persisted.UserID)
}

func TestUploadImportRecords_WithinBatchDuplicateDefaultsToSkip(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96recordsb", Password: "password123!A", Email: "t96recordsb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	// The same device contact (same email) appears twice in one batch.
	body := recordsFixture(
		cardFixture("Jane", "Smith", "jane@example.com", "+15559998888"),
		cardFixture("Jane", "Smith", "jane@example.com", "+15559998888"),
		cardFixture("Bob", "Jones", "bob@example.com", ""),
	)
	req := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Rows, 3)

	assert.Equal(t, "add", resp.Rows[0].SuggestedAction)
	assert.Nil(t, resp.Rows[0].BatchDuplicateOf)

	require.NotNil(t, resp.Rows[1].BatchDuplicateOf, "the twin must be flagged as a within-batch duplicate")
	assert.Equal(t, 0, *resp.Rows[1].BatchDuplicateOf)
	assert.Equal(t, "skip", resp.Rows[1].SuggestedAction)

	assert.Equal(t, "add", resp.Rows[2].SuggestedAction)
}

func TestUploadImportRecords_ValidationRejectsEmptyBatch(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t96recordse", Password: "password123!A", Email: "t96recordse@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, &config.Config{})

	req := newJSONRequest(t, "/contacts/import/records", models.ImportRecordsRequest{Records: []models.ContactRecordInput{}})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_ERROR", decodeError(t, w).Error.Code)
}

func TestUploadImportRecords_NoAuth_Unauthorized(t *testing.T) {
	db := dbtest.New(t)
	_ = db

	gin.SetMode(gin.ReleaseMode)
	router := routerWithoutAuth(db)
	registerImportRoutes(router, &config.Config{})

	body := recordsFixture(cardFixture("Jane", "Smith", "jane@example.com", ""))
	req := newJSONRequest(t, "/contacts/import/records", json.RawMessage(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "UNAUTHORIZED", decodeError(t, w).Error.Code)
}
