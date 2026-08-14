package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContactNotes(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id/notes", GetNotesForContact)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "John",
		Lastname:  "Doe",
	}
	db.Create(&contact)

	// Create some notes for the contact
	note1 := models.Note{UserID: user.ID, Content: "First note", Date: time.Now(), ContactID: &contact.ID}
	note2 := models.Note{UserID: user.ID, Content: "Second note", Date: time.Now(), ContactID: &contact.ID}
	db.Create(&note1)
	db.Create(&note2)

	// Make the request to get notes for the contact
	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody struct {
		Notes      []models.Note `json:"notes"`
		NextCursor string        `json:"next_cursor"`
		Limit      int           `json:"limit"`
	}
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	// Assert that the notes returned belong to the contact. Both notes are
	// within the default page, so ordering is the T17 (updated_at, id) DESC
	// feed order — the newest-created first.
	assert.Len(t, responseBody.Notes, 2)
	assert.Equal(t, note2.Content, responseBody.Notes[0].Content)
	assert.Equal(t, note1.Content, responseBody.Notes[1].Content)
	// M19: cursor envelope, no total/page (a contact's note history
	// accumulates without bound).
	assert.Empty(t, responseBody.NextCursor, "two notes fit in the default page, so no cursor")
	assert.Equal(t, 25, responseBody.Limit)
}

func TestGetContactNotesSearchAndDateFilter(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id/notes", GetNotesForContact)

	contact := models.Contact{UserID: user.ID, Firstname: "John", Lastname: "Doe"}
	db.Create(&contact)

	mkNote := func(content string, daysAgo int) models.Note {
		n := models.Note{
			UserID:    user.ID,
			Content:   content,
			Date:      time.Now().AddDate(0, 0, -daysAgo),
			ContactID: &contact.ID,
		}
		db.Create(&n)
		return n
	}
	mkNote("Loves climbing", 1)
	mkNote("Met at conference", 3)
	mkNote("Birthday party", 10)

	// Search narrows by content (case-insensitive).
	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes?search=CLIMBING", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	notes := body["notes"].([]any)
	assert.Len(t, notes, 1)
	assert.Contains(t, notes[0].(map[string]any)["content"], "climbing")

	// fromDate/toDate filter on the note date, inclusive both ends.
	from := time.Now().AddDate(0, 0, -4).Format("2006-01-02")
	to := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	req2, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes?fromDate="+from+"&toDate="+to, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	json.Unmarshal(w2.Body.Bytes(), &body)
	assert.Len(t, body["notes"].([]any), 2, "the 1- and 3-day-old notes fall inside the window")
}

func TestGetContactNotesPagination(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id/notes", GetNotesForContact)

	contact := models.Contact{UserID: user.ID, Firstname: "John", Lastname: "Doe"}
	db.Create(&contact)

	// Distinct updated_at values keep the cursor walk deterministic: give
	// each note an explicit offset so page boundaries can't land on ties.
	base := time.Now().Add(-time.Hour)
	var created []models.Note
	for i, content := range []string{"One", "Two", "Three"} {
		n := models.Note{UserID: user.ID, Content: content, Date: time.Now(), ContactID: &contact.ID}
		require.NoError(t, db.Create(&n).Error)
		require.NoError(t, db.Model(&n).Update("updated_at", base.Add(time.Duration(i)*time.Minute)).Error)
		created = append(created, n)
	}

	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes?limit=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var page1 struct {
		Notes      []models.Note `json:"notes"`
		NextCursor string        `json:"next_cursor"`
	}
	json.Unmarshal(w.Body.Bytes(), &page1)
	require.Len(t, page1.Notes, 2)
	require.NotEmpty(t, page1.NextCursor, "a full page must carry a next_cursor")

	// Follow the cursor to the last page.
	req2, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes?limit=2&cursor="+page1.NextCursor, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var page2 struct {
		Notes      []models.Note `json:"notes"`
		NextCursor string        `json:"next_cursor"`
	}
	json.Unmarshal(w2.Body.Bytes(), &page2)
	require.Len(t, page2.Notes, 1)
	assert.Empty(t, page2.NextCursor, "no more rows after the last page")

	// Every note appears exactly once across the walk.
	seen := map[uint]bool{}
	for _, n := range append(page1.Notes, page2.Notes...) {
		seen[n.ID] = true
	}
	require.Len(t, seen, 3)
}

