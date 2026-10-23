package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
)

// Pre-amendment gift dates were instants (local midnight, or "now" for mark
// given). The one-shot backfill rewrites each to the calendar day it meant in
// the reminder zone, at UTC midnight -- including soft-deleted rows, which an
// undo can restore -- and leaves calendar days and dateless ideas alone.
func TestNormalizeGiftDatesToCalendarDays(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db := dbtest.New(t)
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	user := models.User{Username: "giftbf", Password: "password123!A", Email: "giftbf@example.com"}
	require.NoError(t, db.Create(&user).Error)

	seed := func(date *time.Time, deleted bool) models.Gift {
		g := models.Gift{UserID: user.ID, EntityID: "c-1", Status: models.GiftStatusGiven, Description: "x", Date: date}
		require.NoError(t, db.Create(&g).Error)
		if deleted {
			require.NoError(t, db.Delete(&g).Error)
		}
		return g
	}
	at := func(tm time.Time) *time.Time { return &tm }
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)

	localMidnight := seed(at(time.Date(2026, 10, 21, 22, 0, 0, 0, time.UTC)), false) // Berlin 22 Oct 00:00
	markedGiven := seed(at(time.Date(2026, 10, 22, 21, 45, 0, 0, time.UTC)), false)  // Berlin 22 Oct 23:45
	softDeleted := seed(at(time.Date(2026, 10, 21, 22, 0, 0, 0, time.UTC)), true)
	alreadyDay := seed(at(day), false)
	idea := seed(nil, false)

	var before models.Gift
	require.NoError(t, db.Unscoped().First(&before, "id = ?", alreadyDay.ID).Error)

	stats, err := NormalizeGiftDatesToCalendarDays(db, berlin)
	require.NoError(t, err)
	assert.EqualValues(t, 4, stats.GiftsScanned, "every dated row, soft-deleted included")
	assert.EqualValues(t, 3, stats.GiftsUpdated)

	for _, g := range []models.Gift{localMidnight, markedGiven, softDeleted, alreadyDay} {
		var got models.Gift
		require.NoError(t, db.Unscoped().First(&got, "id = ?", g.ID).Error)
		require.NotNil(t, got.Date)
		assert.True(t, day.Equal(*got.Date), "gift %s: got %s", g.ID, got.Date)
	}
	var unchanged models.Gift
	require.NoError(t, db.Unscoped().First(&unchanged, "id = ?", alreadyDay.ID).Error)
	assert.True(t, before.UpdatedAt.Equal(unchanged.UpdatedAt), "a row already in shape is not rewritten (no feed churn)")
	var stillIdea models.Gift
	require.NoError(t, db.First(&stillIdea, "id = ?", idea.ID).Error)
	assert.Nil(t, stillIdea.Date)

	// Once per database: the ledger short-circuits a second boot, even if a
	// stray instant appears afterwards (later writes normalize on the way in).
	seed(at(time.Date(2026, 10, 21, 22, 0, 0, 0, time.UTC)), false)
	again, err := NormalizeGiftDatesToCalendarDays(db, berlin)
	require.NoError(t, err)
	assert.True(t, again.AlreadyDone)
	assert.Zero(t, again.GiftsUpdated)
}

func TestNormalizeGiftDatesToCalendarDays_EdgeBranches(t *testing.T) {
	t.Run("nil db is a no-op", func(t *testing.T) {
		logtest.Guard(t)
		stats, err := NormalizeGiftDatesToCalendarDays(nil, nil)
		require.NoError(t, err)
		assert.NotNil(t, stats)
	})
	t.Run("nil zone means UTC", func(t *testing.T) {
		logtest.Guard(t)
		db := dbtest.New(t)
		user := models.User{Username: "giftbf2", Password: "password123!A", Email: "giftbf2@example.com"}
		require.NoError(t, db.Create(&user).Error)
		d := time.Date(2026, 10, 22, 21, 0, 0, 0, time.UTC)
		g := models.Gift{UserID: user.ID, EntityID: "c-1", Status: models.GiftStatusGiven, Description: "x", Date: &d}
		require.NoError(t, db.Create(&g).Error)
		_, err := NormalizeGiftDatesToCalendarDays(db, nil)
		require.NoError(t, err)
		var got models.Gift
		require.NoError(t, db.First(&got, "id = ?", g.ID).Error)
		assert.True(t, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC).Equal(*got.Date))
	})
	t.Run("missing ledger fails closed", func(t *testing.T) {
		logtest.Guard(t)
		db := dbtest.New(t)
		require.NoError(t, db.Exec("DROP TABLE data_backfills").Error)
		_, err := NormalizeGiftDatesToCalendarDays(db, time.UTC)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_backfills")
	})
	failingWrite := func(t *testing.T, trigger string) error {
		t.Helper()
		db := dbtest.New(t)
		user := models.User{Username: "giftbf3", Password: "password123!A", Email: "giftbf3@example.com"}
		require.NoError(t, db.Create(&user).Error)
		d := time.Date(2026, 10, 22, 21, 0, 0, 0, time.UTC)
		require.NoError(t, db.Create(&models.Gift{UserID: user.ID, EntityID: "c-1", Status: models.GiftStatusGiven, Description: "x", Date: &d}).Error)
		require.NoError(t, db.Exec(trigger).Error)
		_, err := NormalizeGiftDatesToCalendarDays(db, time.UTC)
		return err
	}
	t.Run("a failed row update aborts before the ledger is written", func(t *testing.T) {
		logtest.Guard(t)
		err := failingWrite(t, `CREATE TRIGGER fail_gift_update BEFORE UPDATE ON gifts BEGIN SELECT RAISE(ABORT, 'boom'); END`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update gift")
	})
	t.Run("a failed ledger write is reported", func(t *testing.T) {
		logtest.Guard(t)
		err := failingWrite(t, `CREATE TRIGGER fail_ledger BEFORE INSERT ON data_backfills BEGIN SELECT RAISE(ABORT, 'boom'); END`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "record completion")
	})
	t.Run("missing gifts table fails closed", func(t *testing.T) {
		logtest.Guard(t)
		db := dbtest.New(t)
		dbtest.HideTable(t, db, "gifts")
		_, err := NormalizeGiftDatesToCalendarDays(db, time.UTC)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load gifts")
	})
}
