package controllers

import (
	"net/http"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2742 (ADR 0042 amendment): DELETE /share-grants/{id} is the owner's
// dismiss of an ended (declined, expired, revoked) grant from "Shared by me".
// It hard-deletes the sender's row, is scoped by from_user_id, and answers the
// one share_grant_not_found 404 for every other caller and for an unknown or
// already-dismissed id. A live grant is a 409 share_grant_not_ended.

func (e *sgEnv) rowExists(id string) bool {
	var n int64
	require.NoError(e.t, e.db.Model(&models.ShareGrant{}).Where("id = ?", id).Count(&n).Error)
	return n == 1
}

// endedGrants returns one grant per ended status, created through the API and
// ended by the real transitions (declined: recipient leaves; revoked: owner
// revokes; expired: the lifecycle job's write, set directly).
func (e *sgEnv) endedGrants() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	mk := func() string {
		n := e.note(e.A, "n")
		return e.grant(e.A, e.B, "note", n.UUID, nil)
	}
	d := mk()
	require.Equal(e.t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+d+"/accept", nil).Code)
	require.Equal(e.t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+d+"/leave", nil).Code)
	out[models.ShareGrantStatusDeclined] = d
	r := mk()
	require.Equal(e.t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+r+"/revoke", nil).Code)
	out[models.ShareGrantStatusRevoked] = r
	x := mk()
	require.NoError(e.t, e.db.Model(&models.ShareGrant{}).Where("id = ?", x).Update("status", models.ShareGrantStatusExpired).Error)
	out[models.ShareGrantStatusExpired] = x
	return out
}

func TestShareGrantDelete_OwnerDismissesEndedGrants(t *testing.T) {
	e := newSGEnv(t)
	ended := e.endedGrants()
	liveNote := e.note(e.A, "live")
	live := e.grant(e.A, e.B, "note", liveNote.UUID, nil)

	for status, id := range ended {
		w := e.do(e.A, http.MethodDelete, "/share-grants/"+id, nil)
		assert.Equal(t, http.StatusNoContent, w.Code, status+": "+w.Body.String())
		assert.Empty(t, w.Body.String(), status)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.False(t, e.rowExists(id), status+": the row is hard-deleted")
	}
	assert.True(t, e.rowExists(live), "a live grant is untouched")
	assert.NotContains(t, e.do(e.A, http.MethodGet, "/share-grants/outgoing", nil).Body.String(), ended[models.ShareGrantStatusDeclined])
	assert.Contains(t, e.do(e.A, http.MethodGet, "/share-grants/outgoing", nil).Body.String(), live)
	assert.Equal(t, 200, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Code, "recipient views are unaffected")
}

func TestShareGrantDelete_WritesOwnerAuditEventOnly(t *testing.T) {
	e := newSGEnv(t)
	id := e.endedGrants()[models.ShareGrantStatusRevoked]
	var before int64
	require.NoError(t, e.db.Model(&models.AuditEvent{}).Where("entity_type = ? AND entity_id = ? AND operation = ?", models.AuditEntityShareGrant, id, models.AuditOpDelete).Count(&before).Error)
	require.Zero(t, before)
	require.Equal(t, 204, e.do(e.A, http.MethodDelete, "/share-grants/"+id, nil).Code)
	var evs []models.AuditEvent
	require.NoError(t, e.db.Where("entity_type = ? AND entity_id = ? AND operation = ?", models.AuditEntityShareGrant, id, models.AuditOpDelete).Find(&evs).Error)
	require.Len(t, evs, 1)
	assert.Equal(t, e.A.ID, evs[0].UserID, "only the owner's trail; the recipient is never told")
	assert.NotContains(t, evs[0].BeforeSnapshot, "label")
}

func TestShareGrantDelete_LiveGrantIsNotEndedConflict(t *testing.T) {
	e := newSGEnv(t)
	pending := e.grant(e.A, e.B, "note", e.note(e.A, "p").UUID, nil)
	active := e.grant(e.A, e.B, "note", e.note(e.A, "a").UUID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+active+"/accept", nil).Code)
	// A grant made after its recipient blocked the sender is stored `blocked`
	// and reads as pending to the sender, so it is not dismissable either (and
	// the sender learns nothing).
	require.NoError(t, e.db.Create(&models.ShareBlock{BlockerUserID: e.B.ID, BlockedUserID: e.C.ID}).Error)
	blocked := e.grant(e.C, e.B, "note", e.note(e.C, "b").UUID, nil)
	require.Equal(t, models.ShareGrantStatusBlocked, e.status(blocked))

	for name, c := range map[string]struct {
		who models.User
		id  string
	}{"pending": {e.A, pending}, "active": {e.A, active}, "blocked": {e.C, blocked}} {
		w := e.do(c.who, http.MethodDelete, "/share-grants/"+c.id, nil)
		assert.Equal(t, http.StatusConflict, w.Code, name+": "+w.Body.String())
		assert.Contains(t, w.Body.String(), `"reason_code":"share_grant_not_ended"`, name)
		assert.True(t, e.rowExists(c.id), name+": nothing deleted")
	}
}

