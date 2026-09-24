package controllers

import (
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerOccasionCardListRoute(router *gin.Engine) {
	router.GET("/occasion-obligations/card-list", GetOccasionCardListCSV)
}

func doCardListGET(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGetOccasionCardListCSVIncludesAddressedContact(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{
		UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace",
		Addresses: []models.ContactAddress{{Street: "12 Main St", City: "London"}},
	}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")

	body := w.Body.String()
	assert.Contains(t, body, "Ada Lovelace")
	assert.Contains(t, body, "12 Main St")
}

func TestGetOccasionCardListCSVSkipsContactWithNoAddress(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "NoAddress"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.NotContains(t, body, "NoAddress")
	// Only the header row.
	assert.Equal(t, 1, strings.Count(strings.TrimRight(body, "\n"), "\n")+1)
}

func TestGetOccasionCardListCSVSkipsObligationWithNoMatchingContact(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	// EntityID with no matching Contact row (T17: a contact hard-deleted
	// out from under a soft-referenced obligation).
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: "no-such-vcard-uid", Kind: "card", Label: "Orphan",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	// Only the header row -- the orphaned obligation must be skipped, not
	// crash the export.
	assert.Equal(t, 1, strings.Count(strings.TrimRight(body, "\n"), "\n")+1)
}

func TestGetOccasionCardListCSVPrefersNickname(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{
		UserID: user.ID, Firstname: "Robert", Lastname: "Smith", Nickname: "Bob",
		Addresses: []models.ContactAddress{{Street: "1 Elm St", City: "Springfield"}},
	}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "Bob Smith")
	assert.NotContains(t, body, "Robert Smith")
}

func TestGetOccasionCardListCSVFiltersByKind(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "Gwen", Addresses: []models.ContactAddress{{City: "Paris"}}}
	require.NoError(t, db.Create(&contact).Error)
	giftObligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Gift only",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&giftObligation).Error)

	// Default kind=card must not include a gift-kind obligation.
	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "Gwen")

	w = doCardListGET(router, "/occasion-obligations/card-list?kind=gift")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Gwen")
}

// TestGetOccasionCardListCSVSensitivityFilterDiffersFromFullExport pins the
// ADR 0024 trap this endpoint's own doc comment names: the default here
// (secret excluded) is the OPPOSITE of GET /api/v1/export's default (secret
// included, issue #861's full-fidelity exception) — the regression guard for
// accidentally merging this into that path.
func TestGetOccasionCardListCSVSensitivityFilterDiffersFromFullExport(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "PrivateGuest", Addresses: []models.ContactAddress{{City: "Berlin"}}}
	require.NoError(t, db.Create(&contact).Error)
	secret := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Secret card",
		Active: true, Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&secret).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "PrivateGuest", "a secret obligation must be excluded by default")

	w = doCardListGET(router, "/occasion-obligations/card-list?include_sensitive=true")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "PrivateGuest", "include_sensitive=true must include it")
}

func TestGetOccasionCardListCSVNeutralizesFormulaInjection(t *testing.T) {
	db, router := setupRouter()
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	contact := models.Contact{
		UserID: user.ID, Firstname: "=cmd", Lastname: "|calc",
		Addresses: []models.ContactAddress{{Street: "=SUM(A1:A9)"}},
	}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.NotContains(t, body, "\n=cmd", "a formula-leading cell must be neutralized")
	assert.NotContains(t, body, ",=SUM", "a formula-leading cell must be neutralized")
}

// TestGetOccasionCardListCSV_RealMigratedSchema is the real-DB check
// (CLAUDE.md backend trap #1) for issue #387, ticket #1225: the
// active/kind/sensitivity WHERE clause and Contact.Addresses' JSON
// serializer column must run against the real hand-written migration
// schema, not just AutoMigrate's derived one.
func TestGetOccasionCardListCSV_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "cardlist-realdb", Password: "password123!A", Email: "cardlist-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{
		UserID: user.ID, Firstname: "Rowan",
		Addresses: []models.ContactAddress{{Street: "9 Elm St", City: "Bristol"}},
	}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Holiday card",
		Active: true, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerOccasionCardListRoute(router)

	w := doCardListGET(router, "/occasion-obligations/card-list")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Rowan")
	assert.Contains(t, w.Body.String(), "9 Elm St")
}
