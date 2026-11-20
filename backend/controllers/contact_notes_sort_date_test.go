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

func getContactNotesPage(t *testing.T, router http.Handler, contactID uint, query string) (int, noteDatePage, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/contacts/%d/notes?%s", contactID, query), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var page noteDatePage
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	return w.Code, page, w.Body.String()
}

// Issue #2797: GET /contacts/:id/notes?sort=date orders by the note's own
// date, newest first (id tiebreak), exactly like GET /notes?sort=date (#2753),
// and the default stays the last-modified order.
func TestGetNotesForContactSortDateIgnoresLastModified(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/contacts/:id/notes", GetNotesForContact)
	contact := models.Contact{UserID: user.ID, Firstname: "C"}
	require.NoError(t, db.Create(&contact).Error)

	recent := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "Recent", Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	old := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "Old", Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&recent).Error)
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Model(&old).Update("updated_at", time.Now().Add(time.Hour)).Error)

	code, page, body := getContactNotesPage(t, router, contact.ID, "sort=date")
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, page.Notes, 2)
	assert.Equal(t, "Recent", page.Notes[0].Content)
	assert.Equal(t, "Old", page.Notes[1].Content)

	_, def, _ := getContactNotesPage(t, router, contact.ID, "")
	require.Len(t, def.Notes, 2)
	assert.Equal(t, "Old", def.Notes[0].Content)
	_, explicit, _ := getContactNotesPage(t, router, contact.ID, "sort=updated_at")
	assert.Equal(t, "Old", explicit.Notes[0].Content)
}

// Paging with limit=2 over equal dates lands page boundaries on ties; every
// row appears exactly once in (date, id) order, and the search filter and
// other contacts' notes stay out of the way.
func TestGetNotesForContactSortDatePageBoundaryEqualDates(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/contacts/:id/notes", GetNotesForContact)
	contact := models.Contact{UserID: user.ID, Firstname: "C"}
	other := models.Contact{UserID: user.ID, Firstname: "O"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &other.ID, Content: "foreign-contact", Date: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}).Error)

	const n = 7
	for i := 0; i < n; i++ {
		day := time.Date(2026, 1, 1+i/3, 0, 0, 0, 0, time.UTC)
		note := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: fmt.Sprintf("N%d", i), Date: day}
		require.NoError(t, db.Create(&note).Error)
		require.NoError(t, db.Model(&note).Update("updated_at", time.Now().Add(time.Duration((i*5)%n)*time.Minute)).Error)
	}

	run := func(order string) []models.Note {
		var out []models.Note
		seen := map[uint]bool{}
		cursor := ""
		for pages := 0; pages < 10; pages++ {
			q := "sort=date&limit=2&order=" + order
			if cursor != "" {
				q += "&cursor=" + cursor
			}
			code, page, body := getContactNotesPage(t, router, contact.ID, q)
			require.Equal(t, http.StatusOK, code, body)
			for _, nt := range page.Notes {
				require.False(t, seen[nt.ID], "duplicate note %d", nt.ID)
				seen[nt.ID] = true
				out = append(out, nt)
			}
			cursor = page.NextCursor
			if cursor == "" {
				break
			}
		}
		require.Len(t, out, n, "no gaps")
		return out
	}
	desc := run("desc")
	for i := 1; i < len(desc); i++ {
		p, c := desc[i-1], desc[i]
		assert.True(t, p.Date.After(c.Date) || (p.Date.Equal(c.Date) && p.ID > c.ID), "desc (date,id) at %d", i)
	}
	asc := run("asc")
	for i := 1; i < len(asc); i++ {
		p, c := asc[i-1], asc[i]
		assert.True(t, p.Date.Before(c.Date) || (p.Date.Equal(c.Date) && p.ID < c.ID), "asc (date,id) at %d", i)
	}
}

func TestGetNotesForContactSortValidation(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/contacts/:id/notes", GetNotesForContact)
	contact := models.Contact{UserID: user.ID, Firstname: "C"}
	require.NoError(t, db.Create(&contact).Error)

	code, _, body := getContactNotesPage(t, router, contact.ID, "sort=bogus")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, `"reason_code":"validation_sort_invalid"`)

	// A cursor from the other sort is rejected, both directions.
	code, _, body = getContactNotesPage(t, router, contact.ID, "sort=date&cursor="+EncodeCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, `"reason_code":"validation_cursor_invalid"`)
	code, _, body = getContactNotesPage(t, router, contact.ID, "cursor="+EncodeDateCursor(time.Now(), uint(1)))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, `"reason_code":"validation_cursor_invalid"`)

	code, _, body = getContactNotesPage(t, router, contact.ID, "sort=date&cursor="+EncodeDateCursor(time.Now(), "not-a-number"))
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, `"reason_code":"validation_cursor_malformed"`)
}

// Ownership: another user's contact id is the same 404 as an unknown id, under
// sort=date too, and never leaks their notes.
func TestGetNotesForContactSortDateOwnershipIsolation(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/contacts/:id/notes", GetNotesForContact)
	other := models.User{Username: "cnother", Password: "x", Email: "cnother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&theirs).Error)
	require.NoError(t, db.Create(&models.Note{UserID: other.ID, ContactID: &theirs.ID, Content: "secret", Date: time.Now()}).Error)

	code, foreignPage, foreignBody := getContactNotesPage(t, router, theirs.ID, "sort=date")
	assert.Equal(t, http.StatusNotFound, code)
	assert.NotContains(t, foreignBody, "secret")
	assert.Empty(t, foreignPage.Notes, "a foreign contact must never leak notes")
	unknownCode, _, unknownBody := getContactNotesPage(t, router, 999999, "sort=date")
	assert.Equal(t, http.StatusNotFound, unknownCode)

	var a, b struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(foreignBody), &a))
	require.NoError(t, json.Unmarshal([]byte(unknownBody), &b))
	assert.Equal(t, a.Error.Code, b.Error.Code)
	assert.Equal(t, a.Error.Message, b.Error.Message)
	assert.NotEmpty(t, a.Error.Code, "the 404 must carry the error envelope, not an empty body")
}
