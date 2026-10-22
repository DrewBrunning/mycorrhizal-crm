package services

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// targetNow is the pinned "now" every target_behind test runs at (clock.Fake
// via useFakeClock, never the wall clock).
var targetNow = time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)

type targetEnv struct {
	db   *gorm.DB
	cfg  config.Config
	user models.User
}

func newTargetEnv(t *testing.T) *targetEnv {
	t.Helper()
	useFakeClock(t, targetNow)
	db := setupReachOutTestDB(t)
	user := models.User{Username: "targetuser", Password: "password123!A", Email: "target@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return &targetEnv{db: db, cfg: config.Config{ReminderTimezone: "UTC"}, user: user}
}

func (e *targetEnv) contact(t *testing.T, first, tier string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: e.user.ID, Firstname: first, Lastname: "Target"}
	if tier != "" {
		c.TargetCloseness = &tier
	}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *targetEnv) activity(t *testing.T, c models.Contact, daysAgo int) time.Time {
	t.Helper()
	at := targetNow.AddDate(0, 0, -daysAgo)
	createScoreTestActivity(t, e.db, e.user.ID, c, models.InteractionTypeCall, at)
	return at
}

func (e *targetEnv) run(t *testing.T) int {
	t.Helper()
	n, err := detectTargetBehindSuggestions(context.Background(), e.db, e.cfg, Now())
	require.NoError(t, err)
	return n
}

func (e *targetEnv) rows(t *testing.T) []models.ReachOutSuggestion {
	t.Helper()
	var rows []models.ReachOutSuggestion
	require.NoError(t, e.db.Where("kind = ?", models.ReachOutKindTargetBehind).Order("created_at ASC, id ASC").Find(&rows).Error)
	return rows
}

func (e *targetEnv) status(t *testing.T, id string) string {
	t.Helper()
	var s models.ReachOutSuggestion
	require.NoError(t, e.db.First(&s, "id = ?", id).Error)
	return s.Status
}

func TestTargetBehind_CreatesOneNudgePerGap(t *testing.T) {
	e := newTargetEnv(t)
	c := e.contact(t, "Alice", "close")
	last := e.activity(t, c, 40)

	assert.Equal(t, 1, e.run(t))
	rows := e.rows(t)
	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, e.user.ID, row.UserID)
	assert.Equal(t, c.VCardUID, row.ContactVCardUID)
	assert.Equal(t, "close", row.OldValue)
	assert.Equal(t, last.UTC().Format(time.RFC3339), row.NewValue)
	assert.Zero(t, row.AuditEventID)
	assert.Equal(t, models.ReachOutStatusPending, row.Status)
	require.NotNil(t, row.ReminderID)

	var reminder models.Reminder
	require.NoError(t, e.db.First(&reminder, *row.ReminderID).Error)
	assert.Equal(t, "Reach out — behind your close target: Alice Target (last interaction 2030-05-06)", reminder.Message)
	assert.Equal(t, "once", reminder.Recurrence)

	// Re-running the job on the same gap never doubles up.
	assert.Zero(t, e.run(t))
	assert.Len(t, e.rows(t), 1)
}

func TestTargetBehind_DismissedGapStaysQuietUntilANewGap(t *testing.T) {
	e := newTargetEnv(t)
	c := e.contact(t, "Alice", "close")
	e.activity(t, c, 40)
	require.Equal(t, 1, e.run(t))
	first := e.rows(t)[0]

	require.NoError(t, DismissReachOutSuggestion(e.db, e.user.ID, first.ID))
	assert.Zero(t, e.run(t), "a dismissed gap never nudges again")
	assert.Len(t, e.rows(t), 1)

	// A newer (still too old) interaction starts a new gap with a new key.
	e.activity(t, c, 35)
	assert.Equal(t, 1, e.run(t))
	rows := e.rows(t)
	require.Len(t, rows, 2)
	assert.Equal(t, models.ReachOutStatusDismissed, e.status(t, first.ID))

	// Changing the tier also starts a new gap: the old pending row is stale
	// (auto-dismissed) and a new one is created for the new tier.
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("target_closeness", "immediate").Error)
	assert.Equal(t, 1, e.run(t))
	var pendingTiers []string
	require.NoError(t, e.db.Model(&models.ReachOutSuggestion{}).
		Where("kind = ? AND status = ?", models.ReachOutKindTargetBehind, models.ReachOutStatusPending).
		Pluck("old_value", &pendingTiers).Error)
	assert.Equal(t, []string{"immediate"}, pendingTiers)
}

