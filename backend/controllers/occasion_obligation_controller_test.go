package controllers

import (
	"bytes"
	"encoding/json"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func registerOccasionObligationRoutes(t *testing.T, router *gin.Engine) {
	router.POST("/occasion-obligations", withValidated(func() any { return &models.OccasionObligationInput{} }), CreateOccasionObligation)
	router.GET("/occasion-obligations", ListOccasionObligations)
	router.GET("/occasion-obligations/:id", GetOccasionObligation)
	router.PUT("/occasion-obligations/:id", withValidated(func() any { return &models.OccasionObligationInput{} }), UpdateOccasionObligation)
	router.DELETE("/occasion-obligations/:id", DeleteOccasionObligation)
}

func seedOccasionObligationContact(t *testing.T, db *gorm.DB, userID uint) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func doOccasionJSON(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCreateOccasionObligation(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	payload := models.OccasionObligationInput{
		EntityID:     contact.VCardUID,
		Kind:         models.OccasionObligationKindCard,
		Label:        "Christmas card",
		AnchorMonth:  intPtr(12),
		AnchorDay:    intPtr(25),
		LeadTimeDays: 14,
		Notes:        "Send to their new address",
	}
	w := doOccasionJSON(router, "POST", "/occasion-obligations", payload)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 1, count)

	var obligation models.OccasionObligation
	db.First(&obligation)
	assert.Equal(t, models.RelationshipSensitivityNormal, obligation.Sensitivity, "omitted sensitivity must default to normal")
	assert.True(t, obligation.Active, "omitted active must default to true")
	assert.Equal(t, "Christmas card", obligation.Label)
	assert.Equal(t, 12, *obligation.AnchorMonth)
	assert.Equal(t, 25, *obligation.AnchorDay)
	assert.Equal(t, "Send to their new address", obligation.Notes, "notes must persist on create")
}

func TestCreateOccasionObligationRejectsContactFromAnotherUser(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := seedOccasionObligationContact(t, db, otherUser.ID)

	payload := models.OccasionObligationInput{
		EntityID: othersContact.VCardUID,
		Kind:     models.OccasionObligationKindCard,
		Label:    "Christmas card",
	}
	w := doOccasionJSON(router, "POST", "/occasion-obligations", payload)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count)
}

func TestCreateOccasionObligationRejectsPartialAnchor(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	// Month with no day.
	w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Day with no month.
	w = doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:  contact.VCardUID,
		Kind:      models.OccasionObligationKindCard,
		Label:     "Christmas card",
		AnchorDay: intPtr(25),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count, "neither partial-anchor request should have persisted a row")
}

func TestCreateOccasionObligationRejectsMissingAnchor(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	// Neither month nor day is a 400 with its own reason code (maintainer
	// decision on #2670): a dateless obligation never reminds, so it is refused
	// rather than saved silently inert. Active on or off makes no difference.
	inactive := false
	for name, active := range map[string]*bool{"active default": nil, "inactive": &inactive} {
		w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
			EntityID: contact.VCardUID,
			Kind:     models.OccasionObligationKindGift,
			Label:    "Birthday gift",
			Active:   active,
		})
		require.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
		assert.Contains(t, w.Body.String(), `"reason_code":"occasion_anchor_date_required"`, name)
	}

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count, "a rejected request must not persist a row")
}

