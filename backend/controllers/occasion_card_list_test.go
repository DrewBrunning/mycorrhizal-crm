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
	db, router := setupRouter(t)
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
	db, router := setupRouter(t)
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
	db, router := setupRouter(t)
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
	db, router := setupRouter(t)
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
	db, router := setupRouter(t)
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
	db, router := setupRouter(t)
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

// TestSafeDownloadStem pins the filename sanitizer added for the schemathesis
// finding (issue #369): `kind` is an open classifier, so arbitrary bytes —
// including the NUL schemathesis generated — must never reach the
// Content-Disposition header.
func TestSafeDownloadStem(t *testing.T) {
	cases := map[string]string{
		"card":                   "card",
		"gift":                   "gift",
		"a-b_c.d":                "a-b_c.d",
		"":                       "occasions",
		"\x00":                   "occasions",
		"\n\r\t":                 "occasions",
		"\xf0\xbe\x85\xbe7":      "7",
		"c\x00a\x0ar\x01d":       "card",
		"../../etc/passwd":       "....etcpasswd",
		strings.Repeat("a", 300): strings.Repeat("a", 100),
	}
	for in, want := range cases {
		assert.Equal(t, want, safeDownloadStem(in), "input %q", in)
	}
}

// TestGetOccasionCardListCSVSanitizesKindInFilename is the regression test for
// the real schemathesis failure: a `kind` value carrying a NUL byte made Go
// emit an invalid Content-Disposition header, which the all-in-one image's
// nginx rejected with a 502. The response must stay a well-formed HTTP
// response — no control bytes in the filename header — while the raw value
// still drives the DB filter.
func TestGetOccasionCardListCSVSanitizesKindInFilename(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	// The exact class of value schemathesis generated: a NUL plus a newline
	// around otherwise-safe text.
	w := doCardListGET(router, "/occasion-obligations/card-list?kind=%00card%0A")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	hdr := w.Header().Get("Content-Disposition")
	require.NotEmpty(t, hdr)
	for i := 0; i < len(hdr); i++ {
		b := hdr[i]
		assert.True(t, b == '\t' || b >= 0x20 && b != 0x7f,
			"Content-Disposition has control byte 0x%02x at offset %d: %q", b, i, hdr)
	}
	assert.Contains(t, hdr, "filename=mycorrhizal-card-list-")
	assert.True(t, strings.HasSuffix(hdr, ".csv"), hdr)
}

// TestGetOccasionCardListCSVBlacklistedKindFallsBackToDefaultStem covers the
// other half: a kind with no safe characters still yields a usable filename
// rather than an empty one.
func TestGetOccasionCardListCSVBlacklistedKindFallsBackToDefaultStem(t *testing.T) {
	db, router := setupRouter(t)
	registerOccasionCardListRoute(router)

	var user models.User
	db.First(&user)

	w := doCardListGET(router, "/occasion-obligations/card-list?kind=%00")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), "filename=mycorrhizal-occasions-list-")
}

func TestGetOccasionCardListCSVNeutralizesFormulaInjection(t *testing.T) {
	db, router := setupRouter(t)
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