func TestTargetBehind_Skips(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, e *targetEnv)
	}{
		{"cadence policy", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "close")
			e.activity(t, c, 40)
			require.NoError(t, e.db.Create(&models.CadencePolicy{UserID: e.user.ID, EntityID: c.VCardUID, TargetIntervalDays: 30}).Error)
		}},
		{"ambient flag", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "close")
			e.activity(t, c, 40)
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("ambient", true).Error)
		}},
		{"no interaction ever", func(t *testing.T, e *targetEnv) {
			e.contact(t, "A", "close")
		}},
		{"deceased", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "close")
			e.activity(t, c, 40)
			models.ApplyRecordToContact(&c, &contactmodel.Record{Card: contactmodel.Card{
				Anniversaries: []contactmodel.Anniversary{{Kind: "death", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{
					Year: intPtr(2020), Month: intPtr(5), Day: intPtr(1)}}}},
			}}, "")
			c.TargetCloseness = strPtr("close")
			require.NoError(t, e.db.Save(&c).Error)
		}},
		{"archived", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "close")
			e.activity(t, c, 40)
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("archived", true).Error)
		}},
		{"on track", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "close")
			e.activity(t, c, 3)
		}},
		{"no target", func(t *testing.T, e *targetEnv) {
			c := e.contact(t, "A", "")
			e.activity(t, c, 400)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTargetEnv(t)
			tc.setup(t, e)
			assert.Zero(t, e.run(t))
			assert.Empty(t, e.rows(t))
		})
	}
}

func TestTargetBehind_AutoDismissesWhenTheGapNoLongerHolds(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, e *targetEnv, c models.Contact)
	}{
		{"new interaction today", func(t *testing.T, e *targetEnv, c models.Contact) { e.activity(t, c, 0) }},
		{"cadence policy created", func(t *testing.T, e *targetEnv, c models.Contact) {
			require.NoError(t, e.db.Create(&models.CadencePolicy{UserID: e.user.ID, EntityID: c.VCardUID, TargetIntervalDays: 30}).Error)
		}},
		{"target cleared", func(t *testing.T, e *targetEnv, c models.Contact) {
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("target_closeness", nil).Error)
		}},
		{"archived", func(t *testing.T, e *targetEnv, c models.Contact) {
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("archived", true).Error)
		}},
		{"became ambient", func(t *testing.T, e *targetEnv, c models.Contact) {
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("ambient", true).Error)
		}},
		{"contact deleted", func(t *testing.T, e *targetEnv, c models.Contact) {
			require.NoError(t, e.db.Delete(&models.Contact{}, c.ID).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTargetEnv(t)
			c := e.contact(t, "Alice", "close")
			e.activity(t, c, 40)
			require.Equal(t, 1, e.run(t))
			row := e.rows(t)[0]
			var reminderBefore models.Reminder
			require.NoError(t, e.db.First(&reminderBefore, *row.ReminderID).Error)

			tc.change(t, e, c)
			e.run(t)
			assert.Equal(t, models.ReachOutStatusDismissed, e.status(t, row.ID))

			// The companion Reminder is left exactly as it was.
			var reminderAfter models.Reminder
			require.NoError(t, e.db.First(&reminderAfter, *row.ReminderID).Error)
			assert.Equal(t, reminderBefore.Message, reminderAfter.Message)
			assert.Equal(t, reminderBefore.Completed, reminderAfter.Completed)
		})
	}
}

