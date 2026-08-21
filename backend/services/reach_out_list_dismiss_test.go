package services

import (
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedReachOutSuggestion creates a pending suggestion + companion reminder
// directly (the detection job is exercised by TestDetectReachOutSuggestions_*).
func seedReachOutSuggestion(t *testing.T, db *gorm.DB, userID uint, contact models.Contact, kind string) models.ReachOutSuggestion {
	t.Helper()
	reminder := models.Reminder{UserID: userID, Message: "Reach out", RemindAt: time.Now().UTC(), Recurrence: "once", ContactID: &contact.ID}
	require.NoError(t, db.Create(&reminder).Error)
	suggestion := models.ReachOutSuggestion{
		UserID:          userID,
		ContactVCardUID: contact.VCardUID,
		Kind:            kind,
		OldValue:        "Old",
		NewValue:        "New",
		AuditEventID:    1,
		ReminderID:      &reminder.ID,
		Status:          models.ReachOutStatusPending,
	}
	require.NoError(t, db.Create(&suggestion).Error)
	return suggestion
}

func TestListReachOutSuggestions(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutlist", Password: "password123!A", Email: "list@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Smith"}
	require.NoError(t, db.Create(&contact).Error)

	seedReachOutSuggestion(t, db, user.ID, contact, models.ReachOutKindOrganization)

	suggestions, err := ListReachOutSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, contact.VCardUID, suggestions[0].ContactVCardUID)
	assert.Equal(t, contact.ID, suggestions[0].ContactID)
	assert.Equal(t, "Alice Smith", suggestions[0].ContactName)

	// Dismissed suggestions are excluded.
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Where("user_id = ?", user.ID).Update("status", models.ReachOutStatusDismissed).Error)
	suggestions, err = ListReachOutSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

func TestListReachOutSuggestions_EmptyAndOtherUsers(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutempty", Password: "password123!A", Email: "empty@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// No suggestions for a user with none.
	suggestions, err := ListReachOutSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, suggestions)

	// Another user's suggestions are not visible.
	other := models.User{Username: "reachoutother", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	seedReachOutSuggestion(t, db, other.ID, otherContact, models.ReachOutKindTitle)

	suggestions, err = ListReachOutSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, suggestions)
}

func TestListReachOutSuggestions_ArchivedContactDropped(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutarch", Password: "password123!A", Email: "arch@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Archived: true}
	require.NoError(t, db.Create(&contact).Error)
	seedReachOutSuggestion(t, db, user.ID, contact, models.ReachOutKindAddress)

	suggestions, err := ListReachOutSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions, "a suggestion pointing at an archived contact is dropped")
}

func TestDismissReachOutSuggestion(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutdismiss", Password: "password123!A", Email: "dismiss@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	suggestion := seedReachOutSuggestion(t, db, user.ID, contact, models.ReachOutKindOrganization)

	require.NoError(t, DismissReachOutSuggestion(db, user.ID, suggestion.ID))
	var reloaded models.ReachOutSuggestion
	require.NoError(t, db.First(&reloaded, "id = ?", suggestion.ID).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, reloaded.Status)

	// Idempotent: re-dismissing is a no-op success.
	require.NoError(t, DismissReachOutSuggestion(db, user.ID, suggestion.ID))
}

func TestDismissReachOutSuggestion_NotFound(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutmiss", Password: "password123!A", Email: "miss@example.com"}
	require.NoError(t, db.Create(&user).Error)

	err := DismissReachOutSuggestion(db, user.ID, "00000000-0000-4000-8000-000000000000")
	require.Error(t, err)

	// A suggestion owned by another user is not dismissible by this one.
	other := models.User{Username: "reachoutowner", Password: "password123!A", Email: "owner@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	s := seedReachOutSuggestion(t, db, other.ID, otherContact, models.ReachOutKindTitle)
	err = DismissReachOutSuggestion(db, user.ID, s.ID)
	require.Error(t, err, "a suggestion owned by another user must not be dismissible")
}

func TestDismissReachOutSuggestionByReminderID(t *testing.T) {
	db := setupReachOutTestDB(t)
	user := models.User{Username: "reachoutrem", Password: "password123!A", Email: "rem@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	suggestion := seedReachOutSuggestion(t, db, user.ID, contact, models.ReachOutKindOrganization)
	require.NotNil(t, suggestion.ReminderID)

	require.NoError(t, DismissReachOutSuggestionByReminderID(db, user.ID, *suggestion.ReminderID))
	var reloaded models.ReachOutSuggestion
	require.NoError(t, db.First(&reloaded, "id = ?", suggestion.ID).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, reloaded.Status)

	// A no-op when nothing references the reminder.
	require.NoError(t, DismissReachOutSuggestionByReminderID(db, user.ID, 99999))

	// Only the owner's suggestion is affected.
	other := models.User{Username: "reachoutrem2", Password: "password123!A", Email: "rem2@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	otherSuggestion := seedReachOutSuggestion(t, db, other.ID, otherContact, models.ReachOutKindTitle)
	require.NotNil(t, otherSuggestion.ReminderID)

	require.NoError(t, DismissReachOutSuggestionByReminderID(db, user.ID, *otherSuggestion.ReminderID))
	var reloadedOther models.ReachOutSuggestion
	require.NoError(t, db.First(&reloadedOther, "id = ?", otherSuggestion.ID).Error)
	assert.Equal(t, models.ReachOutStatusPending, reloadedOther.Status, "a reminder owned by another user must not dismiss this user's suggestion")
}
