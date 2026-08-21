package services

import (
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestListPushSubscriptions uses the notification test DB's real migrated
// schema (setupNotificationTestDB) and the same endpoint the production
// CreatePushSubscription stores.
func TestListPushSubscriptions(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := models.User{Username: "pushlistuser", Password: "password123!A", Email: "pushlist@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// No subscriptions yet.
	subs, err := ListPushSubscriptions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, subs)

	// Create two for this user and one for another user.
	sub1, err := CreatePushSubscription(db, user.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/1", P256dh: "p1", Auth: "a1", DeviceLabel: "Phone",
	})
	require.NoError(t, err)
	sub2, err := CreatePushSubscription(db, user.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/2", P256dh: "p2", Auth: "a2", DeviceLabel: "Laptop",
	})
	require.NoError(t, err)

	other := models.User{Username: "pushothero", Password: "password123!A", Email: "pushother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	_, err = CreatePushSubscription(db, other.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/other", P256dh: "p9", Auth: "a9", DeviceLabel: "Other",
	})
	require.NoError(t, err)

	subs, err = ListPushSubscriptions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 2, "must return only this user's subscriptions")
	ids := map[uint]string{}
	for _, s := range subs {
		ids[s.ID] = s.DeviceLabel
	}
	assert.Equal(t, "Phone", ids[sub1.ID])
	assert.Equal(t, "Laptop", ids[sub2.ID])
}

func TestDeletePushSubscription(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := models.User{Username: "pushdeluser", Password: "password123!A", Email: "pushdel@example.com"}
	require.NoError(t, db.Create(&user).Error)

	sub, err := CreatePushSubscription(db, user.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/del", P256dh: "p", Auth: "a", DeviceLabel: "Phone",
	})
	require.NoError(t, err)

	require.NoError(t, DeletePushSubscription(db, user.ID, sub.ID))
	subs, err := ListPushSubscriptions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, subs, "the deleted subscription must be gone")

	// Deleting again (already gone) is gorm.ErrRecordNotFound.
	err = DeletePushSubscription(db, user.ID, sub.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestDeletePushSubscription_OtherUsersSubscription(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := models.User{Username: "pushownuser", Password: "password123!A", Email: "pushown@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "pushdelother", Password: "password123!A", Email: "pushdelother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	otherSub, err := CreatePushSubscription(db, other.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/owned-by-other", P256dh: "p", Auth: "a", DeviceLabel: "Other",
	})
	require.NoError(t, err)

	err = DeletePushSubscription(db, user.ID, otherSub.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a user must not delete another user's subscription")

	// The row survives.
	subs, err := ListPushSubscriptions(db, other.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
}
