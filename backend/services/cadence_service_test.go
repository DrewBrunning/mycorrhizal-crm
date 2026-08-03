package services

import (
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCadenceServiceTestDB(t *testing.T) (*gorm.DB, models.User, models.Contact) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.CadencePolicy{}))

	user := models.User{Username: "cadence-tester", Password: "x", Email: "cadence@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

// seedActivity attaches one activity (with the given type and date) to the
// contact via the many2many join, mirroring how CreateActivity writes.
func seedActivity(t *testing.T, db *gorm.DB, user models.User, contact models.Contact, typ string, date time.Time) {
	t.Helper()
	activity := models.Activity{
		UserID: user.ID, Title: "interaction", Date: date, Type: typ,
		Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)
}

func TestCalendarDaysBetween(t *testing.T) {
	loc := time.UTC
	today := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)

	t.Run("positive gap", func(t *testing.T) {
		got := calendarDaysBetween(today, time.Date(2026, 1, 11, 0, 0, 0, 0, loc))
		assert.Equal(t, 10, got)
	})

	t.Run("negative gap", func(t *testing.T) {
		got := calendarDaysBetween(today, time.Date(2025, 12, 25, 0, 0, 0, 0, loc))
		assert.Equal(t, -7, got)
	})

	t.Run("same day is zero regardless of clock time", func(t *testing.T) {
		late := time.Date(2026, 1, 1, 23, 59, 59, 0, loc)
		assert.Equal(t, 0, calendarDaysBetween(today, late))
	})

	t.Run("year boundary wrap", func(t *testing.T) {
		// Dec 31 -> Jan 1 must be exactly 1 day, not 0 (the classic
		// off-by-one when the year flips mid-interval).
		assert.Equal(t, 1, calendarDaysBetween(
			time.Date(2025, 12, 31, 0, 0, 0, 0, loc),
			time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		))
	})

	t.Run("DST boundary does not lose a day", func(t *testing.T) {
		// Berlin springs forward 2026-03-29 (23h day): two local midnights
		// are only 23 absolute hours apart, so truncation would say 0 days.
		berlin, err := time.LoadLocation("Europe/Berlin")
		require.NoError(t, err)
		assert.Equal(t, 1, calendarDaysBetween(
			time.Date(2026, 3, 29, 0, 0, 0, 0, berlin),
			time.Date(2026, 3, 30, 0, 0, 0, 0, berlin),
		))
	})
}

func TestComputeCadenceHealth_NoQualifyingInteractionIsUndefined(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	assert.False(t, health.HasQualifyingInteraction)
	assert.Nil(t, health.LastInteraction)
	assert.Nil(t, health.NextDue)
	assert.Zero(t, health.OverdueBy)
}

