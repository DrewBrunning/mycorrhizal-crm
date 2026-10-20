package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1791: GET /graph defaults to the ego-network of the caller's self
// contact instead of the whole account, and every read is bound to the
// request context.

func seedFriendEdge(t *testing.T, db *gorm.DB, userID uint, a, b models.Contact) {
	t.Helper()
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: userID, SourceID: a.VCardUID, TargetID: b.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
}

func getGraphResp(t *testing.T, router http.Handler, path string, wantStatus int) models.GraphResponse {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, wantStatus, w.Code, w.Body.String())
	var resp models.GraphResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	}
	return resp
}

func nodeIDs(nodes []models.GraphNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func contactNodeID(c models.Contact) string { return "c-" + itoa(c.ID) }

func TestGetGraph_DefaultsToEgoNetworkOfSelfContact(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	mk := func(name string) models.Contact {
		c := models.Contact{UserID: user.ID, Firstname: name}
		require.NoError(t, db.Create(&c).Error)
		return c
	}
	self, a, b, far, island := mk("Self"), mk("A"), mk("B"), mk("Far"), mk("Island")
	require.NoError(t, db.Model(&user).Update("self_contact_vcard_uid", self.VCardUID).Error)
	// self - a - b - far (a chain) plus an unconnected island.
	seedFriendEdge(t, db, user.ID, self, a)
	seedFriendEdge(t, db, user.ID, a, b)
	seedFriendEdge(t, db, user.ID, b, far)

	// An activity between a and b is inside the radius; one between far and
	// island has no in-scope participant at all; one between b and far has a
	// single in-scope participant (b) and so must not become a node.
	inside := models.Activity{UserID: user.ID, Title: "Inside", Date: time.Now()}
	outside := models.Activity{UserID: user.ID, Title: "Outside", Date: time.Now()}
	straddle := models.Activity{UserID: user.ID, Title: "Straddle", Date: time.Now()}
	for _, act := range []*models.Activity{&inside, &outside, &straddle} {
		require.NoError(t, db.Create(act).Error)
	}
	require.NoError(t, db.Model(&inside).Association("Contacts").Append(&a, &b))
	require.NoError(t, db.Model(&outside).Association("Contacts").Append(&far, &island))
	require.NoError(t, db.Model(&straddle).Association("Contacts").Append(&b, &far))

	t.Run("default is depth 2 around the self contact", func(t *testing.T) {
		resp := getGraphResp(t, router, "/graph", http.StatusOK)
		assert.True(t, resp.Scoped)
		assert.Equal(t, self.VCardUID, resp.FromVCardUID)
		assert.Equal(t, 2, resp.Depth)
		assert.ElementsMatch(t, []string{
			contactNodeID(self), contactNodeID(a), contactNodeID(b), "a-" + itoa(inside.ID),
		}, nodeIDs(resp.Nodes))
		// self-a, a-b relationship edges + the two star edges of "Inside".
		assert.Len(t, resp.Edges, 4)
		for _, e := range resp.Edges {
			assert.NotEmpty(t, findGraphNode(resp.Nodes, e.Source), "edge %s source must be a returned node", e.ID)
			assert.NotEmpty(t, findGraphNode(resp.Nodes, e.Target), "edge %s target must be a returned node", e.ID)
		}
	})

	t.Run("depth widens the radius", func(t *testing.T) {
		resp := getGraphResp(t, router, "/graph?depth=3", http.StatusOK)
		assert.Contains(t, nodeIDs(resp.Nodes), contactNodeID(far))
		assert.NotContains(t, nodeIDs(resp.Nodes), contactNodeID(island))
		// "Straddle" now has two in-scope participants.
		assert.Contains(t, nodeIDs(resp.Nodes), "a-"+itoa(straddle.ID))
		assert.NotContains(t, nodeIDs(resp.Nodes), "a-"+itoa(outside.ID))

		one := getGraphResp(t, router, "/graph?depth=1", http.StatusOK)
		assert.ElementsMatch(t, []string{contactNodeID(self), contactNodeID(a)}, nodeIDs(one.Nodes))
	})

	t.Run("from re-centres the ego-network", func(t *testing.T) {
		resp := getGraphResp(t, router, "/graph?from="+b.VCardUID+"&depth=1", http.StatusOK)
		assert.True(t, resp.Scoped)
		assert.Equal(t, b.VCardUID, resp.FromVCardUID)
		assert.ElementsMatch(t, []string{
			contactNodeID(a), contactNodeID(b), contactNodeID(far), "a-" + itoa(inside.ID), "a-" + itoa(straddle.ID),
		}, nodeIDs(resp.Nodes))
	})

	t.Run("full returns the whole account", func(t *testing.T) {
		resp := getGraphResp(t, router, "/graph?full=true", http.StatusOK)
		assert.False(t, resp.Scoped)
		assert.Empty(t, resp.FromVCardUID)
		assert.Len(t, resp.Nodes, 5+3)

		off := getGraphResp(t, router, "/graph?full=false", http.StatusOK)
		assert.True(t, off.Scoped)
	})
}

// The ego-network is the visible graph's neighbourhood: a hop never lands on
// (or walks through) an archived or soft-deleted contact, and a soft-deleted
// activity participant is not counted. Otherwise a contact two hops away via a
// hidden middle node shows up in "within 2 connections of you" with no path
// back, and an activity star edge points at a node the response does not have.
func TestGetGraph_EgoNetworkSkipsHiddenContacts(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	mk := func(name string) models.Contact {
		c := models.Contact{UserID: user.ID, Firstname: name}
		require.NoError(t, db.Create(&c).Error)
		return c
	}
	self, archived, behindArchived, deleted, behindDeleted, friend := mk("Self"), mk("Archived"), mk("BehindArchived"), mk("Deleted"), mk("BehindDeleted"), mk("Friend")
	require.NoError(t, db.Model(&user).Update("self_contact_vcard_uid", self.VCardUID).Error)
	seedFriendEdge(t, db, user.ID, self, archived)
	seedFriendEdge(t, db, user.ID, archived, behindArchived)
	seedFriendEdge(t, db, user.ID, deleted, self) // the source side of the walk
	seedFriendEdge(t, db, user.ID, behindDeleted, deleted)
	seedFriendEdge(t, db, user.ID, self, friend)

	// An activity between friend and the (soon) deleted contact: only one
	// visible participant, so no activity node in either view.
	act := models.Activity{UserID: user.ID, Title: "WithDeleted", Date: time.Now()}
	require.NoError(t, db.Create(&act).Error)
	require.NoError(t, db.Model(&act).Association("Contacts").Append(&friend, &deleted))

	require.NoError(t, db.Model(&archived).Update("archived", true).Error)
	// A bare soft delete (no cascade) leaves the edges and the activity link
	// in place -- the graph must not lean on the cascade to hide them.
	require.NoError(t, db.Delete(&deleted).Error)

	resp := getGraphResp(t, router, "/graph", http.StatusOK)
	require.True(t, resp.Scoped)
	assert.ElementsMatch(t, []string{contactNodeID(self), contactNodeID(friend)}, nodeIDs(resp.Nodes))

	full := getGraphResp(t, router, "/graph?full=true", http.StatusOK)
	assert.NotContains(t, nodeIDs(full.Nodes), "a-"+itoa(act.ID))
	for _, e := range full.Edges {
		assert.NotEmpty(t, findGraphNode(full.Nodes, e.Target), "edge %s target must be a returned node", e.ID)
	}
}

func TestGetGraph_NoSelfContactFallsBackToFullGraph(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	for _, n := range []string{"One", "Two"} {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: n}).Error)
	}

	resp := getGraphResp(t, router, "/graph", http.StatusOK)
	assert.False(t, resp.Scoped, "no anchor to centre on, so the full graph")
	assert.Len(t, resp.Nodes, 2)
}

