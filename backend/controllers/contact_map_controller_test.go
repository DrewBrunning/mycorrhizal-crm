package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mycorrhizal/models"
)

func mapRouter(t *testing.T) (*gorm.DB, *gin.Engine, uint) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.GET("/contacts/map", GetContactMap)
	return db, router, user.ID
}

func getMap(router *gin.Engine) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodGet, "/contacts/map", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// seedMapContact stores a contact whose flat addresses are written directly
// (id/coordinates/sensitivity explicit) so each case controls exactly what the
// map sees.
func seedMapContact(t *testing.T, db *gorm.DB, userID uint, first string, addrs []models.ContactAddress) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: first, Lastname: "Mapper", Addresses: addrs}
	require.NoError(t, db.Create(&c).Error)
	return c
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) ContactMapResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp ContactMapResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestGetContactMap_ReturnsValidPointsOnly_NotSensitivityGated(t *testing.T) {
	db, router, uid := mapRouter(t)
	ada := seedMapContact(t, db, uid, "Ada", []models.ContactAddress{
		{ID: "a-ok", Street: "10 Downing St", City: "London", Coordinates: "geo:51.5034,-0.1276"},
		{ID: "a-none", Street: "No coords"},
		{ID: "a-bad", Street: "Bad", Coordinates: "geo:not,numbers"},
		{ID: "a-range", Street: "Range", Coordinates: "geo:91,0"},
		{ID: "a-secret", Street: "Hidden", Coordinates: "geo:1,2", Sensitivity: models.RelationshipSensitivitySecret},
		{ID: "a-private", Street: "Priv", Coordinates: "geo:3,4", Sensitivity: models.RelationshipSensitivityPrivate},
	})

	resp := decodeMap(t, getMap(router))
	assert.False(t, resp.Truncated)
	require.Len(t, resp.Points, 3, "valid geo points only; malformed/out-of-range skipped, secret+private included")
	assert.Equal(t, ContactMapPoint{
		ContactID: ada.ID, ContactUID: ada.VCardUID, ContactName: "Ada Mapper",
		AddressID: "a-ok", Label: "10 Downing St, London", Coordinates: "geo:51.5034,-0.1276",
	}, resp.Points[0])
	assert.Equal(t, "a-secret", resp.Points[1].AddressID)
	assert.Equal(t, "a-private", resp.Points[2].AddressID)
}

func TestGetContactMap_ScopesToCallerAndExcludesArchivedAndDeleted(t *testing.T) {
	db, router, uid := mapRouter(t)
	other := models.User{Username: "mapother", Password: "password123!A", Email: "mapother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	addr := []models.ContactAddress{{ID: "x", Street: "S", Coordinates: "geo:1,1"}}
	seedMapContact(t, db, other.ID, "Theirs", addr)
	archived := seedMapContact(t, db, uid, "Archived", []models.ContactAddress{{ID: "arch", Coordinates: "geo:1,1"}})
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", archived.ID).Update("archived", true).Error)
	deleted := seedMapContact(t, db, uid, "Deleted", []models.ContactAddress{{ID: "del", Coordinates: "geo:1,1"}})
	require.NoError(t, db.Delete(&deleted).Error)
	mine := seedMapContact(t, db, uid, "Mine", []models.ContactAddress{{ID: "mine", Coordinates: "geo:2,2"}})

	resp := decodeMap(t, getMap(router))
	require.Len(t, resp.Points, 1)
	assert.Equal(t, mine.ID, resp.Points[0].ContactID)
}

func TestGetContactMap_EmptyIsEmptyArrayInRawJSON(t *testing.T) {
	_, router, _ := mapRouter(t)
	w := getMap(router)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["points"]), "an empty map is points: [], not absent or null")
	assert.JSONEq(t, `false`, string(raw["truncated"]))
}

func TestGetContactMap_BoundedAtCeiling(t *testing.T) {
	db, router, uid := mapRouter(t)
	// One contact with MaxContactMapPoints+1 geocoded addresses crosses the
	// ceiling exactly; a second contact proves the early exit stops the scan.
	addrs := make([]models.ContactAddress, MaxContactMapPoints+1)
	for i := range addrs {
		addrs[i] = models.ContactAddress{ID: fmt.Sprintf("a%d", i), Coordinates: "geo:1,1"}
	}
	seedMapContact(t, db, uid, "Big", addrs)
	seedMapContact(t, db, uid, "After", []models.ContactAddress{{ID: "z", Coordinates: "geo:2,2"}})

	resp := decodeMap(t, getMap(router))
	assert.True(t, resp.Truncated)
	assert.Len(t, resp.Points, MaxContactMapPoints)

	// Exactly at the ceiling is not truncated.
	require.NoError(t, db.Exec("DELETE FROM contacts").Error)
	seedMapContact(t, db, uid, "Exact", addrs[:MaxContactMapPoints])
	resp = decodeMap(t, getMap(router))
	assert.False(t, resp.Truncated)
	assert.Len(t, resp.Points, MaxContactMapPoints)
}
