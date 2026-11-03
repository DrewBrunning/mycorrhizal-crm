package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Image-only notes (ADR 0040 decision 11, issue #2004): empty content is valid
// only with at least one live image, created in two steps via awaiting_images.

func imageOnlyEnv(t *testing.T) noteImageTestEnv {
	t.Helper()
	env := setupNoteImageRouter(t)
	env.router.POST("/notes", middleware.ValidateJSONMiddleware(&models.NoteInput{}), CreateUnassignedNote)
	env.router.PUT("/notes/:id", middleware.ValidateJSONMiddleware(&models.NoteInput{}), UpdateNote)
	return env
}

func noteJSON(t *testing.T, env noteImageTestEnv, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, _ := http.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func reasonOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error.ReasonCode
}

func TestCreateNote_EmptyContentNeedsAwaitingImages(t *testing.T) {
	env := imageOnlyEnv(t)
	date := "2024-01-01T00:00:00Z"

	rec := noteJSON(t, env, "POST", "/notes", map[string]any{"content": "", "date": date})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, "note_requires_content_or_image", reasonOf(t, rec))

	rec = noteJSON(t, env, "POST", "/notes", map[string]any{"content": "", "date": date, "awaiting_images": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var count int64
	require.NoError(t, env.db.Model(&models.Note{}).Where("content = ''").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestUpdateNote_EmptyContentNeedsLiveImage(t *testing.T) {
	env := imageOnlyEnv(t)
	note := models.Note{UserID: env.user.ID, Content: "text", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	path := "/notes/" + strconv.FormatUint(uint64(note.ID), 10)
	body := map[string]any{"content": "", "date": "2024-01-01T00:00:00Z"}

	rec := noteJSON(t, env, "PUT", path, body)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, "note_requires_content_or_image", reasonOf(t, rec))

	require.Equal(t, http.StatusCreated, uploadNoteImageForTest(t, env, note.ID, testJPEG(t)).Code)
	rec = noteJSON(t, env, "PUT", path, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestDeleteNoteImage_LastImageOfEmptyNoteRefused(t *testing.T) {
	env := imageOnlyEnv(t)
	note := models.Note{UserID: env.user.ID, Content: "", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	first := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := uploadNoteImageForTest(t, env, note.ID, testJPEGVariant(t, 7))
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	var f, s struct {
		Image NoteImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &f))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &s))
	del := func(id uint) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(id), 10), nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		return rec
	}

	require.Equal(t, http.StatusOK, del(f.Image.ID).Code, "one image remains, delete is fine")
	rec := del(s.Image.ID)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "note_requires_content_or_image", reasonOf(t, rec))
	var live int64
	require.NoError(t, env.db.Model(&models.NoteImage{}).Where("note_id = ?", note.ID).Count(&live).Error)
	assert.EqualValues(t, 1, live, "the refused delete leaves the image")
}

func TestDeleteNoteImage_LastImageOfNoteWithContentAllowed(t *testing.T) {
	env := imageOnlyEnv(t)
	note := models.Note{UserID: env.user.ID, Content: "has text", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	up := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, up.Code)
	var u struct {
		Image NoteImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &u))
	req, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(u.Image.ID), 10), nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestCreateContactNote_EmptyContentNeedsAwaitingImages(t *testing.T) {
	env := imageOnlyEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "C"}
	require.NoError(t, env.db.Create(&contact).Error)
	env.router.POST("/contacts/:id/notes", middleware.ValidateJSONMiddleware(&models.NoteInput{}), CreateNote)
	path := "/contacts/" + strconv.FormatUint(uint64(contact.ID), 10) + "/notes"

	rec := noteJSON(t, env, "POST", path, map[string]any{"content": "  ", "date": "2024-01-01T00:00:00Z"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, "note_requires_content_or_image", reasonOf(t, rec))

	rec = noteJSON(t, env, "POST", path, map[string]any{"content": "", "date": "2024-01-01T00:00:00Z", "awaiting_images": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestUpdateNote_ImageCountFailureIs500(t *testing.T) {
	env := imageOnlyEnv(t)
	note := models.Note{UserID: env.user.ID, Content: "text", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	dbtest.HideTable(t, env.db, "note_images")
	rec := noteJSON(t, env, "PUT", "/notes/"+strconv.FormatUint(uint64(note.ID), 10), map[string]any{"content": "", "date": "2024-01-01T00:00:00Z"})
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}
