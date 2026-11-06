package controllers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2289, ADR 0042 decision 10: deleting a note, activity or circle ends
// the owner's live standing-share grants on it (revoked / entity_deleted)
// through the shared services.RevokeShareGrantsForEntity helper. Contact
// delete is covered by the cascade-registry step in services.

func seedGrantFor(t *testing.T, db *gorm.DB, from uint, etype, eid, status string) (models.ShareGrant, models.User) {
	t.Helper()
	tag := "rcpt-" + etype + "-" + eid
	rcpt := models.User{Username: tag, Password: "password123", Email: tag + "@example.com"}
	require.NoError(t, db.Create(&rcpt).Error)
	g := models.ShareGrant{FromUserID: from, ToUserID: rcpt.ID, EntityType: etype, EntityID: eid, Status: status}
	require.NoError(t, db.Create(&g).Error)
	return g, rcpt
}

func assertGrantRevokedEntityDeleted(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	var got models.ShareGrant
	require.NoError(t, db.First(&got, "id = ?", id).Error)
	assert.Equal(t, models.ShareGrantStatusRevoked, got.Status)
	require.NotNil(t, got.RevokedReason)
	assert.Equal(t, models.ShareGrantReasonEntityDeleted, *got.RevokedReason)
	assert.NotNil(t, got.RevokedAt)
}

func assertGrantStatus(t *testing.T, db *gorm.DB, id, status string) {
	t.Helper()
	var got models.ShareGrant
	require.NoError(t, db.First(&got, "id = ?", id).Error)
	assert.Equal(t, status, got.Status)
}

func TestDeleteNote_RevokesStandingShareGrants(t *testing.T) {
	db, router := setupRouter(t)
	router.DELETE("/notes/:id", DeleteNote)
	var user models.User
	db.First(&user)

	note := models.Note{UserID: user.ID, Content: "shared note"}
	require.NoError(t, db.Create(&note).Error)
	keep := models.Note{UserID: user.ID, Content: "other note"}
	require.NoError(t, db.Create(&keep).Error)
	g, _ := seedGrantFor(t, db, user.ID, models.ShareGrantEntityNote, note.UUID, models.ShareGrantStatusActive)
	other, _ := seedGrantFor(t, db, user.ID, models.ShareGrantEntityNote, keep.UUID, models.ShareGrantStatusActive)
	// Not the owner's grant: a note id string collision across owners must not matter.
	foreignOwner := models.User{Username: "other-owner", Password: "password123", Email: "oo@example.com"}
	require.NoError(t, db.Create(&foreignOwner).Error)
	foreignRcpt := models.User{Username: "other-rcpt", Password: "password123", Email: "or@example.com"}
	require.NoError(t, db.Create(&foreignRcpt).Error)
	foreign := models.ShareGrant{FromUserID: foreignOwner.ID, ToUserID: foreignRcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: note.UUID, Status: models.ShareGrantStatusActive}
	require.NoError(t, db.Create(&foreign).Error)

	req, _ := http.NewRequest("DELETE", "/notes/"+strconv.Itoa(int(note.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assertGrantRevokedEntityDeleted(t, db, g.ID)
	assertGrantStatus(t, db, other.ID, models.ShareGrantStatusActive)
	assertGrantStatus(t, db, foreign.ID, models.ShareGrantStatusActive)
}

func TestDeleteActivity_RevokesStandingShareGrants(t *testing.T) {
	db, router := setupRouter(t)
	router.DELETE("/activities/:id", DeleteActivity)
	var user models.User
	db.First(&user)

	act := models.Activity{UserID: user.ID, Title: "shared activity"}
	require.NoError(t, db.Create(&act).Error)
	require.NotEmpty(t, act.UUID)
	g, _ := seedGrantFor(t, db, user.ID, models.ShareGrantEntityActivity, act.UUID, models.ShareGrantStatusPending)

	req, _ := http.NewRequest("DELETE", "/activities/"+strconv.Itoa(int(act.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assertGrantRevokedEntityDeleted(t, db, g.ID)
}

func TestDeleteCircle_RevokesStandingShareGrants(t *testing.T) {
	db, router := setupRouter(t)
	router.DELETE("/circles/:id", DeleteCircle)
	var user models.User
	db.First(&user)

	circle := models.Circle{UserID: user.ID, Name: "Shared circle"}
	require.NoError(t, db.Create(&circle).Error)
	g, _ := seedGrantFor(t, db, user.ID, models.ShareGrantEntityCircle, circle.ID, models.ShareGrantStatusActive)

	req, _ := http.NewRequest("DELETE", "/circles/"+circle.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assertGrantRevokedEntityDeleted(t, db, g.ID)
}

// A failed delete must not revoke: the grant revoke and the entity delete share
// one transaction.
func TestDeleteNote_GrantRevokeRollsBackWithFailedDelete(t *testing.T) {
	db, router := setupRouter(t)
	router.DELETE("/notes/:id", DeleteNote)
	var user models.User
	db.First(&user)

	note := models.Note{UserID: user.ID, Content: "n"}
	require.NoError(t, db.Create(&note).Error)
	g, _ := seedGrantFor(t, db, user.ID, models.ShareGrantEntityNote, note.UUID, models.ShareGrantStatusActive)

	// Another user's note id: 404, no revoke.
	req, _ := http.NewRequest("DELETE", "/notes/"+strconv.Itoa(int(note.ID)+1000), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertGrantStatus(t, db, g.ID, models.ShareGrantStatusActive)
}
