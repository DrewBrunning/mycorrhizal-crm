package services

import (
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"os"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// ErrJobSkipped is returned by a scheduled job's entry point when the run did
// not execute — the distributed job lock was held by another instance, or the
// job ran too recently. main.go's job wrapper records these as a `skipped`
// job_runs row rather than a false `success` (issue #391; #526 "suppression is
// recorded, not silent").
var ErrJobSkipped = errors.New("scheduled job skipped: rate-limited or locked")

var sendReminderEmailFn = sendReminderEmail

// Default minimum interval between reminder job runs (prevents duplicates during restarts)
const DefaultReminderMinInterval = 1 * time.Hour

// ReminderMinInterval can be overridden for testing
var ReminderMinInterval = DefaultReminderMinInterval

// getInstanceID returns a unique identifier for this server instance
func getInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// acquireJobLock attempts to acquire a lock for the given job.
// Returns true if the lock was acquired, false if the job was run recently
// or is currently locked by another instance.
func acquireJobLock(db *gorm.DB, jobName string, minInterval time.Duration) (bool, error) {
	now := time.Now()
	instanceID := getInstanceID()
	lockTimeout := 5 * time.Minute // Consider locks stale after 5 minutes

	return db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution

		// Try to find existing job execution record
		err := tx.Where("job_name = ?", jobName).First(&job).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		if err == gorm.ErrRecordNotFound {
			// First time running this job - create the record and acquire lock
			job = models.JobExecution{
				JobName:   jobName,
				LastRunAt: now,
				LockedAt:  &now,
				LockedBy:  instanceID,
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock (first run)")
			return nil
		}

		// Job exists - check if we should run
		timeSinceLastRun := now.Sub(job.LastRunAt)
		if timeSinceLastRun < minInterval {
			logger.Info().
				Str("job", jobName).
				Dur("since_last_run", timeSinceLastRun).
				Dur("min_interval", minInterval).
				Msg("Skipping job - ran too recently")
			return fmt.Errorf("job ran too recently")
		}

		// Check if another instance has the lock
		if job.LockedAt != nil {
			lockAge := now.Sub(*job.LockedAt)
			if lockAge < lockTimeout && job.LockedBy != instanceID {
				logger.Info().
					Str("job", jobName).
					Str("locked_by", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Skipping job - locked by another instance")
				return fmt.Errorf("job locked by another instance")
			}
			// Lock is stale, we can take over
			if lockAge >= lockTimeout {
				logger.Warn().
					Str("job", jobName).
					Str("previous_instance", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Taking over stale lock")
			}
		}

		// Acquire the lock
		job.LockedAt = &now
		job.LockedBy = instanceID
		if err := tx.Save(&job).Error; err != nil {
			return err
		}

		logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock")
		return nil
	}) == nil, nil
}

// releaseJobLock releases the lock and updates the last run time
func releaseJobLock(db *gorm.DB, jobName string, success bool) error {
	now := time.Now()
	instanceID := getInstanceID()

	return db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution
		if err := tx.Where("job_name = ?", jobName).First(&job).Error; err != nil {
			return err
		}

		// Only update if we still hold the lock
		if job.LockedBy != instanceID {
			logger.Warn().
				Str("job", jobName).
				Str("expected", instanceID).
				Str("actual", job.LockedBy).
				Msg("Lock was taken by another instance")
			return nil
		}

		if success {
			job.LastRunAt = now
		}
		job.LockedAt = nil
		job.LockedBy = ""

		return tx.Save(&job).Error
	})
}

