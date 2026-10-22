package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1830: users.time_format (auto | 12h | 24h), modelled on date_format.

func patchTimeFormat(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest("PATCH", "/time-format", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestUserTimeFormat_DefaultsToAuto(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	assert.Equal(t, "auto", user.TimeFormat, "a new user is on auto")
}

func TestUpdateTimeFormat_RoundTripsEveryValue(t *testing.T) {
	db, router := setupRouter(t)
	router.PATCH("/time-format", UpdateTimeFormat)
	router.GET("/me", GetCurrentUser)
	var user models.User
	require.NoError(t, db.First(&user).Error)

	// Walk away from auto and back so an explicit "auto" is proven to be
	// stored (not dropped as a zero value) and the order doesn't matter.
	for _, want := range []string{"24h", "12h", "auto", "24h", "auto"} {
		w := patchTimeFormat(t, router, `{"time_format":"`+want+`"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, want, resp["time_format"])

		var stored models.User
		require.NoError(t, db.First(&stored, user.ID).Error)
		assert.Equal(t, want, stored.TimeFormat)

		// GET /users/me reports it, as a raw key (never omitted).
		req, _ := http.NewRequest("GET", "/me", nil)
		gw := httptest.NewRecorder()
		router.ServeHTTP(gw, req)
		require.Equal(t, http.StatusOK, gw.Code, gw.Body.String())
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &raw))
		require.Contains(t, raw, "time_format", "time_format must be present in the raw /users/me JSON")
		assert.JSONEq(t, `"`+want+`"`, string(raw["time_format"]))
	}
}

func TestUpdateTimeFormat_RejectsOtherValues(t *testing.T) {
	db, router := setupRouter(t)
	router.PATCH("/time-format", UpdateTimeFormat)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	require.NoError(t, db.Model(&user).Update("time_format", "24h").Error)

	for _, body := range []string{
		`{"time_format":"13h"}`, `{"time_format":""}`, `{"time_format":"AUTO"}`,
		`{}`, `{"time_format":24}`, `not json`,
	} {
		w := patchTimeFormat(t, router, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "INVALID_INPUT", resp["error"].(map[string]any)["code"], body)
	}
	var after models.User
	require.NoError(t, db.First(&after, user.ID).Error)
	assert.Equal(t, "24h", after.TimeFormat, "a rejected request must not write")
}

func TestUpdateTimeFormat_RequiresAuth(t *testing.T) {
	db, _ := setupRouter(t)
	var before models.User
	require.NoError(t, db.First(&before).Error)

	// A router with no auth middleware: userID is never set.
	bare := gin.New()
	bare.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	bare.PATCH("/time-format", UpdateTimeFormat)
	w := patchTimeFormat(t, bare, `{"time_format":"24h"}`)
	mzErrorCode(t, w, http.StatusUnauthorized, "UNAUTHORIZED")

	var after models.User
	require.NoError(t, db.First(&after, before.ID).Error)
	assert.Equal(t, before.TimeFormat, after.TimeFormat, "an unauthenticated request must not write")
}

func TestUpdateTimeFormat_ScopedToCaller(t *testing.T) {
	db, router := setupRouter(t)
	router.PATCH("/time-format", UpdateTimeFormat)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	w := patchTimeFormat(t, router, `{"time_format":"12h"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var o models.User
	require.NoError(t, db.First(&o, other.ID).Error)
	assert.Equal(t, "auto", o.TimeFormat, "another user's preference is untouched")
}

func TestIsValidTimeFormat(t *testing.T) {
	for _, ok := range []string{"auto", "12h", "24h"} {
		assert.True(t, models.IsValidTimeFormat(ok), ok)
	}
	for _, bad := range []string{"", "AUTO", "13h", "12", "24 h"} {
		assert.False(t, models.IsValidTimeFormat(bad), bad)
	}
}
