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