// SendRemindersWithRateLimit wraps SendReminders with distributed locking to
// prevent duplicate sends during rapid restarts. It returns the number of
// notification sends that succeeded and, for the job-run record (issue #391),
// ErrJobSkipped when the lock is held / it ran too recently — and a non-nil
// error when one or more notification sends failed (so a birthday reminder
// that silently fails to send now marks the reminder job run as failed, not a
// false success).
func SendRemindersWithRateLimit(db *gorm.DB, cfg config.Config) (int, error) {
	acquired, err := acquireJobLock(db, models.JobNameDailyReminders, ReminderMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("Error checking job lock")
		return 0, err
	}

	if !acquired {
		logger.Info().Msg("Skipping reminder job - rate limited")
		return 0, ErrJobSkipped
	}

	// Run the actual reminder logic
	sent, err := SendReminders(db, cfg)

	// Release the lock, marking success if no error
	if releaseErr := releaseJobLock(db, models.JobNameDailyReminders, err == nil); releaseErr != nil {
		logger.Error().Err(releaseErr).Msg("Error releasing job lock")
	}

	return sent, err
}

// notificationDeliveryKey identifies one (reminder, channel) pair in the
// delivered-state set SendReminders builds.
type notificationDeliveryKey struct {
	reminderID uint
	channel    string
}

