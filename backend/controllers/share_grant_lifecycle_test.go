package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2291, ADR 0042 decisions 10-11: in-app badge/notices, recipient
// deletion audit and the trash-restore response.

type noticesBody struct {
	PendingCount  int `json:"pending_count"`
	Notifications []struct {
		ID             uint   `json:"id"`
		Kind           string `json:"kind"`
		EntityType     string `json:"entity_type"`
		SenderUsername string `json:"sender_username"`
	} `json:"notifications"`
}

func TestShareGrantNotifications_BadgeAndNoticesAreTheRecipientsOwn(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "private text")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)

	w := e.do(e.B, http.MethodGet, "/share-grants/notifications", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var body noticesBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 1, body.PendingCount)
	require.Len(t, body.Notifications, 1)
	assert.Equal(t, "pending", body.Notifications[0].Kind)
	assert.Equal(t, "sg_owner", body.Notifications[0].SenderUsername)
	assert.Equal(t, "note", body.Notifications[0].EntityType)
	assert.NotContains(t, w.Body.String(), "private text", "payload is sender username + entity type only")
	assert.NotContains(t, w.Body.String(), n.UUID)

	// Other users see nothing of it.
	for _, u := range []models.User{e.A, e.C, e.Admin} {
		var other noticesBody
		ow := e.do(u, http.MethodGet, "/share-grants/notifications", nil)
		require.Equal(t, http.StatusOK, ow.Code)
		require.NoError(t, json.Unmarshal(ow.Body.Bytes(), &other))
		assert.Zero(t, other.PendingCount)
		assert.Empty(t, other.Notifications)
	}

	// Raw JSON carries [] not an absent key (frontend trap #8).
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(e.do(e.C, http.MethodGet, "/share-grants/notifications", nil).Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["notifications"]))

	// Revoke raises a revoked notice; marking read clears only the caller's.
	require.Equal(t, http.StatusOK, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/revoke", nil).Code)
	require.NoError(t, json.Unmarshal(e.do(e.B, http.MethodGet, "/share-grants/notifications", nil).Body.Bytes(), &body))
	assert.Len(t, body.Notifications, 2)
	assert.Zero(t, body.PendingCount)

	mw := e.do(e.C, http.MethodPost, "/share-grants/notifications/read", nil)
	require.Equal(t, http.StatusOK, mw.Code)
	assert.JSONEq(t, `{"marked":0}`, mw.Body.String())
	mw = e.do(e.B, http.MethodPost, "/share-grants/notifications/read", nil)
	require.Equal(t, http.StatusOK, mw.Code)
	assert.JSONEq(t, `{"marked":2}`, mw.Body.String())
	require.NoError(t, json.Unmarshal(e.do(e.B, http.MethodGet, "/share-grants/notifications", nil).Body.Bytes(), &body))
	assert.Empty(t, body.Notifications)
}

