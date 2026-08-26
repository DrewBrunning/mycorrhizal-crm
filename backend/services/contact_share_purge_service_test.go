package services

import (
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #574: ContactShare snapshots are frozen, independent PII copies with
// no soft delete and no FK back to the original Contact — the ONLY thing that
// ever removed them was DeleteUser's cascade, so every pending/declined/
// accepted share sat in the DB (and every backup) forever. These tests pin the
// CONTACT_SHARE_RETENTION_DAYS window and its two anchors: created_at for
// pending (still actionable), responded_at for accepted/declined (already
// actioned).
//
// Real migrated schema (database.InitDB), not AutoMigrate, per CLAUDE.md trap
// #1 — the raw SQL names the real contact_shares columns.

func newSharePurgeDB(t *testing.T) (*gorm.DB, uint, uint) {
	t.Helper()

	db, err := database.InitDB(filepath.Join(t.TempDir(), "sharepurge.db"))
	require.NoError(t, err)

	from := models.User{Username: "share-from", Email: "share-from@example.com", Password: "x"}
	to := models.User{Username: "share-to", Email: "share-to@example.com", Password: "x"}
	require.NoError(t, db.Create(&from).Error)
	require.NoError(t, db.Create(&to).Error)
	return db, from.ID, to.ID
}

func sharePurgeConfig() config.Config {
	return config.Config{ContactShareRetentionDays: 30}
}

func createShare(t *testing.T, db *gorm.DB, fromID, toID uint, status string) models.ContactShare {
	t.Helper()
	share := models.ContactShare{
		FromUserID:         fromID,
		ToUserID:           toID,
		ContactDisplayName: "Alice",
		Payload:            `[{"@type":"Card","uid":"x","fullName":"Alice"}]`,
		Status:             status,
	}
	require.NoError(t, db.Create(&share).Error)
	return share
}

// backdateShareCreatedAt ages a share's created_at so it falls on the chosen
// side of the retention cutoff — GORM stamps "now" on Create, which is always
// inside the window, so tests must set it explicitly.
func backdateShareCreatedAt(t *testing.T, db *gorm.DB, shareID string, when time.Time) {
	t.Helper()
	require.NoError(t, db.Model(&models.ContactShare{}).Where("id = ?", shareID).
		Update("created_at", when).Error)
}

func setRespondedAt(t *testing.T, db *gorm.DB, shareID string, when time.Time) {
	t.Helper()
	require.NoError(t, db.Model(&models.ContactShare{}).Where("id = ?", shareID).
		Update("responded_at", when).Error)
}

func countShares(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.ContactShare{}).Count(&count).Error)
	return count
}

// A pending share older than the window must be purged: an abandoned invite
// should not be actionable years later, and the frozen snapshot it carries is
// exactly the unbounded PII copy this ticket exists to bound.
func TestPurgeExpiredContactShares_PendingPastRetention(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Zero(t, countShares(t, db), "a pending share created 60 days ago must be purged at a 30-day window")
}

func TestPurgeExpiredContactShares_PendingInsideRetention(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -5))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Equal(t, int64(1), countShares(t, db), "a pending share created 5 days ago is still inside a 30-day window")
}

// An accepted share's payload was already imported into the recipient's
// account; the snapshot's purpose is done once it crosses the window, so it is
// purged from the response to its owner's actual data (which lives on).
func TestPurgeExpiredContactShares_AcceptedPastRetention(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusAccepted)
	setRespondedAt(t, db, share.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Zero(t, countShares(t, db), "an accepted share responded to 60 days ago must be purged")
}

func TestPurgeExpiredContactShares_DeclinedPastRetention(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusDeclined)
	setRespondedAt(t, db, share.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Zero(t, countShares(t, db), "a declined share responded to 60 days ago must be purged")
}

func TestPurgeExpiredContactShares_RespondedInsideRetention(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusAccepted)
	setRespondedAt(t, db, share.ID, time.Now().AddDate(0, 0, -5))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Equal(t, int64(1), countShares(t, db), "an accepted share responded to 5 days ago must survive")
}

// A responded share is aged from responded_at, not created_at — a share
// created long ago but responded to yesterday is fresh, not stale.
func TestPurgeExpiredContactShares_AgesRespondedFromRespondedAt(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusDeclined)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -90))
	setRespondedAt(t, db, share.ID, time.Now().AddDate(0, 0, -5))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Equal(t, int64(1), countShares(t, db), "a declined share created 90 days ago but responded to 5 days ago must survive")
}

// Defensive: a responded status with a NULL responded_at (corrupt state that
// no controller path produces) must never be purged — there is no timestamp to
// age it from, and an unconditional delete could remove data the user can
// still see. The SQL's `responded_at IS NOT NULL` guard pins this.
func TestPurgeExpiredContactShares_NullRespondedAtNeverPurged(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusAccepted)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -90))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Equal(t, int64(1), countShares(t, db), "an accepted share with NULL responded_at must not be purged")
}

func TestPurgeExpiredContactShares_MixedStatuses(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	oldPending := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, oldPending.ID, time.Now().AddDate(0, 0, -60))

	recentPending := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, recentPending.ID, time.Now().AddDate(0, 0, -5))

	oldAccepted := createShare(t, db, fromID, toID, models.ContactShareStatusAccepted)
	setRespondedAt(t, db, oldAccepted.ID, time.Now().AddDate(0, 0, -60))

	recentAccepted := createShare(t, db, fromID, toID, models.ContactShareStatusAccepted)
	setRespondedAt(t, db, recentAccepted.ID, time.Now().AddDate(0, 0, -5))

	oldDeclined := createShare(t, db, fromID, toID, models.ContactShareStatusDeclined)
	setRespondedAt(t, db, oldDeclined.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Equal(t, int64(2), countShares(t, db),
		"only the shares past their window (old pending + old accepted + old declined) must be purged, leaving the two fresh ones")
}

// CONTACT_SHARE_RETENTION_DAYS <= 0 means "disabled", never "delete
// everything" — mirrors audit_purge_service.go's stance on a misconfigured
// window.
func TestPurgeExpiredContactShares_DisabledWhenRetentionZero(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, config.Config{ContactShareRetentionDays: 0})

	assert.Equal(t, int64(1), countShares(t, db), "a zero retention window must disable the purge, not delete every share")
}

func TestPurgeExpiredContactShares_IsIdempotent(t *testing.T) {
	db, fromID, toID := newSharePurgeDB(t)

	share := createShare(t, db, fromID, toID, models.ContactShareStatusPending)
	backdateShareCreatedAt(t, db, share.ID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredContactShares(db, sharePurgeConfig())
	PurgeExpiredContactShares(db, sharePurgeConfig())

	assert.Zero(t, countShares(t, db))
}