func TestOccasionObligationRejectsImpossibleMonthDay(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	cases := []struct {
		month, day int
		ok         bool
	}{
		{2, 29, true}, // leap-day anchor is a real annual date
		{2, 30, false},
		{2, 31, false},
		{4, 30, true},
		{4, 31, false},
		{6, 31, false},
		{9, 31, false},
		{11, 31, false},
		{1, 31, true},
		{12, 31, true},
	}
	for _, tc := range cases {
		w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
			EntityID:    contact.VCardUID,
			Kind:        models.OccasionObligationKindCard,
			Label:       "Card",
			AnchorMonth: intPtr(tc.month),
			AnchorDay:   intPtr(tc.day),
		})
		if tc.ok {
			assert.Equal(t, http.StatusCreated, w.Code, "%d/%d: %s", tc.month, tc.day, w.Body.String())
			continue
		}
		assert.Equal(t, http.StatusBadRequest, w.Code, "%d/%d: %s", tc.month, tc.day, w.Body.String())
		assert.Contains(t, w.Body.String(), `"reason_code":"occasion_anchor_date_invalid"`, "%d/%d", tc.month, tc.day)
	}

	// The same rule applies on update (full replace).
	create := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindCard, Label: "Xmas",
		AnchorMonth: intPtr(12), AnchorDay: intPtr(25),
	})
	require.Equal(t, http.StatusCreated, create.Code, create.Body.String())
	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &created))
	w := doOccasionJSON(router, "PUT", "/occasion-obligations/"+created.OccasionObligation.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindCard, Label: "Xmas",
		AnchorMonth: intPtr(2), AnchorDay: intPtr(31),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason_code":"occasion_anchor_date_invalid"`)
}

