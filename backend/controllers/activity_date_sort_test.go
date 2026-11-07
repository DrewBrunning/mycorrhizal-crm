package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type activityDatePage struct {
	Activities []models.Activity `json:"activities"`
	NextCursor string            `json:"next_cursor"`
}

func getActivityDatePage(t *testing.T, router http.Handler, query string) (int, activityDatePage) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/activities?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var page activityDatePage
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	return w.Code, page
}

// Issue #2360: ?sort=date orders by the activity's own date, newest first,
// so editing an old activity (bumping updated_at) does not move it up.
func TestGetActivitiesSortDateIgnoresLastModified(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/activities", GetActivities)

	old := models.Activity{UserID: user.ID, Title: "Old", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	recent := models.Activity{UserID: user.ID, Title: "Recent", Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&recent).Error)
	require.NoError(t, db.Create(&old).Error)
	// "Edit" the old activity today.
	require.NoError(t, db.Model(&old).Update("updated_at", time.Now().Add(time.Hour)).Error)

	code, page := getActivityDatePage(t, router, "sort=date")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, page.Activities, 2)
	assert.Equal(t, "Recent", page.Activities[0].Title)
	assert.Equal(t, "Old", page.Activities[1].Title)

	// Default (sync) order is unchanged: last modified first.
	_, def := getActivityDatePage(t, router, "")
	require.Len(t, def.Activities, 2)
	assert.Equal(t, "Old", def.Activities[0].Title)
}

// Pagination across the date boundary (including same-date ties) returns
// every row exactly once, in (date, id) descending order.
func TestGetActivitiesSortDatePaginationNoDupesNoGaps(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/activities", GetActivities)

	const n = 9
	for i := 0; i < n; i++ {
		// Three activities per day, so page boundaries land on ties.
		day := time.Date(2026, 1, 1+i/3, 0, 0, 0, 0, time.UTC)
		a := models.Activity{UserID: user.ID, Title: fmt.Sprintf("A%d", i), Date: day}
		require.NoError(t, db.Create(&a).Error)
		// Scramble updated_at so it disagrees with date order.
		require.NoError(t, db.Model(&a).Update("updated_at", time.Now().Add(time.Duration((i*7)%n)*time.Minute)).Error)
	}

	seen := map[uint]bool{}
	var order []models.Activity
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		q := "sort=date&limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		code, page := getActivityDatePage(t, router, q)
		require.Equal(t, http.StatusOK, code)
		for _, a := range page.Activities {
			require.False(t, seen[a.ID], "duplicate activity %d", a.ID)
			seen[a.ID] = true
			order = append(order, a)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	require.Len(t, order, n, "no gaps")
	for i := 1; i < len(order); i++ {
		prev, cur := order[i-1], order[i]
		ok := prev.Date.After(cur.Date) || (prev.Date.Equal(cur.Date) && prev.ID > cur.ID)
		assert.True(t, ok, "order must be (date, id) desc at index %d", i)
	}

	// asc direction reverses it.
	_, asc := getActivityDatePage(t, router, "sort=date&order=asc&limit=1")
	require.Len(t, asc.Activities, 1)
	assert.Equal(t, order[len(order)-1].ID, asc.Activities[0].ID)
}

func TestGetActivitiesSortValidation(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/activities", GetActivities)

	code, _ := getActivityDatePage(t, router, "sort=bogus")
	assert.Equal(t, http.StatusBadRequest, code)

	// sort=date is list browsing, not sync state: it cannot be a change feed.
	code, _ = getActivityDatePage(t, router, "sort=date&since="+EncodeCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)

	// An updated_at cursor must not be silently applied under sort=date.
	code, _ = getActivityDatePage(t, router, "sort=date&cursor="+EncodeCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestDateCursorEncodeDecodeRoundTrip(t *testing.T) {
	ts := time.Date(2026, 6, 1, 0, 0, 0, 0, time.FixedZone("CDT", -5*3600))
	cur, err := DecodeDateCursor(EncodeDateCursor(ts, uint(7)))
	require.NoError(t, err)
	assert.True(t, ts.Equal(cur.UpdatedAt))
	assert.Equal(t, "7", cur.ID)
	_, err = DecodeDateCursor(EncodeCursor(ts, uint(7)))
	assert.Error(t, err)
	_, err = DecodeDateCursor(EncodeCreatedCursor(ts, uint(7)))
	assert.Error(t, err, "created_at cursor must not decode as a date cursor")
}

func TestDateCursorDecodeRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		errFrag string
	}{
		{"!!!not-base64url!!!", "cursor is not valid base64url"},
		{encodeRawURL("date:"), "cursor is malformed"},
		{encodeRawURL("date:2026-06-01T00:00:00Z|"), "cursor is malformed"},
		{encodeRawURL("date:|7"), "cursor is malformed"},
		{encodeRawURL("date:not-a-time|7"), "cursor timestamp is malformed"},
	} {
		_, err := DecodeDateCursor(tc.raw)
		assert.ErrorContains(t, err, tc.errFrag, "cursor %q should fail to decode", tc.raw)
	}
}

// A well-formed date cursor whose id is not numeric is a 400, not a 500.
func TestGetActivitiesSortDateRejectsNonNumericCursorID(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/activities", GetActivities)

	cursor := encodeRawURL("date:2026-06-01T00:00:00Z|abc")
	code, _ := getActivityDatePage(t, router, "sort=date&cursor="+cursor)
	assert.Equal(t, http.StatusBadRequest, code)
}

// Same guard on the default (updated_at) order.
func TestGetActivitiesRejectsNonNumericCursorID(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/activities", GetActivities)

	cursor := encodeRawURL("2026-06-01T00:00:00Z|abc")
	code, _ := getActivityDatePage(t, router, "cursor="+cursor)
	assert.Equal(t, http.StatusBadRequest, code)
}
