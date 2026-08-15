package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupConversationAgendaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &ConversationAgenda{}))
	return db
}

func TestConversationAgendaBeforeCreateGeneratesUUID(t *testing.T) {
	db := setupConversationAgendaTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	item := ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about her mother's surgery"}
	require.NoError(t, db.Create(&item).Error)

	assert.NotEmpty(t, item.ID)
}

func TestConversationAgendaBeforeCreatePreservesExplicitID(t *testing.T) {
	db := setupConversationAgendaTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	item := ConversationAgenda{ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the job"}
	require.NoError(t, db.Create(&item).Error)

	assert.Equal(t, "explicit-id", item.ID)
}

// The discussed flag with its date is the agenda's only resolution mechanism —
// pin that it round-trips and stays queryable both ways.
func TestConversationAgendaDiscussedAtRoundTrip(t *testing.T) {
	db := setupConversationAgendaTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	now := time.Now().Truncate(time.Second)
	item := ConversationAgenda{
		UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the new house",
		DiscussedAt: &now,
	}
	require.NoError(t, db.Create(&item).Error)

	var reloaded ConversationAgenda
	require.NoError(t, db.First(&reloaded, "id = ?", item.ID).Error)
	require.NotNil(t, reloaded.DiscussedAt)
	assert.Equal(t, now.Unix(), reloaded.DiscussedAt.Unix())

	// Open items are filtered out by IS NULL, discussed ones by IS NOT NULL.
	var openCount, discussedCount int64
	require.NoError(t, db.Model(&ConversationAgenda{}).Where("discussed_at IS NULL").Count(&openCount).Error)
	require.NoError(t, db.Model(&ConversationAgenda{}).Where("discussed_at IS NOT NULL").Count(&discussedCount).Error)
	assert.EqualValues(t, 0, openCount)
	assert.EqualValues(t, 1, discussedCount)
}

// T26: agenda items are user-authored content, so delete must be a soft
// delete — discussed items must not vanish irrecoverably.
func TestConversationAgendaSoftDelete(t *testing.T) {
	db := setupConversationAgendaTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	item := ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the trip"}
	require.NoError(t, db.Create(&item).Error)

	require.NoError(t, db.Delete(&item).Error)

	var count int64
	require.NoError(t, db.Model(&ConversationAgenda{}).Where("id = ?", item.ID).Count(&count).Error)
	assert.Zero(t, count, "soft-deleted row must vanish from the browse query")

	var unscopedCount int64
	require.NoError(t, db.Unscoped().Model(&ConversationAgenda{}).Where("id = ?", item.ID).Count(&unscopedCount).Error)
	assert.EqualValues(t, 1, unscopedCount, "soft-deleted row must survive for the change feed / retention")
}
