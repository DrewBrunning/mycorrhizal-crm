package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2294 (ADR 0042 decision 8): a share_blocks row also stops one-time
// ContactShares from the blocked sender. The sender's view must be identical
// to an unblocked pending share.

func blockSender(t *testing.T, db *gorm.DB, blockerID, blockedID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.ShareBlock{BlockerUserID: blockerID, BlockedUserID: blockedID, CreatedAt: time.Now()}).Error)
}

func createShareBody(t *testing.T, db *gorm.DB, cfg *config.Config, fromID, toID uint, firstname string) (int, map[string]any) {
	t.Helper()
	contact := models.Contact{UserID: fromID, Firstname: firstname, Lastname: "Anderson", Email: "blk@example.com"}
	require.NoError(t, db.Create(&contact).Error)
	w := doJSON(t, contactShareRouterFor(db, cfg, fromID), "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: toID, VCardUID: contact.VCardUID, Sections: []string{models.SectionEmails},
	})
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}

func sortedKeys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

func TestCreateContactShare_BlockedSenderIndistinguishableFromUnblocked(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)

	// Unblocked baseline: sender -> third.
	okCode, okBody := createShareBody(t, db, cfg, sender.ID, third.ID, "Baseline")
	// Blocked: third's block does not apply; recipient blocks sender.
	blockSender(t, db, recipient.ID, sender.ID)
	blkCode, blkBody := createShareBody(t, db, cfg, sender.ID, recipient.ID, "Hidden")

	assert.Equal(t, okCode, blkCode, "status code must match")
	assert.Equal(t, http.StatusOK, blkCode)
	assert.Equal(t, sortedKeys(okBody), sortedKeys(blkBody))
	assert.Equal(t, okBody["message"], blkBody["message"])
	okShare := okBody["contact_share"].(map[string]any)
	blkShare := blkBody["contact_share"].(map[string]any)
	assert.Equal(t, sortedKeys(okShare), sortedKeys(blkShare))
	assert.Equal(t, "pending", blkShare["status"], "sender must see pending")
	assert.Equal(t, "Hidden Anderson", blkShare["contact_display_name"])

	// Outgoing list shows it as pending, like any pending share.
	w := doJSON(t, contactShareRouterFor(db, cfg, sender.ID), "GET", "/contact-shares/outgoing", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		ContactShares []map[string]any `json:"contact_shares"`
		Total         int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, 2, out.Total)
	for _, s := range out.ContactShares {
		assert.Equal(t, "pending", s["status"])
	}

	// Stored internally as blocked, without the content snapshot.
	var row models.ContactShare
	require.NoError(t, db.First(&row, "id = ?", blkShare["id"]).Error)
	assert.Equal(t, models.ContactShareStatusBlocked, row.Status)
	assert.Equal(t, "[]", row.Payload, "a blocked share retains no contact content")

	// No audit row on either party's trail for the blocked create.
	var audits int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Where("entity_type = ? AND entity_id = ?", models.AuditEntityContactShare, row.ID).Count(&audits).Error)
	assert.Zero(t, audits)
}

func TestContactShare_BlockedShareInvisibleToRecipient(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	blockSender(t, db, recipient.ID, sender.ID)
	_, body := createShareBody(t, db, cfg, sender.ID, recipient.ID, "Hidden")
	id := body["contact_share"].(map[string]any)["id"].(string)

	rr := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, rr, "GET", "/contact-shares/incoming", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var in struct {
		ContactShares []map[string]any `json:"contact_shares"`
		Total         int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &in))
	assert.Empty(t, in.ContactShares)
	assert.Zero(t, in.Total)

	for _, p := range []string{"accept", "decline"} {
		w = doJSON(t, rr, "POST", "/contact-shares/"+id+"/"+p, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, p)
	}
	w = doJSON(t, rr, "POST", "/contact-shares/"+id+"/confirm", map[string]any{"session_id": "x", "row_actions": []any{}})
	assert.Equal(t, http.StatusNotFound, w.Code)

	var row models.ContactShare
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	assert.Equal(t, models.ContactShareStatusBlocked, row.Status, "recipient requests must not change it")
}

