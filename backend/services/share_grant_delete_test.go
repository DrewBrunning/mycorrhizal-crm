package services

import (
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2742: DeleteShareGrant's compare-and-delete. A grant that moves
// between the owner's read and the delete is not removed, and answers the
// one 404.
func TestDeleteShareGrant_StatusChangedBetweenReadAndDeleteIsNotFoundAndKept(t *testing.T) {
	f := newGrantRevokeFixture(t)
	g := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityNote, "n-1", models.ShareGrantStatusRevoked)

	// After the owner-scoped read, flip the row (as a concurrent transition
	// would) before the delete runs.
	const cb = "test:flip_after_load"
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "share_grants" && tx.Error == nil {
			tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE share_grants SET status = ? WHERE id = ?", models.ShareGrantStatusActive, g.ID)
		}
	}))
	appErr := DeleteShareGrant(f.db, f.owner.ID, g.ID, f.nowRef)
	require.NoError(t, f.db.Callback().Query().Remove(cb))

	require.NotNil(t, appErr)
	assert.Equal(t, ErrShareGrantNotFound().Error(), appErr.Error())
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, g.ID).Status, "the row is still there")
}

func TestDeleteShareGrant_EndedIsDeletedLiveIsConflictForeignIsNotFound(t *testing.T) {
	f := newGrantRevokeFixture(t)
	ended := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityNote, "n-1", models.ShareGrantStatusDeclined)
	live := f.grant(t, f.owner.ID, f.rcpt[1].ID, models.ShareGrantEntityNote, "n-2", models.ShareGrantStatusActive)

	// Not the owner's: the one 404, nothing deleted.
	appErr := DeleteShareGrant(f.db, f.other.ID, ended.ID, f.nowRef)
	require.NotNil(t, appErr)
	assert.Equal(t, ErrShareGrantNotFound().Error(), appErr.Error())
	assert.Equal(t, models.ShareGrantStatusDeclined, f.reload(t, ended.ID).Status)

	// A live grant is the 409 for its owner.
	appErr = DeleteShareGrant(f.db, f.owner.ID, live.ID, f.nowRef)
	require.NotNil(t, appErr)
	assert.Equal(t, "share_grant_not_ended", appErr.ReasonCode)
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, live.ID).Status)

	require.Nil(t, DeleteShareGrant(f.db, f.owner.ID, ended.ID, f.nowRef))
	var n int64
	require.NoError(t, f.db.Model(&models.ShareGrant{}).Where("id = ?", ended.ID).Count(&n).Error)
	assert.Zero(t, n)
	assert.NotNil(t, DeleteShareGrant(f.db, f.owner.ID, ended.ID, f.nowRef), "a dismissed id is an unknown id")
}

func TestDeleteShareGrant_DeleteFailureIsAServerError(t *testing.T) {
	f := newGrantRevokeFixture(t)
	g := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityNote, "n-1", models.ShareGrantStatusRevoked)
	const cb = "test:fail_delete"
	require.NoError(t, f.db.Callback().Delete().Before("gorm:delete").Register(cb, func(tx *gorm.DB) {
		_ = tx.AddError(gorm.ErrInvalidDB)
	}))
	appErr := DeleteShareGrant(f.db, f.owner.ID, g.ID, f.nowRef)
	require.NoError(t, f.db.Callback().Delete().Remove(cb))
	require.NotNil(t, appErr)
	assert.Equal(t, "DATABASE_ERROR", appErr.Code)
}