func TestShareGrantNotifications_UnauthenticatedIs401(t *testing.T) {
	e := newSGEnv(t)
	assert.Equal(t, http.StatusUnauthorized, e.do(models.User{}, http.MethodGet, "/share-grants/notifications", nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do(models.User{}, http.MethodPost, "/share-grants/notifications/read", nil).Code)
}

func TestShareGrantNotifications_DBFailureDoesNotLeakSQL(t *testing.T) {
	e := newSGEnv(t)
	require.NoError(t, e.db.Exec("DROP TABLE share_grant_notifications").Error)
	w := e.do(e.B, http.MethodGet, "/share-grants/notifications", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "share_grant_notifications")
	w = e.do(e.B, http.MethodPost, "/share-grants/notifications/read", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "share_grant_notifications")
}

func TestShareGrantNotifications_CountFailureDoesNotLeakSQL(t *testing.T) {
	e := newSGEnv(t)
	require.NoError(t, e.db.Exec("DROP TABLE share_grants").Error)
	w := e.do(e.B, http.MethodGet, "/share-grants/notifications", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "share_grants")
}

// --- recipient deletion ----------------------------------------------------

func ownerRevokeEvents(t *testing.T, db *gorm.DB, owner uint) []models.AuditEvent {
	t.Helper()
	var ev []models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND user_id = ? AND operation = ?", models.AuditEntityShareGrant, owner, models.AuditOpRevoke).Find(&ev).Error)
	return ev
}

func TestDeleteUser_AdminEmitsRecipientDeletedAndHardDeletesGrants(t *testing.T) {
	db, router := setupRouter(t)
	var admin models.User
	require.NoError(t, db.First(&admin).Error)
	require.NoError(t, db.Model(&admin).Update("is_admin", true).Error)
	models.NewAuditRecorder(db, models.WithSync())
	owner := models.User{Username: "rd-owner", Email: "rd-owner@example.com", Password: "password123"}
	rcpt := models.User{Username: "rd-rcpt", Email: "rd-rcpt@example.com", Password: "password123"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&rcpt).Error)
	live := models.ShareGrant{FromUserID: owner.ID, ToUserID: rcpt.ID, EntityType: "note", EntityID: "n1", Status: models.ShareGrantStatusActive}
	sent := models.ShareGrant{FromUserID: rcpt.ID, ToUserID: owner.ID, EntityType: "note", EntityID: "n2", Status: models.ShareGrantStatusActive}
	require.NoError(t, db.Create(&live).Error)
	require.NoError(t, db.Create(&sent).Error)
	router.DELETE("/users/:id", DeleteUser)

	req, _ := http.NewRequest(http.MethodDelete, "/users/"+strconv.Itoa(int(rcpt.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var n int64
	require.NoError(t, db.Model(&models.ShareGrant{}).Where("from_user_id = ? OR to_user_id = ?", rcpt.ID, rcpt.ID).Count(&n).Error)
	assert.Zero(t, n, "outgoing and incoming grants are hard-deleted")
	ev := ownerRevokeEvents(t, db, owner.ID)
	require.Len(t, ev, 1, "only the incoming grant is a recipient_deleted event")
	assert.Equal(t, live.ID, ev[0].EntityID)
	assert.Contains(t, ev[0].BeforeSnapshot, `"revoked_reason":"recipient_deleted"`)
}

func TestDeleteOwnAccount_EmitsRecipientDeletedForTheOwner(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	models.NewAuditRecorder(db, models.WithSync())
	owner := seedDeleteAccountUser(t, db, "rdowner2", false)
	rcpt := seedDeleteAccountUser(t, db, "rdrcpt2", false)
	g := models.ShareGrant{FromUserID: owner.ID, ToUserID: rcpt.ID, EntityType: "note", EntityID: "n1", Status: models.ShareGrantStatusPending}
	require.NoError(t, db.Create(&g).Error)
	token, err := services.IssueSession(db, rcpt, cfg, "", "")
	require.NoError(t, err)

	w, _ := doRequest(router, deleteAccountRequest(token, map[string]string{"current_password": strongPassword}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var n int64
	require.NoError(t, db.Model(&models.ShareGrant{}).Where("id = ?", g.ID).Count(&n).Error)
	assert.Zero(t, n)
	ev := ownerRevokeEvents(t, db, owner.ID)
	require.Len(t, ev, 1)
	assert.Contains(t, ev[0].BeforeSnapshot, `"revoked_reason":"recipient_deleted"`)
}

// --- trash restore ----------------------------------------------------------

func TestTrashRestore_ListsGrantsRevokedByTheDeleteAndDoesNotRearmThem(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	rcpt := models.User{Username: "tr-rcpt", Email: "tr-rcpt@example.com", Password: "password123"}
	require.NoError(t, db.Create(&rcpt).Error)
	c := models.Contact{UserID: uid, Firstname: "Ada"}
	require.NoError(t, db.Create(&c).Error)
	live := models.ShareGrant{FromUserID: uid, ToUserID: rcpt.ID, EntityType: "contact", EntityID: c.VCardUID,
		Sections: []string{"emails"}, Status: models.ShareGrantStatusActive}
	require.NoError(t, db.Create(&live).Error)
	// An unrelated, older entity_deleted revoke for the same contact must not be listed.
	old := models.ShareGrant{FromUserID: uid, ToUserID: rcpt.ID, EntityType: "contact", EntityID: c.VCardUID,
		Status: models.ShareGrantStatusRevoked, RevokedReason: ptr(models.ShareGrantReasonEntityDeleted)}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Model(&old).UpdateColumn("revoked_at", "2020-01-01 00:00:00+00:00").Error)

	w := trashDo(router, http.MethodDelete, fmt.Sprintf("/contacts/%d", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		RevokedGrants []struct {
			ID         string   `json:"id"`
			ToUserID   uint     `json:"to_user_id"`
			ToUsername string   `json:"to_username"`
			Sections   []string `json:"sections"`
		} `json:"revoked_grants"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.RevokedGrants, 1)
	assert.Equal(t, live.ID, resp.RevokedGrants[0].ID)
	assert.Equal(t, rcpt.ID, resp.RevokedGrants[0].ToUserID)
	assert.Equal(t, "tr-rcpt", resp.RevokedGrants[0].ToUsername)
	assert.Equal(t, []string{"emails"}, resp.RevokedGrants[0].Sections)

	var g models.ShareGrant
	require.NoError(t, db.First(&g, "id = ?", live.ID).Error)
	assert.Equal(t, models.ShareGrantStatusRevoked, g.Status, "restore never re-arms access")
}

func TestTrashRestore_NoGrantsIsAnEmptyArrayNotAbsent(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	c := trashContact(t, db, router, uid, "Solo")
	w := trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["revoked_grants"]))
}

func ptr[T any](v T) *T { return &v }
