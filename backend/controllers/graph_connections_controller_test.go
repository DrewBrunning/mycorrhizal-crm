package controllers

import (
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetGraphConnections exercises the T10 endpoint end-to-end: two-hop
// chain, ownership/validation errors.
func TestGetGraphConnections(t *testing.T) {
	db, router := setupRouter()
	router.GET("/graph/connections", GetGraphConnections)

	var user models.User
	db.First(&user)

	john := models.Contact{UserID: user.ID, Firstname: "John"}
	sister := models.Contact{UserID: user.ID, Firstname: "Sister"}
	husband := models.Contact{UserID: user.ID, Firstname: "Husband"}
	require.NoError(t, db.Create(&john).Error)
	require.NoError(t, db.Create(&sister).Error)
	require.NoError(t, db.Create(&husband).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: sister.VCardUID, TargetID: john.VCardUID, Type: "sibling_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: husband.VCardUID, TargetID: sister.VCardUID, Type: "spouse_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	// Depth 2: both sister and husband reachable.
	w := doGet(t, router, "/graph/connections?from="+john.VCardUID+"&depth=2")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, john.VCardUID, resp.FromVCardUID)
	require.Len(t, resp.Chains, 2)

	var husbandChain *models.GraphChain
	for i := range resp.Chains {
		if resp.Chains[i].TargetVCardUID == husband.VCardUID {
			husbandChain = &resp.Chains[i]
		}
	}
	require.NotNil(t, husbandChain)
	assert.Equal(t, 2, husbandChain.Depth)
	require.Len(t, husbandChain.Steps, 2)
	assert.Equal(t, "sibling_of", husbandChain.Steps[0].Relation)
	assert.Equal(t, "spouse_of", husbandChain.Steps[1].Relation)
}

func TestGetGraphConnections_ValidationErrors(t *testing.T) {
	db, router := setupRouter()
	router.GET("/graph/connections", GetGraphConnections)

	// Missing from.
	w := doGet(t, router, "/graph/connections")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Non-integer depth.
	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	w2 := doGet(t, router, "/graph/connections?from="+contact.VCardUID+"&depth=abc")
	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Unknown/foreign contact.
	w3 := doGet(t, router, "/graph/connections?from=00000000-0000-0000-0000-000000000000&depth=2")
	assert.Equal(t, http.StatusNotFound, w3.Code)

	// Depth beyond the cap is rejected (not run).
	var user2 models.User
	db.First(&user2)
	c2 := models.Contact{UserID: user2.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&c2).Error)
	w4 := doGet(t, router, "/graph/connections?from="+c2.VCardUID+"&depth=6")
	assert.Equal(t, http.StatusBadRequest, w4.Code, "depth above the max must be rejected")
}

func TestGetGraphConnections_SynonymRelationFilter(t *testing.T) {
	db, router := setupRouter()
	router.GET("/graph/connections", GetGraphConnections)

	var user models.User
	db.First(&user)

	john := models.Contact{UserID: user.ID, Firstname: "John"}
	brother := models.Contact{UserID: user.ID, Firstname: "Brother"}
	friend := models.Contact{UserID: user.ID, Firstname: "Friend"}
	require.NoError(t, db.Create(&john).Error)
	require.NoError(t, db.Create(&brother).Error)
	require.NoError(t, db.Create(&friend).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: brother.VCardUID, TargetID: john.VCardUID, Type: "sibling_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: friend.VCardUID, TargetID: john.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	// "brother" is a registry synonym for sibling_of — only the brother chain returns.
	w := doGet(t, router, "/graph/connections?from="+john.VCardUID+"&relation=brother")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Chains, 1)
	assert.Equal(t, brother.VCardUID, resp.Chains[0].TargetVCardUID)
}

func doGet(t *testing.T, router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
