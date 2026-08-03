package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGiftTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &Gift{}))
	return db
}

func TestGiftBeforeCreateGeneratesUUID(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gift := Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "She liked the ceramics shop"}
	require.NoError(t, db.Create(&gift).Error)

	assert.NotEmpty(t, gift.ID)
}

func TestGiftBeforeCreatePreservesExplicitID(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gift := Gift{ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, Description: "A plant"}
	require.NoError(t, db.Create(&gift).Error)

	assert.Equal(t, "explicit-id", gift.ID)
}

// T20b: status defaults to "idea" (a gift idea is captured opportunistically
// without choosing a state), and every GiftStatus* token round-trips.
func TestGiftStatusDefaultAndRoundTrip(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gift := Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "She mentioned she liked X"}
	require.NoError(t, db.Create(&gift).Error)
	assert.Equal(t, GiftStatusIdea, gift.Status, "status must default to idea")

	date := time.Now().Truncate(time.Second)
	require.NoError(t, db.Model(&gift).Updates(map[string]any{
		"status": GiftStatusGiven, "date": date, "value_cents": 2500, "currency": "EUR",
	}).Error)

	var reloaded Gift
	require.NoError(t, db.First(&reloaded, "id = ?", gift.ID).Error)
	assert.Equal(t, GiftStatusGiven, reloaded.Status)
	require.NotNil(t, reloaded.Date)
	assert.Equal(t, date.Unix(), reloaded.Date.Unix())
	assert.EqualValues(t, 2500, reloaded.ValueCents)
	assert.Equal(t, "EUR", reloaded.Currency)
}

// T20b's explicit-currency rule is enforced in the controller, but the model
// must persist the pair — this pins that a value with its currency survives a
// round-trip through the real column names (value_cents/currency).
func TestGiftValueAndCurrencyRoundTrip(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gift := Gift{
		UserID: user.ID, EntityID: contact.VCardUID, Status: GiftStatusPurchased,
		Description: "Scarf", ValueCents: 4200, Currency: "USD",
	}
	require.NoError(t, db.Create(&gift).Error)

	var reloaded Gift
	require.NoError(t, db.First(&reloaded, "id = ?", gift.ID).Error)
	assert.EqualValues(t, 4200, reloaded.ValueCents)
	assert.Equal(t, "USD", reloaded.Currency)
}

// The optional LifeEvent/Activity references are soft references (no FK) — the
// model must round-trip them, and ownership is the controller's job.
func TestGiftReferencesRoundTrip(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	activityID := uint(7)
	gift := Gift{
		UserID: user.ID, EntityID: contact.VCardUID, Description: "Wedding present",
		Occasion: "their wedding", LifeEventID: "evt-123", ActivityID: &activityID,
	}
	require.NoError(t, db.Create(&gift).Error)

	var reloaded Gift
	require.NoError(t, db.First(&reloaded, "id = ?", gift.ID).Error)
	assert.Equal(t, "evt-123", reloaded.LifeEventID)
	require.NotNil(t, reloaded.ActivityID)
	assert.Equal(t, activityID, *reloaded.ActivityID)
}

// T26: gift records are user-authored content, so delete must be a soft delete.
func TestGiftSoftDelete(t *testing.T) {
	db := setupGiftTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gift := Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "A book"}
	require.NoError(t, db.Create(&gift).Error)

	require.NoError(t, db.Delete(&gift).Error)

	var count int64
	require.NoError(t, db.Model(&Gift{}).Where("id = ?", gift.ID).Count(&count).Error)
	assert.Zero(t, count, "soft-deleted row must vanish from the browse query")

	var unscopedCount int64
	require.NoError(t, db.Unscoped().Model(&Gift{}).Where("id = ?", gift.ID).Count(&unscopedCount).Error)
	assert.EqualValues(t, 1, unscopedCount, "soft-deleted row must survive for the change feed / retention")
}
