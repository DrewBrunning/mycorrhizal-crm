package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2006: an Activity may carry an optional end_date. It uses exactly the
// same RFC 3339 calendar-day shape as Date (ADR 0015 category 2), must not
// precede it, and on PUT an absent key keeps the stored value while an
// explicit null or "" clears it.

func activityEndDateRouter(t *testing.T) (*gorm.DB, uint, func(method, path string, body any) *httptest.ResponseRecorder) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.POST("/activities", middleware.ValidateJSONMiddleware(&models.ActivityInput{}), CreateActivity)
	router.PUT("/activities/:id", middleware.ValidateJSONMiddleware(&models.ActivityInput{}), UpdateActivity)
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, _ := http.NewRequest(method, path, bytes.NewBuffer(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	return db, user.ID, do
}

func storedEnd(t *testing.T, db *gorm.DB, id uint) *time.Time {
	t.Helper()
	var a models.Activity
	require.NoError(t, db.First(&a, id).Error)
	return a.EndDate
}

func errField(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	f, _ := body.Error.Details["field"].(string)
	return f
}

func errReason(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Error.ReasonCode
}

func TestCreateActivityEndDate(t *testing.T) {
	db, _, do := activityEndDateRouter(t)

	w := do("POST", "/activities", map[string]any{"title": "Trip", "date": "2026-05-12T00:00:00Z", "end_date": "2026-05-15T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Activity models.Activity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Activity.EndDate)
	assert.Equal(t, "2026-05-15", resp.Activity.EndDate.UTC().Format("2006-01-02"))
	assert.Contains(t, w.Body.String(), `"end_date":"2026-05-15T00:00:00Z"`)
	got := storedEnd(t, db, resp.Activity.ID)
	require.NotNil(t, got)
	assert.Equal(t, "2026-05-15", got.UTC().Format("2006-01-02"))

	// Omitted: the key is absent from the response (purely additive).
	w = do("POST", "/activities", map[string]any{"title": "Lunch", "date": "2026-05-12T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "end_date")

	// null and "" on create mean "no end date".
	for _, v := range []any{nil, ""} {
		w = do("POST", "/activities", map[string]any{"title": "Lunch", "date": "2026-05-12T00:00:00Z", "end_date": v})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "end_date")
	}
}

func TestCreateActivityEndDateZeroLengthIsValid(t *testing.T) {
	_, _, do := activityEndDateRouter(t)
	w := do("POST", "/activities", map[string]any{"title": "Same day", "date": "2026-05-12T00:00:00Z", "end_date": "2026-05-12T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"end_date":"2026-05-12T00:00:00Z"`)
}

func TestCreateActivityEndDateBeforeStartIs400OnEndDate(t *testing.T) {
	db, _, do := activityEndDateRouter(t)
	w := do("POST", "/activities", map[string]any{"title": "Bad", "date": "2026-05-12T00:00:00Z", "end_date": "2026-05-11T00:00:00Z"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "end_date", errField(t, w))
	assert.Equal(t, "activity_end_before_start", errReason(t, w))
	var n int64
	require.NoError(t, db.Model(&models.Activity{}).Where("title = ?", "Bad").Count(&n).Error)
	assert.Zero(t, n)
}

// An end without a start cannot exist: date stays required.
func TestCreateActivityEndDateWithoutStartIsRejected(t *testing.T) {
	_, _, do := activityEndDateRouter(t)
	w := do("POST", "/activities", map[string]any{"title": "Orphan end", "end_date": "2026-05-15T00:00:00Z"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateActivityEndDateMalformedIs400(t *testing.T) {
	_, _, do := activityEndDateRouter(t)
	w := do("POST", "/activities", map[string]any{"title": "Bad", "date": "2026-05-12T00:00:00Z", "end_date": "next friday"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = do("POST", "/activities", map[string]any{"title": "Bad", "date": "2026-05-12T00:00:00Z", "end_date": 12})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestUpdateActivityEndDate(t *testing.T) {
	db, uid, do := activityEndDateRouter(t)
	start := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	a := models.Activity{UserID: uid, Title: "Trip", Date: start, EndDate: &end}
	require.NoError(t, db.Create(&a).Error)
	path := "/activities/" + strconv.Itoa(int(a.ID))

	// Absent key keeps the stored end date (a client that does not know the
	// field must not wipe it).
	w := do("PUT", path, map[string]any{"title": "Trip 2", "date": "2026-05-12T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"end_date":"2026-05-15T00:00:00Z"`)
	require.NotNil(t, storedEnd(t, db, a.ID))

	// Extending the range.
	w = do("PUT", path, map[string]any{"title": "Trip 2", "date": "2026-05-12T00:00:00Z", "end_date": "2026-05-20T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "2026-05-20", storedEnd(t, db, a.ID).UTC().Format("2006-01-02"))

	// Moving the start past the (kept) end is a 400 on end_date and changes
	// nothing -- the end is never cleared silently.
	w = do("PUT", path, map[string]any{"title": "Moved", "date": "2026-05-25T00:00:00Z"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "end_date", errField(t, w))
	var after models.Activity
	require.NoError(t, db.First(&after, a.ID).Error)
	assert.Equal(t, "Trip 2", after.Title)
	assert.Equal(t, "2026-05-12", after.Date.UTC().Format("2006-01-02"))
	assert.Equal(t, "2026-05-20", after.EndDate.UTC().Format("2006-01-02"))

	// Explicit end before start is also a 400.
	w = do("PUT", path, map[string]any{"title": "x", "date": "2026-05-12T00:00:00Z", "end_date": "2026-05-01T00:00:00Z"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "end_date", errField(t, w))

	// Moving both together works.
	w = do("PUT", path, map[string]any{"title": "Moved", "date": "2026-05-25T00:00:00Z", "end_date": "2026-05-27T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Zero-length is valid.
	w = do("PUT", path, map[string]any{"title": "Moved", "date": "2026-05-25T00:00:00Z", "end_date": "2026-05-25T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUpdateActivityEndDateExplicitClear(t *testing.T) {
	for name, v := range map[string]any{"null": nil, "empty string": ""} {
		t.Run(name, func(t *testing.T) {
			db, uid, do := activityEndDateRouter(t)
			end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
			a := models.Activity{UserID: uid, Title: "Trip", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), EndDate: &end}
			require.NoError(t, db.Create(&a).Error)

			w := do("PUT", "/activities/"+strconv.Itoa(int(a.ID)), map[string]any{"title": "Trip", "date": "2026-05-12T00:00:00Z", "end_date": v})
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.NotContains(t, w.Body.String(), "end_date")
			assert.Nil(t, storedEnd(t, db, a.ID))

			// Clearing also lets the start move anywhere.
			a2 := models.Activity{UserID: uid, Title: "Trip", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), EndDate: &end}
			require.NoError(t, db.Create(&a2).Error)
			w = do("PUT", "/activities/"+strconv.Itoa(int(a2.ID)), map[string]any{"title": "Trip", "date": "2026-06-30T00:00:00Z", "end_date": v})
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Nil(t, storedEnd(t, db, a2.ID))
		})
	}
}

// Ownership: another user's activity is a 404 and its end date is untouched.
func TestUpdateActivityEndDateCrossUserScoping(t *testing.T) {
	db, _, do := activityEndDateRouter(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	theirs := models.Activity{UserID: other.ID, Title: "Theirs", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), EndDate: &end}
	require.NoError(t, db.Create(&theirs).Error)

	for _, v := range []any{nil, "2026-07-01T00:00:00Z"} {
		w := do("PUT", "/activities/"+strconv.Itoa(int(theirs.ID)), map[string]any{"title": "Hijack", "date": "2026-05-12T00:00:00Z", "end_date": v})
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	}
	var after models.Activity
	require.NoError(t, db.First(&after, theirs.ID).Error)
	assert.Equal(t, "Theirs", after.Title)
	assert.Equal(t, "2026-05-15", after.EndDate.UTC().Format("2006-01-02"))
}
