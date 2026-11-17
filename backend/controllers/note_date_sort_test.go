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

type noteDatePage struct {
	Notes      []models.Note `json:"notes"`
	NextCursor string        `json:"next_cursor"`
	Total      int64         `json:"total"`
}

func getNoteDatePage(t *testing.T, router http.Handler, query string) (int, noteDatePage, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/notes?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var page noteDatePage
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	return w.Code, page, w.Body.String()
}

// Issue #2744 (item 1): GET /notes?sort=date orders by the note's own date,
// newest first, so editing an old note (bumping updated_at) does not move it.
// Mirrors GET /activities?sort=date (#2360).
func TestGetNotesSortDateIgnoresLastModified(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/notes", GetUnassignedNotes)

	old := models.Note{UserID: user.ID, Content: "Old", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	recent := models.Note{UserID: user.ID, Content: "Recent", Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&recent).Error)
	require.NoError(t, db.Create(&old).Error)
	// "Edit" the old note today.
	require.NoError(t, db.Model(&old).Update("updated_at", time.Now().Add(time.Hour)).Error)

	code, page, body := getNoteDatePage(t, router, "sort=date")
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, page.Notes, 2)
	assert.Equal(t, "Recent", page.Notes[0].Content)
	assert.Equal(t, "Old", page.Notes[1].Content)
	assert.EqualValues(t, 2, page.Total, "total is unchanged by the sort")

	// Default (sync) order is unchanged: last modified first.
	_, def, _ := getNoteDatePage(t, router, "")
	require.Len(t, def.Notes, 2)
	assert.Equal(t, "Old", def.Notes[0].Content)
}

// Pagination across the date boundary, including same-date ties and a mix of
// filed and unfiled notes (scope=all), returns every row exactly once in
// (date, id) descending order, and asc reverses it.
func TestGetNotesSortDatePaginationNoDupesNoGaps(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/notes", GetUnassignedNotes)
	contact := models.Contact{UserID: user.ID, Firstname: "Filed"}
	require.NoError(t, db.Create(&contact).Error)

	const n = 9
	for i := 0; i < n; i++ {
		// Three notes per day, so page boundaries (limit=2) land on ties.
		day := time.Date(2026, 1, 1+i/3, 0, 0, 0, 0, time.UTC)
		note := models.Note{UserID: user.ID, Content: fmt.Sprintf("N%d", i), Date: day}
		if i%2 == 0 {
			note.ContactID = &contact.ID
		}
		require.NoError(t, db.Create(&note).Error)
		// Scramble updated_at so it disagrees with date order.
		require.NoError(t, db.Model(&note).Update("updated_at", time.Now().Add(time.Duration((i*7)%n)*time.Minute)).Error)
	}

	seen := map[uint]bool{}
	var order []models.Note
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		q := "sort=date&scope=all&limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		code, page, body := getNoteDatePage(t, router, q)
		require.Equal(t, http.StatusOK, code, body)
		for _, nt := range page.Notes {
			require.False(t, seen[nt.ID], "duplicate note %d", nt.ID)
			seen[nt.ID] = true
			order = append(order, nt)
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

	_, asc, _ := getNoteDatePage(t, router, "sort=date&scope=all&order=asc&limit=1")
	require.Len(t, asc.Notes, 1)
	assert.Equal(t, order[len(order)-1].ID, asc.Notes[0].ID)

	// The default scope still lists only unfiled notes under sort=date.
	_, unfiled, _ := getNoteDatePage(t, router, "sort=date&limit=50")
	assert.Len(t, unfiled.Notes, 4)
}

// A user's date-sorted list never includes another user's notes.
func TestGetNotesSortDateIsOwnerScoped(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/notes", GetUnassignedNotes)
	other := models.User{Username: "dateother", Password: "x", Email: "dateother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Note{UserID: other.ID, Content: "theirs", Date: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "mine", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}).Error)

	_, page, _ := getNoteDatePage(t, router, "sort=date&scope=all")
	require.Len(t, page.Notes, 1)
	assert.Equal(t, "mine", page.Notes[0].Content)
}

func TestGetNotesSortValidation(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/notes", GetUnassignedNotes)

	code, _, body := getNoteDatePage(t, router, "sort=bogus")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, `"reason_code":"validation_sort_invalid"`)

	// sort=date is list browsing, not sync state: it cannot be a change feed.
	code, _, _ = getNoteDatePage(t, router, "sort=date&since="+EncodeCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)

	// An updated_at cursor must not be silently applied under sort=date.
	code, _, _ = getNoteDatePage(t, router, "sort=date&cursor="+EncodeCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)

	// An explicit sort=updated_at is the default.
	code, _, _ = getNoteDatePage(t, router, "sort=updated_at")
	assert.Equal(t, http.StatusOK, code)
}