// SendReminders dispatches due reminders across every configured notification
// channel. A reminder is due for a channel when NO NotificationDelivery row
// exists with that channel and status='sent' — a 'failed' or 'pending' row
// leaves it due, so a failure in one channel never marks the reminder as sent
// and never blocks another channel from dispatching. Email remains a per-user
// digest (it also carries birthdays); the push-style channels send per reminder.
//
// Returns the number of per-user-per-channel notification sends that
// succeeded, and a non-nil error when one or more sends failed — failure in
// one channel is still logged and does not abort the others, but it no longer
// vanishes: the aggregate error propagates so the reminder job run is recorded
// as failed (issue #391).
func SendReminders(db *gorm.DB, config config.Config) (int, error) {
	ctx := logger.JobContext(models.JobNameDailyReminders)
	logger.Info().Msg("Sending reminders...")
	var reminders []models.Reminder
	// Get the current time in the configured reminder timezone
	loc := config.GetReminderLocation()
	now := time.Now().In(loc)
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)

	// Fetch reminders that are due today or before and not completed. Per-channel
	// eligibility (ByMail for email, per-user toggles for the rest) is decided by
	// each channel sender, keyed off the delivery records below.
	if err := db.Where("remind_at <= ? AND completed = ?", endOfDay, false).Find(&reminders).Error; err != nil {
		return 0, fmt.Errorf("failed to fetch reminders: %w", err)
	}

	// Build the set of (reminder, channel) pairs already marked sent for this
	// occurrence.
	sentDeliveries := make(map[notificationDeliveryKey]bool)
	if len(reminders) > 0 {
		reminderIDs := make([]uint, 0, len(reminders))
		for _, r := range reminders {
			reminderIDs = append(reminderIDs, r.ID)
		}
		var deliveries []models.NotificationDelivery
		if err := db.Where("reminder_id IN ?", reminderIDs).Find(&deliveries).Error; err != nil {
			return 0, fmt.Errorf("failed to fetch notification deliveries: %w", err)
		}
		for _, d := range deliveries {
			if d.Status == "sent" {
				sentDeliveries[notificationDeliveryKey{reminderID: d.ReminderID, channel: d.Channel}] = true
			}
		}
	}

	// Group reminders by user
	remindersByUser := make(map[uint][]models.Reminder)
	for _, reminder := range reminders {
		remindersByUser[reminder.UserID] = append(remindersByUser[reminder.UserID], reminder)
	}

	// Collect user IDs from reminders
	userIDSet := make(map[uint]bool)
	for userID := range remindersByUser {
		userIDSet[userID] = true
	}

	// Also include users who have birthdays today (even without reminders)
	// Check all users and use GetUpcomingBirthdays - if first result is today, include them
	var allUsers []models.User
	if err := db.Find(&allUsers).Error; err != nil {
		logger.Warn().Err(err).Msg("Failed to fetch all users for birthday check, continuing with reminders only")
	} else {
		for _, user := range allUsers {
			if userIDSet[user.ID] {
				continue // Already included via reminders
			}
			birthdays, err := GetUpcomingBirthdays(db, user.ID, now)
			if err != nil {
				logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to fetch birthdays for user")
				continue
			}
			if len(birthdays) > 0 && DaysUntilBirthday(birthdays[0].Birthday, now) == 0 {
				userIDSet[user.ID] = true
			}
		}
	}

	// Convert set to slice
	userIDs := make([]uint, 0, len(userIDSet))
	for userID := range userIDSet {
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		logger.Info().Msg("No reminders or birthdays to send for today")
		return 0, nil
	}

	// Fetch all users we need to reach
	var users []models.User
	if err := db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return 0, fmt.Errorf("failed to fetch users: %w", err)
	}

	userByID := make(map[uint]models.User, len(users))
	for _, user := range users {
		userByID[user.ID] = user
	}

	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

	// Per-run send tallies for the job-run record (issue #391).
	var sendOK, sendFail int

	for _, userID := range userIDs {
		user, exists := userByID[userID]
		if !exists {
			logger.Warn().Uint("user_id", userID).Msg("Skipping user - not found")
			continue
		}

		userReminders := remindersByUser[userID] // May be nil/empty for birthday-only users

		// Birthdays falling today — the email digest includes them (and is the
		// only channel that runs for a birthday-only user), and they drive the
		// birthday.occurred webhooks below.
		var todayBirthdays []models.Birthday
		birthdays, err := GetUpcomingBirthdays(db, userID, now)
		if err != nil {
			logger.Warn().Err(err).Uint("user_id", userID).Msg("Failed to fetch birthdays for user")
		} else {
			for _, bday := range birthdays {
				if DaysUntilBirthday(bday.Birthday, now) == 0 {
					todayBirthdays = append(todayBirthdays, bday)
				}
			}
		}

		// Dispatch each enabled channel. Failure in one channel is logged and
		// does not abort the others.
		for _, sender := range notificationSenders {
			channel := sender.Channel()

			eligible := make([]models.Reminder, 0, len(userReminders))
			for _, r := range userReminders {
				if sentDeliveries[notificationDeliveryKey{reminderID: r.ID, channel: string(channel)}] {
					continue
				}
				// Legacy mirror: a reminder the pre-N9 code already emailed
				// (email_sent=true) must not be re-emailed even if its backfilled
				// delivery row is somehow missing. The other channels have no
				// legacy field — delivery rows are their only record.
				if channel == models.ChannelEmail && r.EmailSent {
					continue
				}
				eligible = append(eligible, r)
			}

			// Email is a per-user digest: it also carries birthdays, so it runs
			// even with no due reminders (but a birthday today). The push-style
			// channels only run when there is a reminder to say.
			if channel == models.ChannelEmail && len(eligible) == 0 && len(todayBirthdays) == 0 {
				continue
			}
			if channel != models.ChannelEmail && len(eligible) == 0 {
				continue
			}

			if !sender.Enabled(db, config, user) {
				continue
			}

			if err := sender.Send(ctx, db, config, user, eligible); err != nil {
				sendFail++
				logger.Error().Err(err).Uint("user_id", user.ID).Str("channel", string(channel)).Msg("Error sending notifications")
			} else {
				sendOK++
			}
		}

		// Fire reminder.triggered webhooks regardless of channel config
		for _, reminder := range userReminders {
			TriggerWebhooksAsync(ctx, db, config, reminder.UserID, "reminder.triggered", reminder)
		}

		// Fire birthday.occurred for each birthday that falls today regardless of channel config
		for _, bday := range todayBirthdays {
			bday := bday
			TriggerWebhooksAsync(ctx, db, config, userID, "birthday.occurred", bday)
		}
	}

	if sendFail > 0 {
		return sendOK, fmt.Errorf("%d notification send(s) failed", sendFail)
	}
	return sendOK, nil
}

