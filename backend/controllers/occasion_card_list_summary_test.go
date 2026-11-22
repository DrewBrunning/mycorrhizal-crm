package controllers

import (
	"encoding/csv"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const cardListSummaryPath = "/occasion-obligations/card-list/summary"

func registerCardListSummaryRoutes(router *gin.Engine) {
	registerOccasionCardListRoute(router)
	router.GET(cardListSummaryPath, GetOccasionCardListSummary)
}

type cardListSummaryBody struct {
	Total          int `json:"total"`
	Included       int `json:"included"`
	Skipped        int `json:"skipped"`
	SkippedReasons struct {
		NoAddress          int `json:"no_address"`
		PrivateAddressOnly int `json:"private_address_only"`
	} `json:"skipped_reasons"`
}

func getCardListSummary(t *testing.T, router *gin.Engine, query string) cardListSummaryBody {
	t.Helper()
	w := doCardListGET(router, cardListSummaryPath+query)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body cardListSummaryBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func seedSummaryRecipient(t *testing.T, db *gorm.DB, userID uint, first string, addrs []models.ContactAddress, kind, obSens string) {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: first, Addresses: addrs}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.OccasionObligation{
		UserID: userID, EntityID: contact.VCardUID, Kind: kind, Label: first + " card",
		Active: true, Sensitivity: obSens,
	}).Error)
}

func TestGetOccasionCardListSummaryCountsIncludedAndSkippedReasons(t *testing.T) {
	db, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	var user models.User
	db.First(&user)
	normal := models.RelationshipSensitivityNormal

	seedSummaryRecipient(t, db, user.ID, "Addressed", []models.ContactAddress{{Street: "1 Main St", City: "London"}}, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "NoAddr", nil, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "OnlySecret", []models.ContactAddress{{Street: "2 Hidden St", Sensitivity: models.RelationshipSensitivitySecret}}, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "OnlyPrivate", []models.ContactAddress{{Street: "3 Hidden St", Sensitivity: models.RelationshipSensitivityPrivate}}, "card", normal)
	// Orphan obligation: the referenced contact no longer exists.
	require.NoError(t, db.Create(&models.OccasionObligation{
		UserID: user.ID, EntityID: "no-such-vcard-uid", Kind: "card", Label: "Orphan", Active: true, Sensitivity: normal,
	}).Error)
	// Different kind, inactive and above-normal obligations are never counted.
	seedSummaryRecipient(t, db, user.ID, "GiftKind", nil, "gift", normal)
	seedSummaryRecipient(t, db, user.ID, "SecretObligation", []models.ContactAddress{{Street: "4 Main St"}}, "card", models.RelationshipSensitivitySecret)

	s := getCardListSummary(t, router, "")
	assert.Equal(t, 5, s.Total)
	assert.Equal(t, 1, s.Included)
	assert.Equal(t, 4, s.Skipped)
	assert.Equal(t, 2, s.SkippedReasons.NoAddress, "no address + orphan contact")
	assert.Equal(t, 2, s.SkippedReasons.PrivateAddressOnly)

	// Opt-in: private/secret addresses and the secret obligation now count.
	s = getCardListSummary(t, router, "?include_sensitive=true")
	assert.Equal(t, 6, s.Total)
	assert.Equal(t, 4, s.Included)
	assert.Equal(t, 2, s.Skipped)
	assert.Equal(t, 2, s.SkippedReasons.NoAddress)
	assert.Equal(t, 0, s.SkippedReasons.PrivateAddressOnly)

	s = getCardListSummary(t, router, "?kind=gift")
	assert.Equal(t, 1, s.Total)
	assert.Equal(t, 1, s.Skipped)
	assert.Equal(t, 1, s.SkippedReasons.NoAddress)
}

