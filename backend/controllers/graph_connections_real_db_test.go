package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
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

// TestGraphConnections_RealMigratedSchema is the real-DB check for T10: the
// recursive-CTE traversal runs against a database.InitDB-migrated file DB so
// the SQL touches the real relationship_edges schema (column names, indexes),
// not the AutoMigrate-derived one. Exercises a two-hop chain with the
// direction-aware labels.
func TestGraphConnections_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "graph-connections-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "graph-realdb", Password: "password123!A", Email: "graph-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.GET("/graph/connections", GetGraphConnections)

	parent := models.Contact{UserID: user.ID, Firstname: "Parent"}
	child := models.Contact{UserID: user.ID, Firstname: "Child"}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&child).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: parent.VCardUID, TargetID: child.VCardUID, Type: "parent_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	// From the child, the parent is reachable with the stored type (parent_of).
	req, _ := http.NewRequest("GET", "/graph/connections?from="+child.VCardUID+"&depth=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Chains, 1)
	assert.Equal(t, parent.VCardUID, resp.Chains[0].TargetVCardUID)
	require.Len(t, resp.Chains[0].Steps, 1)
	assert.Equal(t, "parent_of", resp.Chains[0].Steps[0].Relation)

	// From the parent, the child is reachable with the inverse (child_of).
	req2, _ := http.NewRequest("GET", "/graph/connections?from="+parent.VCardUID+"&depth=2", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var resp2 models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Len(t, resp2.Chains, 1)
	require.Len(t, resp2.Chains[0].Steps, 1)
	assert.Equal(t, "child_of", resp2.Chains[0].Steps[0].Relation)
}