// GetUpcomingReminders returns all of a user's incomplete reminders due
// within the next 7 days of `now` when that set exceeds five; otherwise it
// falls back to at least the next five upcoming reminders overall (so a
// quiet week doesn't show an empty dashboard). Shared by
// GetUpcomingReminders (reminder_controller.go) and the M3 dashboard
// composite (dashboard_controller.go) — one place for this rule so the two
// callers can never drift apart (M3's "reuse the exact semantics" trap).
func GetUpcomingReminders(db *gorm.DB, userID uint, now time.Time) ([]models.Reminder, error) {
	sevenDaysFromNow := now.AddDate(0, 0, 7)

	var remindersNext7Days []models.Reminder
	if err := db.Where("user_id = ? AND remind_at <= ? AND completed = ?", userID, sevenDaysFromNow, false).
		Order("remind_at ASC").
		Find(&remindersNext7Days).Error; err != nil {
		return nil, err
	}

	if len(remindersNext7Days) > 5 {
		return remindersNext7Days, nil
	}

	var remindersNext5 []models.Reminder
	if err := db.Where("user_id = ? AND completed = ?", userID, false).
		Order("remind_at ASC").
		Limit(5).
		Find(&remindersNext5).Error; err != nil {
		return nil, err
	}

	return remindersNext5, nil
}

// formatDateForUser formats a time.Time according to user's date format preference
func formatDateForUser(t time.Time, dateFormat string) string {
	switch dateFormat {
	case "us":
		return t.Format("01/02/2006") // MM/DD/YYYY
	case "iso":
		return t.Format("2006-01-02") // YYYY-MM-DD
	case "ca":
		return t.Format("02/01/2006") // DD/MM/YYYY
	case "eu-hyphen":
		return t.Format("02-01-2006") // DD-MM-YYYY
	case "us-mmm":
		return t.Format("Jan 2, 2006") // MMM D, YYYY
	case "us-mmmm":
		return t.Format("January 2, 2006") // MMMM D, YYYY
	case "eu-mmm":
		return t.Format("2 Jan, 2006") // D MMM, YYYY
	case "eu-mmmm":
		return t.Format("02 January, 2006") // DD MMMM, YYYY
	default:
		return t.Format("02.01.2006") // DD.MM.YYYY (EU default)
	}
}

// formatBirthdayForUser formats a birthday string (YYYY-MM-DD or --MM-DD) according to user's preference
func formatBirthdayForUser(birthday string, dateFormat string) string {
	if birthday == "" {
		return ""
	}

	// Handle year-unknown format: --MM-DD
	if len(birthday) >= 2 && birthday[:2] == "--" {
		if len(birthday) >= 7 {
			month := birthday[2:4]
			day := birthday[5:7]
			switch dateFormat {
			case "us":
				return month + "/" + day
			case "iso":
				return month + "-" + day
			case "ca":
				return day + "/" + month
			case "eu-hyphen":
				return day + "-" + month
			case "us-mmm":
				return shortMonthName(month) + " " + unpad(day)
			case "us-mmmm":
				return fullMonthName(month) + " " + unpad(day)
			case "eu-mmm":
				return unpad(day) + " " + shortMonthName(month)
			case "eu-mmmm":
				return day + " " + fullMonthName(month)
			default:
				return day + "." + month + "."
			}
		}
		return birthday
	}

	// Handle full date format: YYYY-MM-DD
	if len(birthday) >= 10 {
		year := birthday[0:4]
		month := birthday[5:7]
		day := birthday[8:10]

		switch dateFormat {
		case "us":
			return month + "/" + day + "/" + year
		case "iso":
			return year + "-" + month + "-" + day
		case "ca":
			return day + "/" + month + "/" + year
		case "eu-hyphen":
			return day + "-" + month + "-" + year
		case "us-mmm":
			return shortMonthName(month) + " " + unpad(day) + ", " + year
		case "us-mmmm":
			return fullMonthName(month) + " " + unpad(day) + ", " + year
		case "eu-mmm":
			return unpad(day) + " " + shortMonthName(month) + ", " + year
		case "eu-mmmm":
			return day + " " + fullMonthName(month) + ", " + year
		default:
			return day + "." + month + "." + year
		}
	}

	return birthday
}