func TestTargetBehind_NeverTouchesAnotherUser(t *testing.T) {
	e := newTargetEnv(t)
	mine := e.contact(t, "Mine", "close")
	e.activity(t, mine, 40)

	other := models.User{Username: "otheruser", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs", TargetCloseness: strPtr("casual")}
	require.NoError(t, e.db.Create(&theirs).Error)
	createScoreTestActivity(t, e.db, other.ID, theirs, models.InteractionTypeCall, targetNow.AddDate(0, 0, -120))

	assert.Equal(t, 2, e.run(t))
	byUser := map[uint]models.ReachOutSuggestion{}
	for _, r := range e.rows(t) {
		byUser[r.UserID] = r
	}
	require.Len(t, byUser, 2)
	assert.Equal(t, mine.VCardUID, byUser[e.user.ID].ContactVCardUID)
	assert.Equal(t, theirs.VCardUID, byUser[other.ID].ContactVCardUID)
	assert.Equal(t, "casual", byUser[other.ID].OldValue)

	// Clearing only the other user's target dismisses only their row.
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", theirs.ID).Update("target_closeness", nil).Error)
	e.run(t)
	assert.Equal(t, models.ReachOutStatusPending, e.status(t, byUser[e.user.ID].ID))
	assert.Equal(t, models.ReachOutStatusDismissed, e.status(t, byUser[other.ID].ID))
}

func TestDismissPendingTargetBehindSuggestions_OnlyThatContactAndKind(t *testing.T) {
	e := newTargetEnv(t)
	a := e.contact(t, "A", "close")
	b := e.contact(t, "B", "close")
	e.activity(t, a, 40)
	e.activity(t, b, 40)
	require.Equal(t, 2, e.run(t))
	// A pending org suggestion for the same contact is a different kind.
	org := models.ReachOutSuggestion{UserID: e.user.ID, ContactVCardUID: a.VCardUID, Kind: models.ReachOutKindOrganization, NewValue: "X", Status: models.ReachOutStatusPending}
	require.NoError(t, e.db.Create(&org).Error)

	require.NoError(t, DismissPendingTargetBehindSuggestions(e.db, e.user.ID, a.VCardUID))

	for _, r := range e.rows(t) {
		if r.ContactVCardUID == a.VCardUID {
			assert.Equal(t, models.ReachOutStatusDismissed, r.Status)
		} else {
			assert.Equal(t, models.ReachOutStatusPending, r.Status)
		}
	}
	assert.Equal(t, models.ReachOutStatusPending, e.status(t, org.ID))

	// Wrong user: nothing changes.
	require.NoError(t, DismissPendingTargetBehindSuggestions(e.db, e.user.ID+99, b.VCardUID))
	for _, r := range e.rows(t) {
		if r.ContactVCardUID == b.VCardUID {
			assert.Equal(t, models.ReachOutStatusPending, r.Status)
		}
	}
}

func TestTargetBehind_DBErrorsSurface(t *testing.T) {
	for _, table := range []string{"contacts", "reach_out_suggestions", "activities", "reminders"} {
		t.Run(table, func(t *testing.T) {
			e := newTargetEnv(t)
			logtest.AllowWarnings(t, "the pass logs the failure it returns")
			c := e.contact(t, "Alice", "close")
			e.activity(t, c, 40)
			dbtest.HideTable(t, e.db, table)
			_, err := detectTargetBehindSuggestions(context.Background(), e.db, e.cfg, Now())
			assert.Error(t, err)
		})
	}

	t.Run("dismiss", func(t *testing.T) {
		e := newTargetEnv(t)
		dbtest.HideTable(t, e.db, "reach_out_suggestions")
		assert.Error(t, DismissPendingTargetBehindSuggestions(e.db, e.user.ID, "x"))
	})
}

// The job entry point runs the target pass under the same lock and counts its
// rows; a failing pass is logged and never discards the audit pass's count.
func TestDetectReachOutSuggestions_IncludesTargetPass(t *testing.T) {
	e := newTargetEnv(t)
	c := e.contact(t, "Alice", "close")
	e.activity(t, c, 40)

	n, err := DetectReachOutSuggestions(e.db, e.cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Len(t, e.rows(t), 1)
}

func TestDetectReachOutSuggestions_TargetPassFailureIsLoggedNotFatal(t *testing.T) {
	e := newTargetEnv(t)
	logtest.AllowWarnings(t, "the target pass failure is logged by design")
	c := e.contact(t, "Alice", "close")
	e.activity(t, c, 40)
	dbtest.HideTable(t, e.db, "reach_out_suggestions")

	// The audit pass also reads reach_out_cursors only, so the job itself still
	// completes; the hidden table makes just the target pass fail.
	_, err := DetectReachOutSuggestions(e.db, e.cfg)
	require.NoError(t, err)
}

// The reminder's "last interaction" date is the activity's calendar day, so it
// is formatted from the stored UTC date in every reminder zone (ADR 0015
// category 2, issue #1823). Kiritimati (UTC+14) used to move 2030-05-06 12:00
// UTC onto the 7th; the dedup key (new_value) stays RFC3339 UTC.
func TestTargetBehind_ReminderDateUsesActivityUTCDate(t *testing.T) {
	for _, zone := range []string{"America/Chicago", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			e := newTargetEnv(t)
			e.cfg.ReminderTimezone = zone
			c := e.contact(t, "Alice", "close")
			last := time.Date(2030, 5, 6, 0, 0, 0, 0, time.UTC)
			createScoreTestActivity(t, e.db, e.user.ID, c, models.InteractionTypeCall, last)

			require.Equal(t, 1, e.run(t))
			row := e.rows(t)[0]
			assert.Equal(t, last.UTC().Format(time.RFC3339), row.NewValue)
			var reminder models.Reminder
			require.NoError(t, e.db.First(&reminder, *row.ReminderID).Error)
			assert.Equal(t, "Reach out — behind your close target: Alice Target (last interaction 2030-05-06)", reminder.Message)
		})
	}
}
