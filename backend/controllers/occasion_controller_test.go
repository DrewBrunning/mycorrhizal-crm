package controllers

import (
	"encoding/json"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerOccasionRoutes(router *gin.Engine) {
	router.GET("/occasions/upcoming", GetUpcomingOccasions)
}

func doOccasionGET(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGetUpcomingOccasionsComposesAllSources(t *testing.T) {
	db, router := setupRouter()
	registerOccasionRoutes(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)

	birthdayContact := models.Contact{UserID: user.ID, Firstname: "Bea", Birthday: soon.Format("2006-01-02")}
	require.NoError(t, db.Create(&birthdayContact).Error)

	lifeEventContact := models.Contact{UserID: user.ID, Firstname: "Leo"}
	require.NoError(t, db.Create(&lifeEventContact).Error)
	month := int(soon.Month())
	day := soon.Day()
	event := models.LifeEvent{
		UserID: user.ID, EntityID: lifeEventContact.VCardUID, Type: "graduated", Remind: true,
		Date: &contactmodel.PartialDate{Month: intPtr(month), Day: intPtr(day)},
	}
	require.NoError(t, db.Create(&event).Error)

	obligationContact := models.Contact{UserID: user.ID, Firstname: "Ozzy"}
	require.NoError(t, db.Create(&obligationContact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: obligationContact.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(month), AnchorDay: intPtr(day), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	// A contact whose birthday is far outside the window must not appear.
	farContact := models.Contact{UserID: user.ID, Firstname: "Faraway", Birthday: now.AddDate(0, 0, 60).Format("2006-01-02")}
	require.NoError(t, db.Create(&farContact).Error)

	w := doOccasionGET(router, "/occasions/upcoming?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Occasions []models.UpcomingOccasion `json:"occasions"`
		Days      int                       `json:"days"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 30, resp.Days)

	sources := map[string]bool{}
	for _, o := range resp.Occasions {
		sources[o.Source] = true
		assert.LessOrEqual(t, o.DaysUntil, 30)
	}
	assert.True(t, sources["birthday"], "birthday within window must appear")
	assert.True(t, sources["life_event"], "life event with Remind within window must appear")
	assert.True(t, sources["obligation"], "active obligation within window must appear")

	for _, o := range resp.Occasions {
		assert.NotEqual(t, "Faraway", o.ContactName, "a contact outside the window must not appear")
	}
}

func TestGetUpcomingOccasionsSortedByDaysUntil(t *testing.T) {
	db, router := setupRouter()
	registerOccasionRoutes(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	near := now.AddDate(0, 0, 3)
	far := now.AddDate(0, 0, 20)

	nearContact := models.Contact{UserID: user.ID, Firstname: "Near", Birthday: near.Format("2006-01-02")}
	require.NoError(t, db.Create(&nearContact).Error)
	farContact := models.Contact{UserID: user.ID, Firstname: "Far", Birthday: far.Format("2006-01-02")}
	require.NoError(t, db.Create(&farContact).Error)

	w := doOccasionGET(router, "/occasions/upcoming?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Occasions []models.UpcomingOccasion `json:"occasions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Occasions, 2)
	assert.Equal(t, "Near", resp.Occasions[0].ContactName)
	assert.Equal(t, "Far", resp.Occasions[1].ContactName)
	assert.LessOrEqual(t, resp.Occasions[0].DaysUntil, resp.Occasions[1].DaysUntil)
}

func TestGetUpcomingOccasionsSensitivityFilter(t *testing.T) {
	db, router := setupRouter()
	registerOccasionRoutes(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Secretive"}
	require.NoError(t, db.Create(&contact).Error)

	secret := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Secret gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&secret).Error)

	w := doOccasionGET(router, "/occasions/upcoming?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Occasions []models.UpcomingOccasion `json:"occasions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Occasions, "a secret obligation must be excluded by default")

	w = doOccasionGET(router, "/occasions/upcoming?days=30&include_sensitive=true")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Occasions, 1, "include_sensitive=true must include the secret obligation")
}

func TestGetUpcomingOccasionsRejectsInvalidDays(t *testing.T) {
	_, router := setupRouter()
	registerOccasionRoutes(router)

	w := doOccasionGET(router, "/occasions/upcoming?days=45")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetUpcomingOccasionsEmptyResultIsArrayNotNull pins CLAUDE.md frontend
// trap #8: a nil slice with the wrong JSON handling silently disappears
// instead of serializing as `[]`, which crashed the prep view for exactly
// this class of bug before.
func TestGetUpcomingOccasionsEmptyResultIsArrayNotNull(t *testing.T) {
	_, router := setupRouter()
	registerOccasionRoutes(router)

	w := doOccasionGET(router, "/occasions/upcoming?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	occasionsRaw, present := raw["occasions"]
	require.True(t, present, "the occasions key must be present even when empty")
	assert.Equal(t, "[]", string(occasionsRaw), "an empty result must serialize as [], not be absent or null")
}