func TestGetOccasionCardListSummaryEmptyIsZeroedObject(t *testing.T) {
	_, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	w := doCardListGET(router, cardListSummaryPath)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"total":0,"included":0,"skipped":0,"skipped_reasons":{"no_address":0,"private_address_only":0}}`, w.Body.String())
}

func TestGetOccasionCardListSummaryNeverDisclosesNamesOrAddresses(t *testing.T) {
	db, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	var user models.User
	db.First(&user)
	seedSummaryRecipient(t, db, user.ID, "Zebediah", []models.ContactAddress{{Street: "77 Secret Lane"}}, "card", models.RelationshipSensitivityNormal)
	w := doCardListGET(router, cardListSummaryPath)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "Zebediah")
	assert.NotContains(t, w.Body.String(), "Secret Lane")
	assert.Equal(t, `{"included":1,"skipped":0,"skipped_reasons":{"no_address":0,"private_address_only":0},"total":1}`, normalizeJSON(t, w.Body.Bytes()))
}

func normalizeJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v map[string]any
	require.NoError(t, json.Unmarshal(raw, &v))
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

func TestGetOccasionCardListSummaryNeverCountsAnotherUsersData(t *testing.T) {
	db, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	var user models.User
	db.First(&user)
	other := models.User{Username: "intruder", Password: "password123", Email: "intruder@example.com"}
	require.NoError(t, db.Create(&other).Error)

	seedSummaryRecipient(t, db, other.ID, "TheirAddressed", []models.ContactAddress{{Street: "9 Their St"}}, "card", models.RelationshipSensitivityNormal)
	seedSummaryRecipient(t, db, other.ID, "TheirNoAddr", nil, "card", models.RelationshipSensitivityNormal)
	// An obligation of the caller that points at the other user's contact
	// must not resolve it: the contact lookup is owner-scoped.
	theirs := models.Contact{UserID: other.ID, Firstname: "Cross", Addresses: []models.ContactAddress{{Street: "5 Cross St"}}}
	require.NoError(t, db.Create(&theirs).Error)
	require.NoError(t, db.Create(&models.OccasionObligation{
		UserID: user.ID, EntityID: theirs.VCardUID, Kind: "card", Label: "Cross", Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	s := getCardListSummary(t, router, "?include_sensitive=true")
	assert.Equal(t, 1, s.Total, "only the caller's own obligation counts")
	assert.Equal(t, 0, s.Included, "a foreign contact's address is not resolved")
	assert.Equal(t, 1, s.SkippedReasons.NoAddress)
}

func TestGetOccasionCardListSummaryRejectsControlCharsInKind(t *testing.T) {
	_, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	w := doCardListGET(router, cardListSummaryPath+"?kind=a%00b")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "occasion_kind_invalid")
}

// The summary must never disagree with the CSV: included equals the CSV data
// row count for the same query, for both sensitivity modes.
func TestGetOccasionCardListSummaryMatchesCSVRowCount(t *testing.T) {
	db, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	var user models.User
	db.First(&user)
	normal := models.RelationshipSensitivityNormal
	seedSummaryRecipient(t, db, user.ID, "A", []models.ContactAddress{{Street: "1 St"}}, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "B", []models.ContactAddress{{Street: "2 St", Sensitivity: models.RelationshipSensitivitySecret}, {Street: "3 St"}}, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "C", []models.ContactAddress{{Street: "4 St", Sensitivity: models.RelationshipSensitivityPrivate}}, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "D", nil, "card", normal)
	seedSummaryRecipient(t, db, user.ID, "E", []models.ContactAddress{{Sensitivity: ""}}, "card", normal)

	for _, q := range []string{"", "?include_sensitive=true"} {
		w := doCardListGET(router, "/occasion-obligations/card-list"+q)
		require.Equal(t, http.StatusOK, w.Code)
		rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
		require.NoError(t, err)
		s := getCardListSummary(t, router, q)
		assert.Equal(t, len(rows)-1, s.Included, "query %q", q)
		assert.Equal(t, s.Total, s.Included+s.Skipped)
		assert.Equal(t, s.Skipped, s.SkippedReasons.NoAddress+s.SkippedReasons.PrivateAddressOnly)
	}
}

// TestGetOccasionCardListSummary_QueryCountIsBounded pins "no N+1": the query
// count is one obligation query plus one contact lookup per 500-uid batch,
// never one per obligation.
func TestGetOccasionCardListSummary_QueryCountIsBounded(t *testing.T) {
	db, router, counter, userID := setupCountingRouter(t)
	router.GET(cardListSummaryPath, GetOccasionCardListSummary)

	const n = 1100 // three contact batches
	contacts := make([]models.Contact, n)
	for i := range contacts {
		contacts[i] = models.Contact{UserID: userID, Firstname: "C", Addresses: []models.ContactAddress{{Street: "1 St"}}}
	}
	require.NoError(t, db.CreateInBatches(&contacts, 50).Error)
	obligations := make([]models.OccasionObligation, n)
	for i := range obligations {
		obligations[i] = models.OccasionObligation{
			UserID: userID, EntityID: contacts[i].VCardUID, Kind: "card", Label: "L",
			Active: true, Sensitivity: models.RelationshipSensitivityNormal,
		}
	}
	require.NoError(t, db.CreateInBatches(&obligations, 50).Error)

	counter.Reset()
	w := doCardListGET(router, cardListSummaryPath)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body cardListSummaryBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, n, body.Total)
	assert.Equal(t, n, body.Included)
	assert.EqualValues(t, 4, counter.Count(), "1 obligation query + 3 contact batches, independent of row count")
}

func TestGetOccasionCardListSummaryReportsDatabaseFailure(t *testing.T) {
	db, router := setupRouter(t)
	registerCardListSummaryRoutes(router)
	logtest.AllowWarnings(t, "the path under test legitimately logs a database error")
	dbtest.HideTable(t, db, "occasion_obligations")

	w := doCardListGET(router, cardListSummaryPath)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var env struct {
		Error struct {
			Code       string `json:"code"`
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	assert.Equal(t, "DATABASE_ERROR", env.Error.Code)
	assert.Equal(t, "common_database_error", env.Error.ReasonCode,
		"a database failure must carry the shared reason, not a bare 500")
}