func TestComputeCadenceHealth_QualifyingInteractionResetsCadence(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	// Last qualifying interaction on Jan 1; interval 30 -> next due Jan 31.
	seedActivity(t, db, user, contact, models.InteractionTypeCall,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.True(t, health.HasQualifyingInteraction)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"))
	// Due today: not overdue.
	assert.Zero(t, health.OverdueBy)
}

func TestComputeCadenceHealth_MostRecentQualifyingInteractionWins(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	// Two qualifying interactions; the most recent must be the one that
	// anchors next_due.
	seedActivity(t, db, user, contact, models.InteractionTypeVisit,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	seedActivity(t, db, user, contact, models.InteractionTypeMeal,
		time.Date(2026, 1, 10, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-02-09", health.NextDue.Format("2006-01-02"), "next due anchored to Jan 10 + 30 days")
}

func TestComputeCadenceHealth_NonQualifyingInteractionDoesNotReset(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	// photo is the only globally non-qualifying type. A wall of recent photo
	// shares must NOT move next_due — this is the behavioral core of "task
	// completion does not reset cadence": only a qualifying interaction does.
	seedActivity(t, db, user, contact, models.InteractionTypeCall,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	seedActivity(t, db, user, contact, models.InteractionTypePhoto,
		time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC))
	seedActivity(t, db, user, contact, models.InteractionTypePhoto,
		time.Date(2026, 1, 25, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"),
		"the two photo interactions must be ignored; next due stays Jan 1 + 30")
	assert.Zero(t, health.OverdueBy)
}

func TestComputeCadenceHealth_OverdueAcrossYearBoundary(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	// Last qualifying interaction Dec 30 2025; interval 30 -> next due
	// Jan 29 2026. The year wrap must not corrupt the overdue computation.
	seedActivity(t, db, user, contact, models.InteractionTypeVisit,
		time.Date(2025, 12, 30, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-29", health.NextDue.Format("2006-01-02"))
	assert.Equal(t, 2, health.OverdueBy, "Jan 31 is 2 whole days past the Jan 29 due date")
}

func TestComputeCadenceHealth_ExplicitQualifyingTypesFilter(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	seedActivity(t, db, user, contact, models.InteractionTypeCall,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	// meal is qualifying globally but NOT in this policy's list, so it must
	// not reset the cadence.
	seedActivity(t, db, user, contact, models.InteractionTypeMeal,
		time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{
		UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30,
		QualifyingTypes: []string{models.InteractionTypeCall},
	}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"),
		"the unlisted meal must be ignored; next due stays Jan 1 + 30")
	assert.Zero(t, health.OverdueBy)
}

func TestComputeCadenceHealth_ScopedToUser(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	// Another user's qualifying interaction on the same-named contact's uid
	// must not leak into this user's timeline.
	seedActivity(t, db, otherUser, contact, models.InteractionTypeCall,
		time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	assert.False(t, health.HasQualifyingInteraction,
		"the other user's activity must not be visible to this user")
}

func TestListOverdueCadences_OnlyOverduePolicies(t *testing.T) {
	db, user, _ := setupCadenceServiceTestDB(t)
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	// Alice: last interaction Jan 1, 30-day interval -> due Jan 31, not overdue.
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)
	seedActivity(t, db, user, alice, models.InteractionTypeCall,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	require.NoError(t, db.Create(&models.CadencePolicy{
		UserID: user.ID, EntityID: alice.VCardUID, TargetIntervalDays: 30,
	}).Error)

	// Bob: last interaction Dec 15, 30-day interval -> due Jan 14, 17 days overdue.
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&bob).Error)
	seedActivity(t, db, user, bob, models.InteractionTypeCall,
		time.Date(2025, 12, 15, 10, 0, 0, 0, time.UTC))
	require.NoError(t, db.Create(&models.CadencePolicy{
		UserID: user.ID, EntityID: bob.VCardUID, TargetIntervalDays: 30,
	}).Error)

	// Carol: policy with NO qualifying interaction ever -> never overdue.
	carol := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&carol).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{
		UserID: user.ID, EntityID: carol.VCardUID, TargetIntervalDays: 30,
	}).Error)

	overdue, err := ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, overdue, 1)
	assert.Equal(t, bob.VCardUID, overdue[0].Policy.EntityID)
	assert.Equal(t, 17, overdue[0].Health.OverdueBy)
	assert.Equal(t, bob.ID, overdue[0].ContactID)
	assert.Equal(t, "Bob", overdue[0].ContactName)
}

func TestListOverdueCadences_SortsMostOverdueFirst(t *testing.T) {
	db, user, _ := setupCadenceServiceTestDB(t)
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	type seed struct {
		name  string
		last  string
		days  int
		hours int
	}
	seeds := []seed{
		{"Mildly", "2026-01-01", 10, 0},  // due Jan 11 -> 20 days overdue
		{"Heavily", "2025-11-01", 10, 0}, // due Nov 11 -> 81 days overdue
		{"Fresh", "2026-01-20", 10, 0},   // due Jan 30 -> 1 day overdue
	}
	for _, s := range seeds {
		c := models.Contact{UserID: user.ID, Firstname: s.name}
		require.NoError(t, db.Create(&c).Error)
		last, err := time.Parse("2006-01-02", s.last)
		require.NoError(t, err)
		seedActivity(t, db, user, c, models.InteractionTypeVisit, last.Add(time.Duration(s.hours)*time.Hour))
		require.NoError(t, db.Create(&models.CadencePolicy{
			UserID: user.ID, EntityID: c.VCardUID, TargetIntervalDays: s.days,
		}).Error)
	}

	overdue, err := ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, overdue, 3)
	assert.Equal(t, "Heavily", overdue[0].ContactName)
	assert.Equal(t, "Mildly", overdue[1].ContactName)
	assert.Equal(t, "Fresh", overdue[2].ContactName)
}

// TestComputeCadenceHealth_TimezoneNormalisation verifies that deriveHealth
// (and therefore ComputeCadenceHealth) normalises the activity date to
// midnight in the user's timezone before computing next_due. Without this, an
// activity stored in UTC whose wall-clock date differs from the local date
// (e.g. 20:00 UTC on Dec 31 = 09:00 Jan 01 in NZ) would carry the wrong
// next_due, and overdue_by would be off by one.
func TestComputeCadenceHealth_TimezoneNormalisation(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)

	// Activity stored as 2025-12-31 20:00 UTC, which in NZ (+13) is
	// 2026-01-01 09:00 — a different wall-clock date. next_due must be
	// computed from Jan 01 NZ, not Dec 31 UTC.
	nz, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)

	seedActivity(t, db, user, contact, models.InteractionTypeCall,
		time.Date(2025, 12, 31, 20, 0, 0, 0, time.UTC))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, nz) // Feb 01 NZ, 30 days past Jan 01

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"),
		"next_due anchored to NZ Jan 01 (not UTC Dec 31) + 30 = NZ Jan 31")
	assert.Equal(t, 1, health.OverdueBy, "Feb 01 is 1 day past Jan 31 due in NZ")
}