// shortMonthName returns the English abbreviated month name for a "01".."12"
// month string. English by design — the month-name display formats the user
// picks (us-mmm / eu-mmm, etc.) are the en-US-style tokens, independent of
// the recipient's UI language.
func shortMonthName(month string) string {
	names := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	if i, err := strconv.Atoi(month); err == nil && i >= 1 && i <= 12 {
		return names[i-1]
	}
	return month
}

// fullMonthName returns the English full month name for a "01".."12" month
// string (see shortMonthName for why English).
func fullMonthName(month string) string {
	names := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if i, err := strconv.Atoi(month); err == nil && i >= 1 && i <= 12 {
		return names[i-1]
	}
	return month
}

// unpad strips a leading zero from a two-digit day string ("07" -> "7").
func unpad(s string) string {
	if len(s) == 2 && s[0] == '0' {
		return s[1:]
	}
	return s
}

// Send email using Resend with daily reminders and upcoming birthdays
func sendReminderEmail(user models.User, reminders []models.Reminder, config config.Config, db *gorm.DB) error {
	if user.Email == "" {
		logger.Warn().Uint("user_id", user.ID).Msg("Skipping reminder email because user email is missing")
		return nil
	}

	// Get user's language preference (default to "en" if not set)
	lang := user.Language
	if lang == "" {
		lang = i18n.DefaultLanguage
	}

	// Get user's date format preference (default to "eu" if not set)
	dateFormat := user.DateFormat
	if dateFormat == "" {
		dateFormat = "eu"
	}

	// Build reminder items — batch-fetch all referenced contacts first.
	contactIDs := make([]uint, 0, len(reminders))
	for _, r := range reminders {
		if r.ContactID != nil {
			contactIDs = append(contactIDs, *r.ContactID)
		}
	}
	contactMap := make(map[uint]string, len(contactIDs))
	if len(contactIDs) > 0 {
		var contacts []models.Contact
		if err := db.Where("user_id = ? AND id IN ?", user.ID, contactIDs).Find(&contacts).Error; err != nil {
			logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to batch-fetch contacts for reminder email")
		}
		for _, c := range contacts {
			contactMap[c.ID] = c.Firstname + " " + c.Lastname
		}
	}

	reminderItems := make([]ReminderItem, 0, len(reminders))
	for _, reminder := range reminders {
		contactName := i18n.T(lang, "email.reminder.unknownContact")
		if reminder.ContactID != nil {
			if name, ok := contactMap[*reminder.ContactID]; ok {
				contactName = name
			}
		}
		reminderItems = append(reminderItems, ReminderItem{
			Date:        formatDateForUser(reminder.RemindAt, dateFormat),
			Message:     reminder.Message,
			ContactName: contactName,
		})
	}

	// Build birthday items
	now := time.Now().In(config.GetReminderLocation())
	birthdays, birthdayErr := GetUpcomingBirthdays(db, user.ID, now)
	if birthdayErr != nil {
		logger.Warn().Err(birthdayErr).Uint("user_id", user.ID).Msg("Failed to fetch birthdays for email, continuing without them")
	}
	birthdayItems := make([]BirthdayItem, 0, len(birthdays))
	for _, birthday := range birthdays {
		days := DaysUntilBirthday(birthday.Birthday, now)
		var daysText, badgeType string
		switch days {
		case 0:
			daysText = i18n.T(lang, "email.reminder.today")
			badgeType = "today"
		case 1:
			daysText = i18n.T(lang, "email.reminder.tomorrow")
			badgeType = "tomorrow"
		default:
			daysText = i18n.T(lang, "email.reminder.inDays", map[string]string{"days": strconv.Itoa(days)})
			badgeType = "future"
		}
		birthdayItems = append(birthdayItems, BirthdayItem{
			FormattedDate: formatBirthdayForUser(birthday.Birthday, dateFormat),
			Name:          birthday.Name,
			DaysText:      daysText,
			BadgeType:     badgeType,
		})
	}

	htmlContent, err := renderReminderEmail(ReminderEmailData{
		RemindersTitle: i18n.T(lang, "email.reminder.remindersTitle"),
		BirthdaysTitle: i18n.T(lang, "email.reminder.birthdaysTitle"),
		ContactLabel:   i18n.T(lang, "email.reminder.contactLabel"),
		Footer:         i18n.T(lang, "email.footer"),
		Reminders:      reminderItems,
		Birthdays:      birthdayItems,
	})
	if err != nil {
		logger.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to render reminder email template")
		return err
	}

	logger.Debug().Int("reminder_count", len(reminderItems)).Int("birthday_count", len(birthdayItems)).Uint("user_id", user.ID).Str("language", lang).Msg("Sending reminder email")

	if err := SendEmail(config, EmailMessage{
		To:      user.Email,
		Subject: i18n.T(lang, "email.reminder.subject"),
		HTML:    htmlContent,
	}); err != nil {
		logger.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to send reminder email")
		return err
	}

	logger.Info().Uint("user_id", user.ID).Msg("Reminder email sent successfully")

	return nil
}