func TestShareGrantDelete_EffectivelyExpiredLiveGrantIsDismissable(t *testing.T) {
	e := newSGEnv(t)
	// The owner's list already shows these as expired before the job moves the
	// row, so Remove must work on what the owner sees.
	pending := e.grant(e.A, e.B, "note", e.note(e.A, "p").UUID, nil)
	e.clock.Advance(31 * 24 * time.Hour)
	require.Equal(t, "pending", e.status(pending))
	assert.Equal(t, 204, e.do(e.A, http.MethodDelete, "/share-grants/"+pending, nil).Code)
	assert.False(t, e.rowExists(pending))
}

func TestShareGrantDelete_NonOwnerAndUnknownAreIdentical404(t *testing.T) {
	e := newSGEnv(t)
	ended := e.endedGrants()
	absent := e.do(e.A, http.MethodDelete, "/share-grants/00000000-0000-4000-8000-000000000000", nil)
	require.Equal(t, http.StatusNotFound, absent.Code)
	want := noTimestamp(absent.Body.String())
	assert.Contains(t, want, "share_grant_not_found")

	for status, id := range ended {
		for _, who := range []models.User{e.B, e.C, e.Admin} {
			w := e.do(who, http.MethodDelete, "/share-grants/"+id, nil)
			assert.Equal(t, http.StatusNotFound, w.Code, "%s as user %d", status, who.ID)
			assert.Equal(t, want, noTimestamp(w.Body.String()), "%s as user %d", status, who.ID)
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			assert.True(t, e.rowExists(id), "%s as user %d: no side effect", status, who.ID)
		}
		assert.Equal(t, http.StatusUnauthorized, e.do(models.User{}, http.MethodDelete, "/share-grants/"+id, nil).Code)
	}
	// A live grant that is not yours is also the 404 (not the 409 that would reveal its state).
	live := e.grant(e.A, e.B, "note", e.note(e.A, "l").UUID, nil)
	w := e.do(e.C, http.MethodDelete, "/share-grants/"+live, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, want, noTimestamp(w.Body.String()))
}

func TestShareGrantDelete_RepeatIsTheSame404AndHarmless(t *testing.T) {
	e := newSGEnv(t)
	ended := e.endedGrants()
	id := ended[models.ShareGrantStatusDeclined]
	other := ended[models.ShareGrantStatusRevoked]
	require.Equal(t, 204, e.do(e.A, http.MethodDelete, "/share-grants/"+id, nil).Code)
	w := e.do(e.A, http.MethodDelete, "/share-grants/"+id, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "a dismissed id is an unknown id")
	assert.Contains(t, w.Body.String(), "share_grant_not_found")
	assert.True(t, e.rowExists(other), "the repeat touched nothing else")
}

func TestShareGrantDelete_OwnerScopeIsInTheQuery(t *testing.T) {
	// Another user's ended grant on the same entity id string is never touched
	// by the owner's dismiss, and cannot be dismissed by a non-owner.
	e := newSGEnv(t)
	n := e.note(e.A, "n")
	mine := e.grant(e.A, e.B, "note", n.UUID, nil)
	require.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+mine+"/revoke", nil).Code)
	theirs := e.grant(e.C, e.B, "note", e.note(e.C, "n").UUID, nil)
	require.Equal(t, 200, e.do(e.C, http.MethodPost, "/share-grants/"+theirs+"/revoke", nil).Code)

	assert.Equal(t, 404, e.do(e.A, http.MethodDelete, "/share-grants/"+theirs, nil).Code)
	assert.True(t, e.rowExists(theirs))
	assert.Equal(t, 204, e.do(e.A, http.MethodDelete, "/share-grants/"+mine, nil).Code)
	assert.True(t, e.rowExists(theirs))
}
