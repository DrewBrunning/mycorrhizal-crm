package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuggestRelationshipEdges_RealMigratedSchema exercises the T104 trigger
// against a database.InitDB-migrated real file database: the graph engine
// infers suggestions from confirmed edges, stamps them graph-inferred, never
// duplicates on re-run, and stays scoped to the caller's own data.
func TestSuggestRelationshipEdges_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "relationship-suggest-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "suggesttester", Password: "password123!A", Email: "suggest@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "othertester", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	contact := func(firstname string) models.Contact {
		return models.Contact{UserID: user.ID, Firstname: firstname}
	}
	p1 := contact("P1")
	c1 := contact("C1")
	c2 := contact("C2")
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)

	edge := func(ownerID uint, source, target, edgeType string) models.RelationshipEdge {
		return models.RelationshipEdge{
			UserID: ownerID, SourceID: source, TargetID: target, Type: edgeType,
			Directional: !models.IsSymmetricRelationType(edgeType),
			Source:      models.RelationshipSourceUserConfirmed, Confidence: 1.0,
			Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
		}
	}
	parentEdge := edge(user.ID, p1.VCardUID, c1.VCardUID, "parent_of")
	require.NoError(t, db.Create(&parentEdge).Error)
	siblingEdge := edge(user.ID, c1.VCardUID, c2.VCardUID, "sibling_of")
	require.NoError(t, db.Create(&siblingEdge).Error)

	// Another user's confirmed edge must never seed this user's suggestions.
	op := models.Contact{UserID: other.ID, Firstname: "Other"}
	require.NoError(t, db.Create(&op).Error)
	otherEdge := edge(other.ID, op.VCardUID, p1.VCardUID, "sibling_of")
	require.NoError(t, db.Create(&otherEdge).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/relationship-edges/suggest", SuggestRelationshipEdges)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// First press: parent·sibling -> parent_of(P1, C2).
	first := doJSON("POST", "/relationship-edges/suggest", nil)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstResp struct {
		SuggestedEdges []models.RelationshipEdge `json:"suggested_edges"`
		Total          int                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.Equal(t, 1, firstResp.Total)
	require.Len(t, firstResp.SuggestedEdges, 1)
	got := firstResp.SuggestedEdges[0]
	assert.Equal(t, p1.VCardUID, got.SourceID)
	assert.Equal(t, c2.VCardUID, got.TargetID)
	assert.Equal(t, "parent_of", got.Type)
	assert.Equal(t, models.RelationshipSourceGraphInferred, got.Source)
	assert.Equal(t, models.RelationshipStatusSuggested, got.Status)
	assert.Equal(t, models.RelationshipSensitivityNormal, got.Sensitivity)
	assert.Equal(t, 0.7, got.Confidence)

	// Second press: idempotent — no new edges.
	second := doJSON("POST", "/relationship-edges/suggest", nil)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	var secondResp struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))
	assert.Equal(t, 0, secondResp.Total)

	// The other user's edges were never touched: still exactly one edge for
	// them, and it is confirmed.
	var otherEdges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", other.ID).Find(&otherEdges).Error)
	require.Len(t, otherEdges, 1)
	assert.Equal(t, models.RelationshipStatusConfirmed, otherEdges[0].Status)
}