// addMonths adds the specified number of months to a date, clamping to the last
// valid day of the target month to handle edge cases like Jan 31 + 1 month -> Feb 28/29
func addMonths(t time.Time, months int) time.Time {
	// Get the original day of month
	originalDay := t.Day()

	// Add months using Go's AddDate (which may overflow into next month)
	result := t.AddDate(0, months, 0)

	// If the day changed unexpectedly (overflow occurred), clamp to last day of target month
	// For example: Jan 31 + 1 month = March 3 (in non-leap year), we want Feb 28
	if result.Day() != originalDay {
		// Go back to the last day of the previous month (the intended target month)
		result = result.AddDate(0, 0, -result.Day())
	}

	return result
}

// addYears adds the specified number of years to a date, handling Feb 29 edge case
func addYears(t time.Time, years int) time.Time {
	originalDay := t.Day()
	result := t.AddDate(years, 0, 0)

	// Handle Feb 29 -> Feb 28 transition for leap year edge case
	if result.Day() != originalDay {
		result = result.AddDate(0, 0, -result.Day())
	}

	return result
}

// CalculateNextReminderTime determines the next reminder date based on recurrence settings.
// All calculations are done in UTC to ensure consistency.
func CalculateNextReminderTime(reminder models.Reminder) time.Time {
	// Normalize to UTC for consistent calculations
	now := time.Now().UTC()
	remindAtUTC := reminder.RemindAt.UTC()

	var baseTime time.Time
	// Default to true if not specified (nil)
	reoccurFromCompletion := reminder.ReoccurFromCompletion == nil || *reminder.ReoccurFromCompletion
	if reoccurFromCompletion {
		if remindAtUTC.After(now) {
			// For reminders in the future, use the original remind at time (e.g. if I already complete a monthly reminder set for next week I am reminded again next week in one month)
			baseTime = remindAtUTC
		} else {
			// For reminders in the past use now as reference (if I complete a weekly reminder that was due last week, the next reminder is in one week from today)
			baseTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}
	} else {
		baseTime = remindAtUTC
	}

	switch reminder.Recurrence {
	case "once":
		// Will be deleted anyway
		return reminder.RemindAt
	case "weekly":
		return baseTime.AddDate(0, 0, 7)
	case "monthly":
		return addMonths(baseTime, 1)
	case "quarterly":
		return addMonths(baseTime, 3)
	case "six-months":
		return addMonths(baseTime, 6)
	case "yearly":
		return addYears(baseTime, 1)
	default:
		// If the recurrence type is unrecognized, return the original RemindAt
		logger.Warn().Str("recurrence", reminder.Recurrence).Uint("reminder_id", reminder.ID).Msg("Unrecognized recurrence type")
		return reminder.RemindAt
	}
}
