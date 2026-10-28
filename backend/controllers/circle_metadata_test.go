package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

func decodeCircleBody(t *testing.T, raw []byte) models.Circle {
	t.Helper()
	var wrapped struct {
		Circle models.Circle `json:"circle"`
	}
	require.NoError(t, json.Unmarshal(raw, &wrapped))
	if wrapped.Circle.ID != "" {
		return wrapped.Circle
	}
	var bare models.Circle
	require.NoError(t, json.Unmarshal(raw, &bare))
	return bare
}

// TestCircleController_DescriptionColor_RoundTrip: create stores a trimmed
// description and a palette colour, and GET returns them.
func TestCircleController_DescriptionColor_RoundTrip(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	resp := circleDoJSON(t, router, "POST", "/circles", map[string]any{
		"name": "Climbing", "description": "  Tuesday gym crew \n", "color": "teal",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	created := decodeCircleBody(t, resp.Body.Bytes())
	require.Equal(t, "Tuesday gym crew", created.Description)
	require.Equal(t, "teal", created.Color)

	get := circleDoJSON(t, router, "GET", "/circles/"+created.ID, nil)
	require.Equal(t, http.StatusOK, get.Code)
	got := decodeCircleBody(t, get.Body.Bytes())
	require.Equal(t, "Tuesday gym crew", got.Description)
	require.Equal(t, "teal", got.Color)
}

// TestCircleController_CreateDefaults: a name-only create (older client) gets
// the empty defaults.
func TestCircleController_CreateDefaults(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	resp := circleDoJSON(t, router, "POST", "/circles", map[string]any{"name": "Plain"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	c := decodeCircleBody(t, resp.Body.Bytes())
	require.Empty(t, c.Description)
	require.Empty(t, c.Color)
}

// TestCircleController_Validation rejects over-long descriptions and colours
// outside the palette (including CSS-looking values).
func TestCircleController_Validation(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	cases := map[string]map[string]any{
		"description too long": {"name": "x", "description": strings.Repeat("a", 501)},
		"unknown colour":       {"name": "x", "color": "chartreuse"},
		"css colour":           {"name": "x", "color": "#ff0000"},
		"css injection":        {"name": "x", "color": "red;background:url(x)"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := circleDoJSON(t, router, "POST", "/circles", body)
			require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
		})
	}

	ok := circleDoJSON(t, router, "POST", "/circles", map[string]any{"name": "x", "description": strings.Repeat("é", 500)})
	require.Equal(t, http.StatusOK, ok.Code, "500 runes is the limit, not 500 bytes: %s", ok.Body.String())
	for _, colour := range models.CircleColors {
		resp := circleDoJSON(t, router, "POST", "/circles", map[string]any{"name": "x", "color": colour})
		require.Equal(t, http.StatusOK, resp.Code, colour)
	}
}

// TestCircleController_Update_AbsentKeepsEmptyClears: a rename-only PUT keeps
// description/colour; explicit "" clears them.
func TestCircleController_Update_AbsentKeepsEmptyClears(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	circle := models.Circle{UserID: owner.ID, Name: "old", Description: "keep me", Color: "pink"}
	require.NoError(t, db.Create(&circle).Error)

	resp := circleDoJSON(t, router, "PUT", "/circles/"+circle.ID, map[string]any{"name": "renamed"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	got := decodeCircleBody(t, resp.Body.Bytes())
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "keep me", got.Description)
	require.Equal(t, "pink", got.Color)

	resp = circleDoJSON(t, router, "PUT", "/circles/"+circle.ID, map[string]any{"name": "renamed", "description": "", "color": ""})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var reloaded models.Circle
	require.NoError(t, db.First(&reloaded, "id = ?", circle.ID).Error)
	require.Empty(t, reloaded.Description)
	require.Empty(t, reloaded.Color)
}

// TestCircleController_Update_CrossUserCannotSetMetadata: another user's PUT
// 404s and leaves description/colour untouched.
func TestCircleController_Update_CrossUserCannotSetMetadata(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createCircleTestUsers(t, db)
	circle := models.Circle{UserID: owner.ID, Name: "mine", Description: "private", Color: "red"}
	require.NoError(t, db.Create(&circle).Error)

	resp := circleDoJSON(t, newCircleRealDBRouter(db, other.ID), "PUT", "/circles/"+circle.ID,
		map[string]any{"name": "mine", "description": "pwned", "color": "blue"})
	require.Equal(t, http.StatusNotFound, resp.Code)

	var reloaded models.Circle
	require.NoError(t, db.First(&reloaded, "id = ?", circle.ID).Error)
	require.Equal(t, "private", reloaded.Description)
	require.Equal(t, "red", reloaded.Color)
}
