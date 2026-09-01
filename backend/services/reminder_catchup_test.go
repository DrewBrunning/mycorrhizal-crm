package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// reminderCatchupCfg is a config with email delivery enabled (Resend), so the
// email notification sender is Enabled and would attempt a send.
func reminderCatchupCfg() config.Config {
	return config.Config{
		UseResend:        true,
		ResendAPIKey:     "test_api_key",
		ResendFromEmail:  "noreply@example.com",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
	}
}

func seedDueReminder(t *testing.T, db *gorm.DB) models.Reminder {
	t.Helper()
	user := models.User{Username: "catchup", Password: "password123!A", Email: "catchup@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Due"}
	require.NoError(t, db.Create(&contact).Error)
	byMail := true
	rem := models.Reminder{
		UserID:     user.ID,
		ContactID:  &contact.ID,
		Message:    "call about the thing",
		ByMail:     &byMail,
		RemindAt:   time.Now().Add(-2 * time.Hour), // due
		Recurrence: "weekly",
	}
	require.NoError(t, db.Create(&rem).Error)
	return rem
}

// TestReminderCatchup_RestartTwiceInWindow_OneDeliveryPerChannel is issue
// #526's rule-3 check against the restart path (the ticket's "How to verify"
// bullet 3), on the real migrated schema (dbtest — CLAUDE.md backend trap #1).
//
// The job lock's de-dup window is set tiny here on purpose, so it does NOT
// suppress the second run — that forces SendReminders to execute twice, the
// exact scenario the notificationDeliveryKey `(reminder, channel)` guard is
// the last line of defence for. Exactly one 'sent' email delivery must exist,
// and the email digest must have been dispatched exactly once.
func TestReminderCatchup_RestartTwiceInWindow_OneDeliveryPerChannel(t *testing.T) {
	db := dbtest.New(t)
	rem := seedDueReminder(t, db)

	orig := ReminderMinInterval
	ReminderMinInterval = time.Millisecond // the lock will NOT suppress
	defer func() { ReminderMinInterval = orig }()

	sends := 0
	origSender := sendReminderEmailFn
	sendReminderEmailFn = func(models.User, []models.Reminder, config.Config, *gorm.DB) error {
		sends++
		return nil
	}
	defer func() { sendReminderEmailFn = origSender }()

	cfg := reminderCatchupCfg()

	// Two "restarts" inside the window: each fires the reminder job.
	for i := 0; i < 2; i++ {
		time.Sleep(2 * time.Millisecond) // clear the 1ms lock window
		_, err := SendRemindersWithRateLimit(db, cfg)
		require.NoError(t, err, "run %d", i)
	}

	assert.Equal(t, 1, sends, "the email digest must be dispatched exactly once across two runs (notificationDeliveryKey guard)")

	var sentEmail int64
	require.NoError(t, db.Model(&models.NotificationDelivery{}).
		Where("reminder_id = ? AND channel = ? AND status = ?", rem.ID, models.ChannelEmail, "sent").
		Count(&sentEmail).Error)
	assert.EqualValues(t, 1, sentEmail, "exactly one 'sent' email delivery row for (reminder, email)")

	var totalForReminder int64
	require.NoError(t, db.Model(&models.NotificationDelivery{}).
		Where("reminder_id = ?", rem.ID).Count(&totalForReminder).Error)
	assert.EqualValues(t, 1, totalForReminder, "no duplicate delivery rows of any status for this reminder")
}

// TestReminderCatchup_TwoRestartsNinetyMinutesApart_SecondIsSuppressed is the
// #526 rule-2 regression, and the hand-verify anchor: the daily reminder job's
// de-dup window is now derived from its 24h period (~23h30m), so two process
// restarts 90 minutes apart execute it exactly once. Under the old hand-picked
// 1-hour window the 90-minute-later restart re-entered SendReminders.
//
// Hand-verify (CLAUDE.md): set ReminderMinInterval back to time.Hour and this
// test fails — the second run is no longer ErrJobSkipped.
func TestReminderCatchup_TwoRestartsNinetyMinutesApart_SecondIsSuppressed(t *testing.T) {
	db := dbtest.New(t)
	seedDueReminder(t, db)

	// ReminderMinInterval keeps its real default here (JobCatchupWindow(24h) ≈ 23h30m).
	sends := 0
	origSender := sendReminderEmailFn
	sendReminderEmailFn = func(models.User, []models.Reminder, config.Config, *gorm.DB) error {
		sends++
		return nil
	}
	defer func() { sendReminderEmailFn = origSender }()

	cfg := reminderCatchupCfg()

	// Restart 1.
	_, err := SendRemindersWithRateLimit(db, cfg)
	require.NoError(t, err)

	// 90 minutes pass, then Restart 2.
	require.NoError(t, db.Model(&models.JobExecution{}).
		Where("job_name = ?", models.JobNameDailyReminders).
		Update("last_run_at", time.Now().Add(-90*time.Minute)).Error)

	_, err = SendRemindersWithRateLimit(db, cfg)
	require.ErrorIs(t, err, ErrJobSkipped,
		"a restart 90 min after the daily job ran must be suppressed by the period-derived window")

	assert.Equal(t, 1, sends, "the digest ran once across both restarts")

	var je models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameDailyReminders).First(&je).Error)
	assert.Equal(t, models.JobOutcomeDeduped, je.LastOutcome, "the suppressed run is recorded, not silent (rule 4)")
}