func TestGetGraph_RejectsBadScopeParameters(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)
	var user models.User
	require.NoError(t, db.First(&user).Error)

	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Foreign"}
	require.NoError(t, db.Create(&foreign).Error)

	for _, path := range []string{"/graph?depth=0", "/graph?depth=6", "/graph?depth=abc", "/graph?full=maybe"} {
		getGraphResp(t, router, path, http.StatusBadRequest)
	}
	getGraphResp(t, router, "/graph?from=does-not-exist", http.StatusNotFound)
	// Ownership scoping: another user's contact is indistinguishable from a
	// missing one.
	getGraphResp(t, router, "/graph?from="+foreign.VCardUID, http.StatusNotFound)
}

// A request whose context is already cancelled (the client went away) must
// stop at the first query: no body is written and nothing is reported as a
// server error.
func TestGraphAndDashboard_StopWhenClientDisconnects(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)
	router.GET("/graph/connections", GetGraphConnections)
	router.GET("/dashboard", GetDashboard)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	for _, path := range []string{"/graph", "/graph?full=true", "/graph/connections?from=" + contact.VCardUID, "/dashboard"} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Empty(t, w.Body.String(), "a cancelled request must not produce a response body")
			assert.NotEqual(t, http.StatusInternalServerError, w.Code)
		})
	}
}

func TestGetGraph_ScopeLookupFailuresUseErrorEnvelope(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test logs: Request error")
	t.Run("users table unavailable", func(t *testing.T) {
		db, router := setupRouter(t)
		router.GET("/graph", GetGraph)
		dbtest.HideTable(t, db, "users")
		getGraphResp(t, router, "/graph", http.StatusInternalServerError)
	})
	t.Run("contacts table unavailable for from lookup", func(t *testing.T) {
		db, router := setupRouter(t)
		router.GET("/graph", GetGraph)
		dbtest.HideTable(t, db, "contacts")
		getGraphResp(t, router, "/graph?from=abc", http.StatusInternalServerError)
	})
}

// A user row that no longer exists has no self contact to centre on: the full
// (empty) graph, not an error.
func TestGetGraph_MissingUserRowFallsBackToFull(t *testing.T) {
	db := dbtest.New(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(424242))
		c.Next()
	})
	router.GET("/graph", GetGraph)
	resp := getGraphResp(t, router, "/graph", http.StatusOK)
	assert.False(t, resp.Scoped)
	assert.Empty(t, resp.Nodes)
}

func TestGetGraphConnections_TooDeepAndBlankName(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/graph/connections", GetGraphConnections)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	blank := models.Contact{UserID: user.ID}
	require.NoError(t, db.Create(&blank).Error)

	req, _ := http.NewRequest("GET", "/graph/connections?depth=6&from="+blank.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req, _ = http.NewRequest("GET", "/graph/connections?from="+blank.VCardUID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.GraphConnectionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Unknown", resp.FromName)
}
