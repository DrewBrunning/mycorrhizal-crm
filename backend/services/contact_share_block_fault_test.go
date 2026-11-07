package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbfault"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

// Issue #2294: a database failure while ending a blocked sender's pending
// one-time shares surfaces as an error (so the surrounding block transaction
// rolls back) and never leaves a half-declined state.
func TestDeclineBlockedOneTimeShares_DatabaseFailuresSurface(t *testing.T) {
	for _, verb := range []string{"query", "update"} {
		t.Run(verb, func(t *testing.T) {
			db := dbtest.New(t, dbtest.WithFaults())
			a := models.User{Username: "blk-a", Email: "blk-a@example.com", Password: "x"}
			b := models.User{Username: "blk-b", Email: "blk-b@example.com", Password: "x"}
			require.NoError(t, db.Create(&a).Error)
			require.NoError(t, db.Create(&b).Error)
			share := models.ContactShare{FromUserID: a.ID, ToUserID: b.ID, Payload: "[]", Status: models.ContactShareStatusPending}
			require.NoError(t, db.Create(&share).Error)

			dbfault.For(db).FailMatching(verb, "contact_shares")
			require.Error(t, declineBlockedOneTimeShares(db, b.ID, a.ID, time.Now()))
			dbfault.For(db).Disarm()

			var got models.ContactShare
			require.NoError(t, db.First(&got, "id = ?", share.ID).Error)
			require.Equal(t, models.ContactShareStatusPending, got.Status)
		})
	}
}