// A row saved before the date became mandatory (ADR 0024 originally allowed
// "neither") stays readable, and can still be retired or given a date; it just
// cannot be re-saved active and dateless.
func TestUpdateOccasionObligationLegacyDatelessRow(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	legacy := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: models.OccasionObligationKindInvite,
		Label: "Annual summer BBQ", Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&legacy).Error)

	get := doOccasionJSON(router, "GET", "/occasion-obligations/"+legacy.ID, nil)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())

	// Re-saving it active and still dateless is refused.
	w := doOccasionJSON(router, "PUT", "/occasion-obligations/"+legacy.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindInvite, Label: "Annual summer BBQ (edited)",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason_code":"occasion_anchor_date_required"`)

	// Retiring it (inactive, still dateless) is allowed.
	inactive := false
	w = doOccasionJSON(router, "PUT", "/occasion-obligations/"+legacy.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindInvite, Label: "Annual summer BBQ", Active: &inactive,
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Giving it a date is allowed and materializes the reminder.
	w = doOccasionJSON(router, "PUT", "/occasion-obligations/"+legacy.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindInvite, Label: "Annual summer BBQ",
		AnchorMonth: intPtr(7), AnchorDay: intPtr(4),
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var reminders int64
	db.Model(&models.Reminder{}).Where("occasion_obligation_id = ?", legacy.ID).Count(&reminders)
	assert.EqualValues(t, 1, reminders)

	// A row that HAS a date cannot be saved dateless, even inactive.
	w = doOccasionJSON(router, "PUT", "/occasion-obligations/"+legacy.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: models.OccasionObligationKindInvite, Label: "Annual summer BBQ", Active: &inactive,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateOccasionObligationRejectsLinkedLifeEventFromAnotherUser(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	otherUser := models.User{Username: "other2", Password: "x", Email: "other2@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersEvent := models.LifeEvent{UserID: otherUser.ID, EntityID: contact.VCardUID, Type: "graduation"}
	require.NoError(t, db.Create(&othersEvent).Error)

	w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:          contact.VCardUID,
		Kind:              models.OccasionObligationKindGift,
		Label:             "Grad gift",
		AnchorMonth:       intPtr(6),
		AnchorDay:         intPtr(1),
		LinkedLifeEventID: othersEvent.ID,
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count)
}

func TestUpdateOccasionObligationNotFound(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	existing := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Untouched"}
	require.NoError(t, db.Create(&existing).Error)

	w := doOccasionJSON(router, "PUT", "/occasion-obligations/no-such-id", models.OccasionObligationInput{
		EntityID: contact.VCardUID, Kind: "card", Label: "x",
	})
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")

	var count int64
	require.NoError(t, db.Model(&models.OccasionObligation{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a not-found update must not create a row")
	var reloaded models.OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", existing.ID).Error)
	assert.Equal(t, "Untouched", reloaded.Label)
}

func TestUpdateOccasionObligationRejectsContactFromAnotherUser(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	obligation := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Original"}
	require.NoError(t, db.Create(&obligation).Error)

	otherUser := models.User{Username: "other3", Password: "x", Email: "other3@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := seedOccasionObligationContact(t, db, otherUser.ID)

	w := doOccasionJSON(router, "PUT", "/occasion-obligations/"+obligation.ID, models.OccasionObligationInput{
		EntityID: othersContact.VCardUID, Kind: "card", Label: "Hijacked",
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var reloaded models.OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", obligation.ID).Error)
	assert.Equal(t, "Original", reloaded.Label, "a rejected update must not have persisted")
}

func TestDeleteOccasionObligationNotFound(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	existing := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Keep me"}
	require.NoError(t, db.Create(&existing).Error)

	w := doOccasionJSON(router, "DELETE", "/occasion-obligations/no-such-id", nil)
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")

	var count int64
	require.NoError(t, db.Model(&models.OccasionObligation{}).Where("id = ?", existing.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a not-found delete must not remove other rows")
}

func TestListOccasionObligationsFiltersByEntity(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contactA := seedOccasionObligationContact(t, db, user.ID)
	contactB := seedOccasionObligationContact(t, db, user.ID)

	require.NoError(t, db.Create(&models.OccasionObligation{UserID: user.ID, EntityID: contactA.VCardUID, Kind: "card", Label: "A card"}).Error)
	require.NoError(t, db.Create(&models.OccasionObligation{UserID: user.ID, EntityID: contactB.VCardUID, Kind: "card", Label: "B card"}).Error)

	w := doOccasionJSON(router, "GET", "/occasion-obligations?entity_id="+contactA.VCardUID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		OccasionObligations []models.OccasionObligation `json:"occasion_obligations"`
		Total               int64                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.OccasionObligations, 1)
	assert.Equal(t, "A card", resp.OccasionObligations[0].Label)
	assert.EqualValues(t, 1, resp.Total)
}

func TestListOccasionObligationsBrowseModePaginatesWithNextCursor(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.OccasionObligation{
			UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Ob",
		}).Error)
	}

	w := doOccasionJSON(router, "GET", "/occasion-obligations?limit=2", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		OccasionObligations []models.OccasionObligation `json:"occasion_obligations"`
		NextCursor          string                      `json:"next_cursor"`
		Total               int64                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.OccasionObligations, 2, "must truncate to the requested limit")
	assert.NotEmpty(t, resp.NextCursor, "a truncated page must return a next_cursor")
	assert.EqualValues(t, 3, resp.Total)
}

func TestUpdateOccasionObligationDeactivate(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	obligation := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card", Active: true}
	require.NoError(t, db.Create(&obligation).Error)

	inactive := false
	w := doOccasionJSON(router, "PUT", "/occasion-obligations/"+obligation.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     "card",
		Label:    "Christmas card",
		Active:   &inactive,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", obligation.ID).Error)
	assert.False(t, reloaded.Active, "explicit active=false must persist, not default back to true")
}

func TestDeleteOccasionObligationSoftDeletesAndAllowsRecreate(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	obligation := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card"}
	require.NoError(t, db.Create(&obligation).Error)

	w := doOccasionJSON(router, "DELETE", "/occasion-obligations/"+obligation.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count, "soft-deleted row must not appear in the default-scoped count")

	var unscopedCount int64
	db.Unscoped().Model(&models.OccasionObligation{}).Count(&unscopedCount)
	assert.EqualValues(t, 1, unscopedCount, "soft-deleted row must still exist (tombstone)")

	// No natural-key unique constraint: recreating for the same contact/kind
	// must succeed even though the old row still exists, soft-deleted.
	w = doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        "card",
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
		AnchorDay:   intPtr(25),
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestGetOccasionObligationRejectsOtherUser(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other2", Password: "x", Email: "other2@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := seedOccasionObligationContact(t, db, otherUser.ID)

	obligation := models.OccasionObligation{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Kind: "card", Label: "Not yours"}
	require.NoError(t, db.Create(&obligation).Error)

	w := doOccasionJSON(router, "GET", "/occasion-obligations/"+obligation.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateOccasionObligationMaterializesReminder(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)
	require.NoError(t, db.Model(&contact).Update("lastname", "Smith").Error)

	w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:     contact.VCardUID,
		Kind:         models.OccasionObligationKindGift,
		Label:        "Birthday gift",
		AnchorMonth:  intPtr(12),
		AnchorDay:    intPtr(25),
		LeadTimeDays: 14,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	var reminders []models.Reminder
	require.NoError(t, db.Where("occasion_obligation_id = ?", created.OccasionObligation.ID).Find(&reminders).Error)
	require.Len(t, reminders, 1, "an active obligation with a valid anchor must materialize exactly one reminder")
	assert.Equal(t, "yearly", reminders[0].Recurrence, "recurrence must be yearly so the reminder engine self-regenerates without a separate job")
	// Issue #2915: the stored message must not bake the English "Occasion — "
	// prefix into the row; the web renders a localized title around "label: name".
	assert.Equal(t, "Birthday gift: Alice Smith", reminders[0].Message)

	loc := time.UTC
	wantAnchor := nextRemindAt(12, 25, time.Now().In(loc), loc)
	wantRemindAt := wantAnchor.AddDate(0, 0, -14)
	assert.WithinDuration(t, wantRemindAt, reminders[0].RemindAt, time.Second, "reminder must fire lead_time_days before the anchor date")
}

func TestUpdateOccasionObligationDeactivateRemovesReminder(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	createResp := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
		AnchorDay:   intPtr(25),
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	var beforeCount int64
	db.Model(&models.Reminder{}).Where("occasion_obligation_id = ?", created.OccasionObligation.ID).Count(&beforeCount)
	require.EqualValues(t, 1, beforeCount)

	inactive := false
	updateResp := doOccasionJSON(router, "PUT", "/occasion-obligations/"+created.OccasionObligation.ID, models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
		AnchorDay:   intPtr(25),
		Active:      &inactive,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())

	var afterCount int64
	db.Model(&models.Reminder{}).Where("occasion_obligation_id = ?", created.OccasionObligation.ID).Count(&afterCount)
	assert.EqualValues(t, 0, afterCount, "deactivating an obligation must remove its materialized reminder")
}

func TestUpdateOccasionObligationEditAnchorRegeneratesReminderWithoutDuplicate(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	createResp := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
		AnchorDay:   intPtr(25),
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	updateResp := doOccasionJSON(router, "PUT", "/occasion-obligations/"+created.OccasionObligation.ID, models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(11),
		AnchorDay:   intPtr(1),
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())

	var reminders []models.Reminder
	require.NoError(t, db.Where("occasion_obligation_id = ?", created.OccasionObligation.ID).Find(&reminders).Error)
	require.Len(t, reminders, 1, "editing the anchor must regenerate, not duplicate, the reminder")

	loc := time.UTC
	want := nextRemindAt(11, 1, time.Now().In(loc), loc)
	assert.WithinDuration(t, want, reminders[0].RemindAt, time.Second)
}

func TestDeleteOccasionObligationRemovesReminder(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	createResp := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
		AnchorDay:   intPtr(25),
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	deleteResp := doOccasionJSON(router, "DELETE", "/occasion-obligations/"+created.OccasionObligation.ID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())

	var count int64
	db.Unscoped().Model(&models.Reminder{}).Where("occasion_obligation_id = ?", created.OccasionObligation.ID).Count(&count)
	assert.EqualValues(t, 0, count, "deleting an obligation must hard-delete its machine-synthesized reminder")
}
