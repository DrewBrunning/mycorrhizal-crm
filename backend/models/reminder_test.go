package models

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupReminderTestDB(t *testing.T) (*gorm.DB, User, Contact) {
	t.Helper()
	db := dbtest.New(t)
	user := User{Username: "reminder-uuid", Password: "x", Email: "reminder-uuid@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

// TestReminderBeforeCreateGeneratesUUID covers the stable portable UUID added
// for the account bundle (issue #1260, migration 000067): a new reminder gets
// one when the caller supplies none.
func TestReminderBeforeCreateGeneratesUUID(t *testing.T) {
	t.Parallel()
	db, user, contact := setupReminderTestDB(t)

	reminder := Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "call",
		RemindAt: time.Now(), Recurrence: "once",
	}
	require.NoError(t, db.Create(&reminder).Error)
	assert.NotEmpty(t, reminder.UUID, "BeforeCreate must mint a stable UUID")
}

func TestReminderBeforeCreatePreservesExplicitUUID(t *testing.T) {
	t.Parallel()
	db, user, contact := setupReminderTestDB(t)

	reminder := Reminder{
		UUID: "explicit-reminder-uuid", UserID: user.ID, ContactID: &contact.ID,
		Message: "call", RemindAt: time.Now(), Recurrence: "once",
	}
	require.NoError(t, db.Create(&reminder).Error)
	assert.Equal(t, "explicit-reminder-uuid", reminder.UUID)
}

func TestReminderCompletionBeforeCreateGeneratesUUID(t *testing.T) {
	t.Parallel()
	db, user, contact := setupReminderTestDB(t)

	completion := ReminderCompletion{
		UserID: user.ID, ContactID: contact.ID, Message: "done", CompletedAt: time.Now(),
	}
	require.NoError(t, db.Create(&completion).Error)
	assert.NotEmpty(t, completion.UUID, "BeforeCreate must mint a stable UUID")
}

func TestReminderCompletionBeforeCreatePreservesExplicitUUID(t *testing.T) {
	t.Parallel()
	db, user, contact := setupReminderTestDB(t)

	completion := ReminderCompletion{
		UUID: "explicit-completion-uuid", UserID: user.ID, ContactID: contact.ID,
		Message: "done", CompletedAt: time.Now(),
	}
	require.NoError(t, db.Create(&completion).Error)
	assert.Equal(t, "explicit-completion-uuid", completion.UUID)
}