func TestGetContactNotesNotFound(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id/notes", GetNotesForContact)

	req, _ := http.NewRequest("GET", "/contacts/999999/notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetContactNotesMalformedCursor(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id/notes", GetNotesForContact)

	contact := models.Contact{UserID: user.ID, Firstname: "John", Lastname: "Doe"}
	db.Create(&contact)

	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes?cursor=not-a-cursor", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateContactNote(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.POST("/contacts/:id/notes", withValidated(func() any { return &models.NoteInput{} }), CreateNote)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
	}
	db.Create(&contact)

	// Create a note
	now := time.Now()
	newNote := models.NoteInput{
		Content:   "This is a new note.",
		Date:      now,
		ContactID: &contact.ID,
	}

	jsonValue, _ := json.Marshal(newNote)

	req, _ := http.NewRequest("POST", "/contacts/"+strconv.Itoa(int(contact.ID))+"/notes", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Note created successfully", responseBody["message"])
}

func TestGetNote(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/notes/:id", GetNote)

	// Create a note
	note := models.Note{
		UserID:  user.ID,
		Content: "Note for retrieval.",
	}
	db.Create(&note)

	// Make the request to get the note by ID
	req, _ := http.NewRequest("GET", "/notes/"+strconv.Itoa(int(note.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody models.Note
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, note.Content, responseBody.Content)
}

func TestGetNotes(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/notes", GetUnassignedNotes)

	// Create some unassigned notes
	notes := []models.Note{
		{UserID: user.ID, Content: "Unassigned Note 1"},
		{UserID: user.ID, Content: "Unassigned Note 2"},
	}
	db.Create(&notes)

	// Make the request to get unassigned notes
	req, _ := http.NewRequest("GET", "/notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	noteItems, ok := responseBody["notes"].([]any)
	if !ok {
		t.Fatalf("expected notes array in response")
	}
	assert.Len(t, noteItems, 2)
	// T17: the cursor envelope has no offset-style `page`.
	//
	// `total` IS present here, deliberately, and this assertion was relaxed to
	// say so rather than deleted. T17 removed counts because a contact's note
	// history is unbounded; the unfiled set this endpoint returns is a queue
	// the user drains, and N4's inbox chip is a queue depth. See
	// TestGetUnassignedNotes_TotalCountsWholeSetNotPage.
	assert.NotContains(t, responseBody, "page")
	assert.Contains(t, responseBody, "total")
	assert.EqualValues(t, 2, responseBody["total"])
}

func TestGetNotesSearch(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/notes", GetUnassignedNotes)

	db.Create(&models.Note{UserID: user.ID, Content: "Call Alice"})
	db.Create(&models.Note{UserID: user.ID, Content: "Call Bob"})

	req, _ := http.NewRequest("GET", "/notes?search=alice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	noteItems, ok := responseBody["notes"].([]any)
	if !ok {
		t.Fatalf("expected notes array in response")
	}
	assert.Len(t, noteItems, 1)
	// The total tracks the active search filter, so it stays consistent with
	// the list rendered beside it (see TestGetUnassignedNotes_TotalRespectsSearchFilter).
	assert.EqualValues(t, 1, responseBody["total"])
}

func TestCreateNote(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.POST("/notes", withValidated(func() any { return &models.NoteInput{} }), CreateUnassignedNote)

	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Standalone",
		Lastname:  "Owner",
	}
	db.Create(&contact)

	// Create a note
	noteDate := time.Now()
	newNote := models.NoteInput{
		Content:   "This is a standalone note.",
		Date:      noteDate,
		ContactID: &contact.ID,
	}

	jsonValue, _ := json.Marshal(newNote)

	req, _ := http.NewRequest("POST", "/notes", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Note created successfully", responseBody["message"])
}

func TestCreateUnassignedNoteWithoutContactID(t *testing.T) {
	_, router := setupRouter()

	router.POST("/notes", middleware.ValidateJSONMiddleware(&models.NoteInput{}), CreateUnassignedNote)

	noteDate := time.Now()
	newNote := models.NoteInput{
		Content: "This is a floating note without a contact.",
		Date:    noteDate,
	}

	jsonValue, _ := json.Marshal(newNote)

	req, _ := http.NewRequest("POST", "/notes", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Message string      `json:"message"`
		Note    models.Note `json:"note"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "Note created successfully", response.Message)
	assert.Nil(t, response.Note.ContactID)
}

func TestUpdateNote(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)
	router.GET("/notes/:id", GetNote)

	// Seed a contact to satisfy validation and ownership checks.
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Linked",
		Lastname:  "Contact",
	}
	db.Create(&contact)

	// Create a note
	note := models.Note{
		UserID:    user.ID,
		Content:   "Original note content.",
		Date:      time.Now(),
		ContactID: &contact.ID,
	}
	db.Create(&note)

	// Update the note
	updatedNote := models.NoteInput{
		Content:   "Updated note content.",
		Date:      time.Now().Add(time.Hour),
		ContactID: &contact.ID,
	}
	jsonValue, _ := json.Marshal(updatedNote)

	req, _ := http.NewRequest("PUT", "/notes/"+strconv.Itoa(int(note.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Note updated successfully", responseBody["message"])

	// Fetch the note back to verify changes
	fetchReq, _ := http.NewRequest("GET", "/notes/"+strconv.Itoa(int(note.ID)), nil)
	fetchW := httptest.NewRecorder()
	router.ServeHTTP(fetchW, fetchReq)

	var fetchedNote models.Note
	json.Unmarshal(fetchW.Body.Bytes(), &fetchedNote)

	assert.Equal(t, updatedNote.Content, fetchedNote.Content)
}

func TestDeleteNote(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.DELETE("/notes/:id", DeleteNote)

	// Create a note
	note := models.Note{
		UserID:  user.ID,
		Content: "Note to be deleted.",
	}
	db.Create(&note)

	// Make the request to delete the note
	req, _ := http.NewRequest("DELETE", "/notes/"+strconv.Itoa(int(note.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Note deleted", responseBody["message"])
}

// T2: UpdateNote clears contact_id (unassigned). GORM's Save must write nil.
func TestUpdateNoteClearsContactID(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)

	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Original",
		Lastname:  "Holder",
	}
	db.Create(&contact)

	note := models.Note{
		UserID:    user.ID,
		Content:   "Assigned note.",
		Date:      time.Now(),
		ContactID: &contact.ID,
	}
	db.Create(&note)

	// Update with contact_id omitted (nil)
	updatedNote := models.NoteInput{
		Content: "Now unassigned.",
		Date:    time.Now(),
	}
	jsonValue, _ := json.Marshal(updatedNote)
	req, _ := http.NewRequest("PUT", "/notes/"+strconv.Itoa(int(note.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Note
	db.First(&reloaded, note.ID)
	assert.Nil(t, reloaded.ContactID, "contact_id must be nil after clear")
}

// T3: UpdateNote changes contact_id to a different contact.
func TestUpdateNoteChangesContactID(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)

	note := models.Note{
		UserID:    user.ID,
		Content:   "Assignable note.",
		Date:      time.Now(),
		ContactID: &alice.ID,
	}
	db.Create(&note)

	updated := models.NoteInput{
		Content:   note.Content,
		Date:      note.Date,
		ContactID: &bob.ID,
	}
	jsonValue, _ := json.Marshal(updated)
	req, _ := http.NewRequest("PUT", "/notes/"+strconv.Itoa(int(note.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Note
	db.First(&reloaded, note.ID)
	assert.NotNil(t, reloaded.ContactID)
	assert.Equal(t, bob.ID, *reloaded.ContactID)
}

// T4: CreateUnassignedNote with a contact_id (note created already filed).
func TestCreateUnassignedNoteWithContactID(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.POST("/notes", withValidated(func() any { return &models.NoteInput{} }), CreateUnassignedNote)

	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Filing",
	}
	db.Create(&contact)

	input := models.NoteInput{
		Content:   "Born assigned.",
		Date:      time.Now(),
		ContactID: &contact.ID,
	}
	jsonValue, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/notes", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Note models.Note `json:"note"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.NotNil(t, response.Note.ContactID)
	assert.Equal(t, contact.ID, *response.Note.ContactID)
}

// T5: UpdateNote rejects contact_id referencing another user's contact (IDOR).
func TestUpdateNoteRejectsCrossUserContactID(t *testing.T) {
	db, router := setupRouter()

	var owner models.User
	var other models.User
	db.First(&owner)
	db.Create(&other)

	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)

	othersContact := models.Contact{UserID: other.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)

	note := models.Note{
		UserID:  owner.ID,
		Content: "My note.",
		Date:    time.Now(),
	}
	db.Create(&note)

	updated := models.NoteInput{
		Content:   note.Content,
		Date:      note.Date,
		ContactID: &othersContact.ID,
	}
	jsonValue, _ := json.Marshal(updated)
	req, _ := http.NewRequest("PUT", "/notes/"+strconv.Itoa(int(note.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// N4's inbox chip is a queue depth, so GET /notes (unassigned) returns a
// `total`. It reports the whole filtered result set, NOT the page — the
// frontend previously rendered `notes.length`, so a user with more than one
// page of unfiled notes saw an under-count that grew as they paged.
//
// This is a deliberate exception to T17's removal of counts: that decision was
// about a contact's note history, which accumulates without bound. The unfiled
// set is a queue the user drains, and counting it is the point of an inbox.
func TestGetUnassignedNotes_TotalCountsWholeSetNotPage(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notes", GetUnassignedNotes)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	const unfiled = 7
	for i := 0; i < unfiled; i++ {
		require.NoError(t, db.Create(&models.Note{
			UserID: user.ID, Content: fmt.Sprintf("unfiled %d", i),
		}).Error)
	}

	// A page smaller than the result set: `total` must still report all of it.
	req, _ := http.NewRequest("GET", "/notes?limit=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Notes      []models.Note `json:"notes"`
		Total      int64         `json:"total"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Len(t, resp.Notes, 3, "the page is limited")
	assert.NotEmpty(t, resp.NextCursor, "more pages remain")
	assert.Equal(t, int64(unfiled), resp.Total,
		"total must count the whole unfiled set, not the returned page")
}

// A note that gains a contact leaves the inbox, and the total must follow it
// down — otherwise the queue depth never decreases as the user files.
func TestGetUnassignedNotes_TotalExcludesFiledNotes(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notes", GetUnassignedNotes)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Filed"}
	require.NoError(t, db.Create(&contact).Error)

	unfiled := models.Note{UserID: user.ID, Content: "still unfiled"}
	require.NoError(t, db.Create(&unfiled).Error)
	filed := models.Note{UserID: user.ID, Content: "already filed", ContactID: &contact.ID}
	require.NoError(t, db.Create(&filed).Error)

	req, _ := http.NewRequest("GET", "/notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Notes []models.Note `json:"notes"`
		Total int64         `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, int64(1), resp.Total, "a filed note must not count toward the inbox depth")
	require.Len(t, resp.Notes, 1)
	assert.Equal(t, "still unfiled", resp.Notes[0].Content)
}

// The count is computed on the filtered query, so it stays consistent with the
// list rendered next to it rather than contradicting a visible search filter.
func TestGetUnassignedNotes_TotalRespectsSearchFilter(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notes", GetUnassignedNotes)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "buy oranges"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "buy apples"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "call the plumber"}).Error)

	req, _ := http.NewRequest("GET", "/notes?search=buy", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(2), resp.Total, "total must reflect the active search filter")
}

// Another user's unfiled notes must never inflate this user's queue depth.
func TestGetUnassignedNotes_TotalScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notes", GetUnassignedNotes)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	other := models.User{Username: "other-notes", Email: "other-notes@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)

	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "mine"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: other.ID, Content: "theirs"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: other.ID, Content: "theirs too"}).Error)

	req, _ := http.NewRequest("GET", "/notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Total)
}
