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

func setupDataDecayServiceTestDB(t *testing.T) (*gorm.DB, models.User, models.Contact) {
	t.Helper()

	db := dbtest.New(t)

	user := models.User{Username: "decay-tester", Password: "x", Email: "decay@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

// TestComputeDataDecayHealth_NeverVerifiedUsesCreatedAtAsBaseline pins the
// one deliberate divergence from CadenceHealth: unlike a cadence policy with
// no qualifying interaction (undefined health), a data-decay policy always
// has a baseline -- CreatedAt when LastVerifiedAt is nil -- so NextDue is
// always defined.
func TestComputeDataDecayHealth_NeverVerifiedUsesCreatedAtAsBaseline(t *testing.T) {
	created := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	policy := models.DataDecayPolicy{CreatedAt: created, IntervalDays: 30}
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

	health := ComputeDataDecayHealth(&policy, now)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"))
	// Due today: not overdue.
	assert.Zero(t, health.OverdueBy)
}

func TestComputeDataDecayHealth_JustVerifiedIsNotOverdue(t *testing.T) {
	created := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	verified := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	policy := models.DataDecayPolicy{CreatedAt: created, LastVerifiedAt: &verified, IntervalDays: 365}
	now := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	health := ComputeDataDecayHealth(&policy, now)
	assert.Equal(t, "2027-01-15", health.NextDue.Format("2006-01-02"))
	assert.Zero(t, health.OverdueBy)
}

func TestComputeDataDecayHealth_OverdueByN(t *testing.T) {
	verified := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	policy := models.DataDecayPolicy{CreatedAt: verified, LastVerifiedAt: &verified, IntervalDays: 30}
	// Next due Jan 31; now is Feb 5 -- 5 days overdue.
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)

	health := ComputeDataDecayHealth(&policy, now)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"))
	assert.Equal(t, 5, health.OverdueBy)
}

// TestComputeDataDecayHealth_DueTodayIsNotOverdue pins the exact interval
// boundary: due today is defined but not overdue, matching CadenceHealth's
// "due today" convention.
func TestComputeDataDecayHealth_DueTodayIsNotOverdue(t *testing.T) {
	verified := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	policy := models.DataDecayPolicy{CreatedAt: verified, LastVerifiedAt: &verified, IntervalDays: 30}
	now := time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC)

	health := ComputeDataDecayHealth(&policy, now)
	assert.Zero(t, health.OverdueBy, "due today must not count as overdue")
}

func TestComputeDataDecayHealth_OneDayPastDueIsOverdueByOne(t *testing.T) {
	verified := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	policy := models.DataDecayPolicy{CreatedAt: verified, LastVerifiedAt: &verified, IntervalDays: 30}
	now := time.Date(2026, 2, 1, 0, 0, 1, 0, time.UTC)

	health := ComputeDataDecayHealth(&policy, now)
	assert.Equal(t, 1, health.OverdueBy)
}

func TestListOverdueDataDecayPolicies_OnlyOverdueAndActivePolicies(t *testing.T) {
	db, user, contact := setupDataDecayServiceTestDB(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	overdueVerified := now.AddDate(0, 0, -40)
	overdue := models.DataDecayPolicy{
		UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 30,
		LastVerifiedAt: &overdueVerified, Active: true,
	}
	require.NoError(t, db.Create(&overdue).Error)

	otherContact := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	freshVerified := now.AddDate(0, 0, -1)
	notOverdue := models.DataDecayPolicy{
		UserID: user.ID, EntityID: otherContact.VCardUID, IntervalDays: 30,
		LastVerifiedAt: &freshVerified, Active: true,
	}
	require.NoError(t, db.Create(&notOverdue).Error)

	pausedContact := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&pausedContact).Error)
	pausedVerified := now.AddDate(0, 0, -400)
	paused := models.DataDecayPolicy{
		UserID: user.ID, EntityID: pausedContact.VCardUID, IntervalDays: 30,
		LastVerifiedAt: &pausedVerified, Active: false,
	}
	require.NoError(t, db.Create(&paused).Error)

	result, err := ListOverdueDataDecayPolicies(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, result, 1, "only the active, overdue policy must appear -- not the fresh one, not the paused one")
	assert.Equal(t, overdue.ID, result[0].Policy.ID)
	assert.Equal(t, contact.ID, result[0].ContactID)
	assert.Equal(t, "Alice", result[0].ContactName)
}

func TestListOverdueDataDecayPolicies_SortsMostOverdueFirst(t *testing.T) {
	db, user, contact := setupDataDecayServiceTestDB(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	slightlyOverdueVerified := now.AddDate(0, 0, -31)
	slightly := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 30, LastVerifiedAt: &slightlyOverdueVerified, Active: true}
	require.NoError(t, db.Create(&slightly).Error)

	otherContact := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	veryOverdueVerified := now.AddDate(0, 0, -100)
	very := models.DataDecayPolicy{UserID: user.ID, EntityID: otherContact.VCardUID, IntervalDays: 30, LastVerifiedAt: &veryOverdueVerified, Active: true}
	require.NoError(t, db.Create(&very).Error)

	result, err := ListOverdueDataDecayPolicies(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, very.ID, result[0].Policy.ID, "the most-overdue policy must be first")
	assert.Equal(t, slightly.ID, result[1].Policy.ID)
}

func TestListOverdueDataDecayPolicies_ScopedToUser(t *testing.T) {
	db, user, _ := setupDataDecayServiceTestDB(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Mine"}
	require.NoError(t, db.Create(&otherContact).Error)
	overdueVerified := now.AddDate(0, 0, -400)
	require.NoError(t, db.Create(&models.DataDecayPolicy{
		UserID: otherUser.ID, EntityID: otherContact.VCardUID, IntervalDays: 30,
		LastVerifiedAt: &overdueVerified, Active: true,
	}).Error)

	result, err := ListOverdueDataDecayPolicies(db, user.ID, now)
	require.NoError(t, err)
	assert.Empty(t, result, "another user's overdue policy must never leak into this user's list")
}
