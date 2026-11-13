package controllers

import (
	"net/http"
	"strconv"
	"testing"

	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2562: deleting a contact soft-deletes its notes, so a standing-share
// grant on one of those notes must end with it -- and a trash restore must not
// make the recipient's content readable again (ADR 0042 decision 10).

func (e *sgEnv) contactNoteGrant() (models.Contact, models.Note, string) {
	c := e.contact(e.A, "Noted")
	n := models.Note{UserID: e.A.ID, ContactID: &c.ID, Content: "secret body", Date: e.clock.Now()}
	require.NoError(e.t, e.db.Create(&n).Error)
	id := e.grant(e.A, e.B, "note", n.UUID, nil)
	require.Equal(e.t, http.StatusOK, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	require.Equal(e.t, http.StatusOK, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code, "precondition: readable")
	return c, n, id
}

func TestContactDelete_RevokesNoteGrantAndRestoreDoesNotRearmIt(t *testing.T) {
	e := newSGEnv(t)
	e.router.DELETE("/contacts/:id", DeleteContact)
	e.router.POST("/trash/contacts/:id/restore", RestoreTrashedContact)
	c, _, id := e.contactNoteGrant()

	require.Equal(t, http.StatusOK, e.do(e.A, http.MethodDelete, "/contacts/"+strconv.Itoa(int(c.ID)), nil).Code)
	assert.Equal(t, "revoked", e.status(id))
	assert.Equal(t, http.StatusNotFound, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code)

	w := e.do(e.A, http.MethodPost, "/trash/contacts/"+strconv.Itoa(int(c.ID))+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), id, "restore lists the revoked note grant")
	assert.Equal(t, "revoked", e.status(id))
	assert.Equal(t, http.StatusNotFound, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code, "restore must not re-arm the recipient's access")
}

func TestBulkContactDelete_RevokesNoteGrantAndRestoreDoesNotRearmIt(t *testing.T) {
	e := newSGEnv(t)
	e.router.POST("/contacts/bulk", middleware.ValidateJSONMiddleware(&models.BulkContactOperationInput{}), BulkContactOperation)
	e.router.POST("/trash/contacts/:id/restore", RestoreTrashedContact)
	c, _, id := e.contactNoteGrant()

	w := e.do(e.A, http.MethodPost, "/contacts/bulk", map[string]any{"action": "delete", "vcard_uids": []string{c.VCardUID}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "revoked", e.status(id))

	require.Equal(t, http.StatusOK, e.do(e.A, http.MethodPost, "/trash/contacts/"+strconv.Itoa(int(c.ID))+"/restore", nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code)
}
