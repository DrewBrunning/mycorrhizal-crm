package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1796: GET /graph scores the contacts it loaded itself (a narrow
// Select), so that Select must carry target_closeness or a targeted contact's
// graph health score silently disagrees with GET /contacts/:id/score (the
// target tier sets the Recency/Frequency interval).
func TestGetGraph_HealthScoreHonoursTargetCloseness(t *testing.T) {
	db, router, user := targetRouter(t)
	router.GET("/graph", GetGraph)

	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	activity := models.Activity{UserID: user.ID, Title: "Coffee", Date: time.Now().AddDate(0, 0, -40)}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Model(&activity).Association("Contacts").Append(&contact))

	endpointScore := func() int {
		t.Helper()
		w := tcJSON(router, http.MethodGet, "/contacts/"+idString(contact.ID)+"/score", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp models.ContactScoreResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Score
	}
	graphScore := func() int {
		t.Helper()
		w := tcJSON(router, http.MethodGet, "/graph", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp models.GraphResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		node := findGraphNode(resp.Nodes, "c-"+idString(contact.ID))
		require.NotNil(t, node)
		require.NotNil(t, node.HealthScore)
		return *node.HealthScore
	}

	untargeted := endpointScore()
	require.Equal(t, untargeted, graphScore())

	require.Equal(t, http.StatusOK, tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "immediate"}).Code)
	targeted := endpointScore()
	require.NotEqual(t, untargeted, targeted, "precondition: the 14-day immediate target must move the score")
	assert.Equal(t, targeted, graphScore(), "the graph node's health score must match the score endpoint for a targeted contact")
}
