package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupUpcomingReminderTestDB opens a real migrated schema (CLAUDE.md backend
// trap 1 — Reminder's columns must come from migration SQL, not AutoMigrate).
func setupUpcomingReminderTestDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "upcominguser", Password: "password123!A", Email: "upcoming@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func createReminder(t *testing.T, db *gorm.DB, userID uint, message string, remindAt time.Time, completed bool) models.Reminder {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Reminder"}
	require.NoError(t, db.Create(&contact).Error)
	r := models.Reminder{UserID: userID, Message: message, RemindAt: remindAt, Recurrence: "once", Completed: completed, ContactID: &contact.ID}
	require.NoError(t, db.Create(&r).Error)
	return r
}

func TestGetUpcomingReminders(t *testing.T) {
	db, user := setupUpcomingReminderTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// One due within the next 7 days.
	dueSoon := createReminder(t, db, user.ID, "Due soon", now.Add(2*24*time.Hour), false)
	// One far in the future (beyond the 7-day window).
	createReminder(t, db, user.ID, "Far future", now.Add(30*24*time.Hour), false)
	// One already completed (must never appear).
	createReminder(t, db, user.ID, "Completed", now.Add(1*24*time.Hour), true)

	// Fewer than 5 due in the window, so the fallback (next 5 overall) applies
	// and includes the far-future reminder but excludes completed ones.
	result, err := GetUpcomingReminders(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, dueSoon.ID, result[0].ID)
	assert.Equal(t, "Far future", result[1].Message)
}

func TestGetUpcomingReminders_SixInWindowUsesWindow(t *testing.T) {
	db, user := setupUpcomingReminderTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// Six reminders within the 7-day window: the window wins, even though the
	// fallback would also include the far-future one.
	for i := 0; i < 6; i++ {
		createReminder(t, db, user.ID, "Window "+string(rune('A'+i)), now.Add(time.Duration(i)*24*time.Hour), false)
	}
	createReminder(t, db, user.ID, "Far future", now.Add(60*24*time.Hour), false)

	result, err := GetUpcomingReminders(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, result, 6)
	for _, r := range result {
		assert.False(t, r.Completed)
		assert.False(t, r.RemindAt.After(now.AddDate(0, 0, 7)), "all returned reminders must be within the 7-day window")
	}
}

func TestGetUpcomingReminders_OrderingAndIsolation(t *testing.T) {
	db, user := setupUpcomingReminderTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// Created out of order; the result must be ordered by remind_at ASC.
	createReminder(t, db, user.ID, "Later", now.Add(5*24*time.Hour), false)
	createReminder(t, db, user.ID, "Earlier", now.Add(1*24*time.Hour), false)

	// Another user's reminders must never leak in.
	other := models.User{Username: "upcomingother", Password: "password123!A", Email: "upcomingother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	createReminder(t, db, other.ID, "Other user", now.Add(3*24*time.Hour), false)

	result, err := GetUpcomingReminders(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Earlier", result[0].Message)
	assert.Equal(t, "Later", result[1].Message)

	otherResult, err := GetUpcomingReminders(db, other.ID, now)
	require.NoError(t, err)
	require.Len(t, otherResult, 1)
	assert.Equal(t, "Other user", otherResult[0].Message)
}
