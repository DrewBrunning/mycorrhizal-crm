package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
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
	db := dbtest.New(t)

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

// TestGraphConnections_DeceasedContactDecoration is the regression test for
// issue #1193: GetGraphConnections must decorate a chain target with
// Deceased=true when that contact records a death anniversary
// (Card.Anniversaries[kind=death]) -- this is Android's only surface for the
// deceased state (see GraphChain's doc comment), so it must always be
// populated here, not just on GetGraph's canvas nodes.
func TestGraphConnections_DeceasedContactDecoration(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "graph-deceased", Password: "password123!A", Email: "graph-deceased@example.com"}
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

	anchor := models.Contact{UserID: user.ID, Firstname: "Anchor"}
	require.NoError(t, db.Create(&anchor).Error)

	deceased := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&deceased, &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Departed"}}},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "death", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{
					Year: intPtr(2020), Month: intPtr(5), Day: intPtr(1),
				}}},
			},
		},
	}, "")
	require.NoError(t, db.Create(&deceased).Error)

	alive := models.Contact{UserID: user.ID, Firstname: "Alive"}
	require.NoError(t, db.Create(&alive).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: anchor.VCardUID, TargetID: deceased.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: anchor.VCardUID, TargetID: alive.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	req, _ := http.NewRequest("GET", "/graph/connections?from="+anchor.VCardUID+"&depth=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Chains, 2)

	for _, chain := range resp.Chains {
		if chain.TargetVCardUID == deceased.VCardUID {
			assert.True(t, chain.Deceased, "the deceased target must be decorated Deceased=true")
		} else {
			assert.False(t, chain.Deceased, "a living target must not be marked deceased")
		}
	}
}
