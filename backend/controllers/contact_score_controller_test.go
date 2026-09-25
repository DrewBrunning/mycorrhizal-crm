package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContactScore_ReturnsFullBreakdown(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp models.ContactScoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, contact.ID, resp.ContactID)
	assert.GreaterOrEqual(t, resp.Score, 0)
	assert.LessOrEqual(t, resp.Score, 100)
	assert.Contains(t, []string{"moss", "chanterelle", "russula"}, resp.Band)

	for name, facet := range map[string]models.ContactScoreFacet{
		"recency": resp.Recency, "frequency": resp.Frequency, "closeness": resp.Closeness,
		"reach_out": resp.ReachOut, "last_updated": resp.LastUpdated,
	} {
		assert.NotEmptyf(t, facet.Reason, "%s facet must always carry an explanation", name)
		assert.Greaterf(t, facet.Weight, 0.0, "%s facet must carry a non-zero weight", name)
	}
}

func TestGetContactScore_NotFound(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	req, _ := http.NewRequest("GET", "/contacts/999999/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetContactScore_ScopedToOwner(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	otherUser := models.User{Username: "other-score", Password: "x", Email: "other-score@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(othersContact.ID)+"/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetContactScore_ArchivedContactStillScored proves the deliberate
// asymmetry documented on services.ComputeContactScore: unlike the bulk
// graph endpoints (which only ever include non-archived contacts), the
// single-contact breakdown endpoint still computes a real score for an
// archived contact, matching GetContactBriefing's own no-archived-filter
// behavior.
func TestGetContactScore_ArchivedContactStillScored(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Archived", Archived: true}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp models.ContactScoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotZero(t, resp.Closeness.Weight)
}

// TestGetContactScore_Unauthorized mirrors TestGetGraph_Unauthorized: a
// request that reached this handler without passing through auth middleware
// (no "userID" in context) must 401, not panic or leak data.
func TestGetContactScore_Unauthorized(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		// Deliberately no "userID" set.
		c.Next()
	})
	router.GET("/contacts/:id/score", GetContactScore)

	req, _ := http.NewRequest("GET", "/contacts/1/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetContactScore_DatabaseError mirrors TestCreateActivity_DatabaseError:
// closing the underlying *sql.DB makes the contact lookup itself fail with a
// non-ErrRecordNotFound error, exercising the 500 branch distinct from the
// 404 branch TestGetContactScore_NotFound already covers.
func TestGetContactScore_DatabaseError(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// TestGetContactScore_ComputeFailureIsA500 exercises the branch distinct
// from TestGetContactScore_DatabaseError: the contact lookup itself
// succeeds, but the subsequent services.ComputeContactScore call fails.
// Simulated the same way services' own error-path tests are (a minimal
// schema missing cadence_policies, the first table ComputeContactScore
// queries internally) rather than closing the whole DB, which would fail
// the contact lookup too.
func TestGetContactScore_ComputeFailureIsA500(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)

	user := models.User{Username: "computefail", Password: "x", Email: "computefail@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	// Scoring's cadence-policy query is the one that fails: hide the table
	// on the real migrated schema.
	dbtest.HideTable(t, db, "cadence_policies")

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/contacts/:id/score", GetContactScore)

	req, _ := http.NewRequest("GET", "/contacts/"+idString(contact.ID)+"/score", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