func TestContactShare_BlockOnlyAffectsTheBlockedPair(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)
	blockSender(t, db, recipient.ID, sender.ID)

	// Same sender to someone else, and a different sender to the blocker, deliver.
	_, _ = createShareBody(t, db, cfg, sender.ID, third.ID, "ToThird")
	_, _ = createShareBody(t, db, cfg, third.ID, recipient.ID, "FromThird")
	var n int64
	require.NoError(t, db.Model(&models.ContactShare{}).Where("status = ?", models.ContactShareStatusPending).Count(&n).Error)
	assert.Equal(t, int64(2), n)

	// The block is directional: the blocker can still share to the blocked user.
	_, _ = createShareBody(t, db, cfg, recipient.ID, sender.ID, "Reverse")
	require.NoError(t, db.Model(&models.ContactShare{}).Where("from_user_id = ? AND status = ?", recipient.ID, models.ContactShareStatusPending).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

func TestContactShare_UnblockDoesNotRevealBlockedShares(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	blockSender(t, db, recipient.ID, sender.ID)
	_, _ = createShareBody(t, db, cfg, sender.ID, recipient.ID, "Hidden")
	require.NoError(t, db.Delete(&models.ShareBlock{}, "blocker_user_id = ? AND blocked_user_id = ?", recipient.ID, sender.ID).Error)

	w := doJSON(t, contactShareRouterFor(db, cfg, recipient.ID), "GET", "/contact-shares/incoming", nil)
	var in struct {
		ContactShares []map[string]any `json:"contact_shares"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &in))
	assert.Empty(t, in.ContactShares)
}

func TestBlock_DeclinesPendingOneTimeSharesAndAuditsWithoutContent(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)
	pending := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Pend", []string{models.SectionEmails}, false)
	other := createPendingShareFixture(t, db, cfg, sender.ID, third.ID, "Other", []string{models.SectionEmails}, false)
	accepted := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Acc", []string{models.SectionEmails}, false)
	require.NoError(t, db.Model(&models.ContactShare{}).Where("id = ?", accepted.ID).Update("status", models.ContactShareStatusAccepted).Error)

	// A standing grant to trigger decline-with-block.
	g := models.ShareGrant{FromUserID: sender.ID, ToUserID: recipient.ID, EntityType: "contact", EntityID: "x",
		Status: models.ShareGrantStatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Select("ID", "FromUserID", "ToUserID", "EntityType", "EntityID", "Status", "CreatedAt", "UpdatedAt").Create(&g).Error)
	_, appErr := services.DeclineShareGrant(db, recipient.ID, g.ID, true, time.Now())
	require.Nil(t, appErr)

	status := func(id string) string {
		var s models.ContactShare
		require.NoError(t, db.First(&s, "id = ?", id).Error)
		return s.Status
	}
	assert.Equal(t, models.ContactShareStatusDeclined, status(pending.ID))
	assert.Equal(t, models.ContactShareStatusPending, status(other.ID), "other recipients unaffected")
	assert.Equal(t, models.ContactShareStatusAccepted, status(accepted.ID), "terminal shares untouched")

	var ev models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ?", models.AuditEntityContactShare, pending.ID).First(&ev).Error)
	assert.Equal(t, recipient.ID, ev.UserID, "recorded on the blocker's trail only")
	assert.Equal(t, models.AuditOpUpdate, ev.Operation)
	assert.NotContains(t, ev.BeforeSnapshot, "Pend")
	assert.NotContains(t, ev.BeforeSnapshot, "Anderson")
	var snap map[string]any
	require.NoError(t, json.Unmarshal([]byte(ev.BeforeSnapshot), &snap))
	assert.Equal(t, "declined", snap["status"])
	var n int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Where("entity_type = ? AND user_id = ?", models.AuditEntityContactShare, sender.ID).Count(&n).Error)
	assert.Zero(t, n, "nothing on the sender's trail")
}

func TestContactShare_BlockedSharePurgedAtPendingTTL(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	blockSender(t, db, recipient.ID, sender.ID)
	_, body := createShareBody(t, db, cfg, sender.ID, recipient.ID, "Hidden")
	id := body["contact_share"].(map[string]any)["id"].(string)
	old := time.Now().AddDate(0, 0, -400)
	require.NoError(t, db.Model(&models.ContactShare{}).Where("id = ?", id).UpdateColumn("created_at", old).Error)
	cfg.ContactShareRetentionDays = 30
	require.NoError(t, services.PurgeExpiredContactShares(db, *cfg))
	var n int64
	require.NoError(t, db.Model(&models.ContactShare{}).Where("id = ?", id).Count(&n).Error)
	assert.Zero(t, n)
}
