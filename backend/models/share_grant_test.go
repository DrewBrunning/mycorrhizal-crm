package models_test

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedGrantUsers(t *testing.T, db *gorm.DB) (models.User, models.User) {
	t.Helper()
	a := models.User{Username: "grant-a", Email: "grant-a@example.com", Password: "x"}
	b := models.User{Username: "grant-b", Email: "grant-b@example.com", Password: "x"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	return a, b
}

func newGrant(from, to uint, entityID, status string) models.ShareGrant {
	return models.ShareGrant{FromUserID: from, ToUserID: to, EntityType: models.ShareGrantEntityContact, EntityID: entityID, Status: status}
}

func TestShareGrant_BeforeCreateDefaults(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)

	g := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusPending)
	require.NoError(t, db.Create(&g).Error)
	assert.NotEmpty(t, g.ID, "UUID generated in BeforeCreate")
	assert.EqualValues(t, 1, g.Revision)
	assert.Equal(t, "e-"+g.ID+"-1", g.ETag())

	var raw struct{ Sections, ReconfirmSections string }
	require.NoError(t, db.Raw("SELECT sections AS sections, reconfirm_sections AS reconfirm_sections FROM share_grants WHERE id = ?", g.ID).Scan(&raw).Error)
	assert.Equal(t, "[]", raw.Sections, "nil sections persist as an empty JSON array, never null")
	assert.Equal(t, "[]", raw.ReconfirmSections)

	var back models.ShareGrant
	require.NoError(t, db.First(&back, "id = ?", g.ID).Error)
	assert.NotNil(t, back.Sections)
	assert.Empty(t, back.Sections)
}

func TestShareGrant_SectionsRoundTripAndRevisionNotClobbered(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)

	g := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusActive)
	g.Sections = []string{"phones", "emails"}
	require.NoError(t, db.Create(&g).Error)

	var back models.ShareGrant
	require.NoError(t, db.First(&back, "id = ?", g.ID).Error)
	assert.Equal(t, []string{"phones", "emails"}, back.Sections)

	// <-:create: a plain Save cannot rewrite the revision counter.
	back.Revision = 99
	back.Status = models.ShareGrantStatusRevoked
	require.NoError(t, db.Save(&back).Error)
	var again models.ShareGrant
	require.NoError(t, db.First(&again, "id = ?", g.ID).Error)
	assert.EqualValues(t, 1, again.Revision)
	assert.Equal(t, models.ShareGrantStatusRevoked, again.Status)
}

func TestShareGrant_ChecksRejectedByRealSchema(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)

	self := newGrant(a.ID, a.ID, "uid-1", models.ShareGrantStatusPending)
	assert.Error(t, db.Create(&self).Error, "from_user_id <> to_user_id")

	badType := newGrant(a.ID, b.ID, "x", models.ShareGrantStatusPending)
	badType.EntityType = "reminder"
	assert.Error(t, db.Create(&badType).Error, "entity_type closed set")

	badStatus := newGrant(a.ID, b.ID, "x", "snoozed")
	assert.Error(t, db.Create(&badStatus).Error, "status closed set")

	reason := "because"
	badReason := newGrant(a.ID, b.ID, "x", models.ShareGrantStatusRevoked)
	badReason.RevokedReason = &reason
	assert.Error(t, db.Create(&badReason).Error, "revoked_reason closed set")

	for _, r := range []string{models.ShareGrantReasonOwnerRevoked, models.ShareGrantReasonEntityDeleted, models.ShareGrantReasonRecipientDeleted} {
		rv := r
		ok := newGrant(a.ID, b.ID, "reason-"+r, models.ShareGrantStatusRevoked)
		ok.RevokedReason = &rv
		assert.NoError(t, db.Create(&ok).Error, r)
	}
}

func TestShareGrant_PartialUniqueLiveIndex(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)

	live := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusActive)
	require.NoError(t, db.Create(&live).Error)

	for _, st := range models.ShareGrantLiveStatuses {
		dup := newGrant(a.ID, b.ID, "uid-1", st)
		assert.Error(t, db.Create(&dup).Error, "second live grant (%s) for the same tuple", st)
	}

	// History rows coexist with the live grant and with each other.
	now := time.Now()
	for _, st := range []string{models.ShareGrantStatusDeclined, models.ShareGrantStatusRevoked, models.ShareGrantStatusExpired, models.ShareGrantStatusRevoked} {
		h := newGrant(a.ID, b.ID, "uid-1", st)
		h.RespondedAt = &now
		require.NoError(t, db.Create(&h).Error, st)
	}

	// Another entity, direction, or type is a distinct tuple.
	other := newGrant(a.ID, b.ID, "uid-2", models.ShareGrantStatusPending)
	require.NoError(t, db.Create(&other).Error)
	rev := newGrant(b.ID, a.ID, "uid-1", models.ShareGrantStatusPending)
	require.NoError(t, db.Create(&rev).Error)
	note := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusPending)
	note.EntityType = models.ShareGrantEntityNote
	require.NoError(t, db.Create(&note).Error)

	// Ending the live grant frees the slot.
	require.NoError(t, db.Model(&models.ShareGrant{}).Where("id = ?", live.ID).Update("status", models.ShareGrantStatusRevoked).Error)
	again := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusPending)
	assert.NoError(t, db.Create(&again).Error)
}

func TestShareGrant_UserDeleteCascadesBothSides(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	out := newGrant(a.ID, b.ID, "uid-1", models.ShareGrantStatusActive)
	in := newGrant(b.ID, a.ID, "uid-2", models.ShareGrantStatusActive)
	require.NoError(t, db.Create(&out).Error)
	require.NoError(t, db.Create(&in).Error)
	require.NoError(t, db.Create(&models.ShareBlock{BlockerUserID: a.ID, BlockedUserID: b.ID}).Error)

	require.NoError(t, db.Unscoped().Delete(&models.User{}, a.ID).Error)

	var grants, blocks int64
	require.NoError(t, db.Model(&models.ShareGrant{}).Count(&grants).Error)
	require.NoError(t, db.Model(&models.ShareBlock{}).Count(&blocks).Error)
	assert.Zero(t, grants)
	assert.Zero(t, blocks)
}

func TestShareBlock_CompositeKeyHardDelete(t *testing.T) {
	db := dbtest.New(t)
	a, b := seedGrantUsers(t, db)

	require.NoError(t, db.Create(&models.ShareBlock{BlockerUserID: a.ID, BlockedUserID: b.ID}).Error)
	assert.Error(t, db.Create(&models.ShareBlock{BlockerUserID: a.ID, BlockedUserID: b.ID}).Error, "one row per pair")
	require.NoError(t, db.Create(&models.ShareBlock{BlockerUserID: b.ID, BlockedUserID: a.ID}).Error, "reverse is separate")

	require.NoError(t, db.Delete(&models.ShareBlock{}, "blocker_user_id = ? AND blocked_user_id = ?", a.ID, b.ID).Error)
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.ShareBlock{}).Where("blocker_user_id = ?", a.ID).Count(&n).Error)
	assert.Zero(t, n, "unblock is a physical delete")
}
