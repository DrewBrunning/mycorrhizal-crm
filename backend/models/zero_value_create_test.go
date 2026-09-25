package models

import (
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The PR #1240 bug class, found by TestModelTagDefaultsMatchMigrations on
// three more models: a `default:true` / `default:N` GORM tag on a non-pointer
// field makes GORM leave the zero value out of the INSERT, so the column
// default wins. Every one of these was reachable from the API:
//
//   - POST /calendars {"sync_enabled": false, "past_days": 0,
//     "future_days": 0} persisted sync_enabled=true, past_days=5,
//     future_days=10 (the DTO validates min=0);
//   - POST /contact-subscriptions {"sync_enabled": false} persisted true;
//   - PUT /immich/config {"sync_enabled": false} on first connect persisted
//     true.
//
// Each case creates the zero value against the real migrated schema, reloads,
// and asserts it stuck.
func TestCreateKeepsExplicitZeroValues(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "zero-create", Password: "x", Email: "zero-create@example.com"}
	require.NoError(t, db.Create(&user).Error)

	t.Run("CalendarSubscription", func(t *testing.T) {
		sub := CalendarSubscription{UserID: user.ID, Name: "cal", URL: "https://example.com/cal.ics", SyncEnabled: false, PastDays: 0, FutureDays: 0}
		require.NoError(t, db.Create(&sub).Error)
		var got CalendarSubscription
		require.NoError(t, db.First(&got, sub.ID).Error)
		assert.False(t, got.SyncEnabled, "sync_enabled=false must survive Create")
		assert.Zero(t, got.PastDays, "past_days=0 must survive Create")
		assert.Zero(t, got.FutureDays, "future_days=0 must survive Create")
	})

	t.Run("ContactSubscription", func(t *testing.T) {
		sub := ContactSubscription{UserID: user.ID, Name: "dav", URL: "https://example.com/dav/", SyncEnabled: false}
		require.NoError(t, db.Create(&sub).Error)
		var got ContactSubscription
		require.NoError(t, db.First(&got, sub.ID).Error)
		assert.False(t, got.SyncEnabled, "sync_enabled=false must survive Create")
	})

	t.Run("ImmichConfig", func(t *testing.T) {
		ic := ImmichConfig{UserID: user.ID, BaseURL: "https://immich.example.com", APIKeyEncrypted: "enc", SyncEnabled: false}
		require.NoError(t, db.Create(&ic).Error)
		var got ImmichConfig
		require.NoError(t, db.First(&got, ic.ID).Error)
		assert.False(t, got.SyncEnabled, "sync_enabled=false must survive Create")
	})
}
