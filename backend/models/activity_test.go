package models

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupActivityTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &Activity{}))
	return db
}

// setupActivityTestDBFrozenClock is setupActivityTestDB with GORM's NowFunc
// pinned to a single instant. It existed to make the old
// UpdatedAt.Unix()-derived ETag deterministic across back-to-back Save()
// calls; under ADR 0006 the token is a monotonic revision counter with no
// wall-clock input, so the frozen clock is kept only for general
// deterministic UpdatedAt values.
func setupActivityTestDBFrozenClock(t *testing.T) *gorm.DB {
	t.Helper()

	frozen := time.Now()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NowFunc: func() time.Time { return frozen },
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &Activity{}))
	return db
}

func TestActivityBeforeCreateGeneratesUUID(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now(), Type: InteractionTypeMeal}
	require.NoError(t, db.Create(&activity).Error)

	assert.NotEmpty(t, activity.UUID)
}

func TestActivityBeforeCreatePreservesExplicitUUID(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now(), UUID: "explicit-uuid"}
	require.NoError(t, db.Create(&activity).Error)

	assert.Equal(t, "explicit-uuid", activity.UUID)
}

func TestActivityQualifying(t *testing.T) {
	visit := Activity{Type: InteractionTypeVisit}
	assert.True(t, visit.Qualifying(), "a visit is a real qualifying interaction")

	photo := Activity{Type: InteractionTypePhoto}
	assert.False(t, photo.Qualifying(), "a passive/social-media-like photo share does not qualify")

	unset := Activity{}
	assert.True(t, unset.Qualifying(), "an unrecognized/unset type defaults to qualifying")
}

func TestActivityETagGeneratedOnCreateAndPersists(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now(), Type: InteractionTypeMeal}
	require.NoError(t, db.Create(&activity).Error)

	// ADR 0006: token = revision counter stamped at 1 on create, ETag derived
	// from it (e-{id}-{revision}).
	require.NotEmpty(t, activity.ETag)
	assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), activity.ETag)
	assert.Equal(t, int64(1), activity.Revision)
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, activity.Revision), activity.ETag)

	var reloaded Activity
	require.NoError(t, db.First(&reloaded, activity.ID).Error)
	assert.Equal(t, activity.ETag, reloaded.ETag, "ETag must be persisted, not just set in memory")
	assert.Equal(t, int64(1), reloaded.Revision, "revision must be persisted, not just set in memory")
}

func TestActivityETagChangesOnUpdate(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	firstETag := activity.ETag

	// updated_at in the map no longer drives the token (ADR 0006) — the
	// revision counter does.
	require.NoError(t, db.Model(&activity).Updates(map[string]any{"title": "Dinner", "updated_at": time.Now().Add(10 * time.Second)}).Error)

	assert.NotEqual(t, firstETag, activity.ETag, "updating an Activity must change its ETag")
	assert.Equal(t, int64(2), activity.Revision, "an update bumps the revision to 2")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, activity.Revision), activity.ETag)
}

// TestActivityRevisionBumpsPerSaveNoLoop replaces the old
// TestActivityETagSaveDoesNotLoop: under ADR 0006 every persisted write IS a
// new revision, so back-to-back Save() calls bump revision exactly twice and
// never loop (UpdateColumns bypasses hooks).
func TestActivityRevisionBumpsPerSaveNoLoop(t *testing.T) {
	db := setupActivityTestDBFrozenClock(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	require.Equal(t, int64(1), activity.Revision)

	// AfterSave must use UpdateColumns (which skips hooks), not a nested Save
	// — otherwise these re-saves would recurse forever.
	require.NoError(t, db.Save(&activity).Error)
	require.NoError(t, db.Save(&activity).Error)

	assert.Equal(t, int64(3), activity.Revision, "each plain Save bumps the revision (1 create + 2 saves)")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, activity.Revision), activity.ETag)

	var reloaded Activity
	require.NoError(t, db.First(&reloaded, activity.ID).Error)
	assert.Equal(t, int64(3), reloaded.Revision, "in-memory and persisted revisions must agree (no loop drift)")
	assert.Equal(t, activity.ETag, reloaded.ETag)
}

// TestActivityETagBulkUpdateOnZeroValueReceiverDoesNotCorrupt pins the
// zero-value-receiver guard in Activity.AfterSave: a bulk
// Model(&Activity{}).Where(...).Update on a zero-value receiver fires the
// hook with no primary key, and a naive hook would widen its UpdateColumns to
// every row in the table (writing "e-0-..." and resetting every revision).
// Both must be left alone.
func TestActivityETagBulkUpdateOnZeroValueReceiverDoesNotCorrupt(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	one := Activity{UserID: user.ID, Title: "One", Date: time.Now()}
	two := Activity{UserID: user.ID, Title: "Two", Date: time.Now()}
	require.NoError(t, db.Create(&one).Error)
	require.NoError(t, db.Create(&two).Error)

	// Capture the ETags/revisions the create-time hook assigned, so the
	// assertions below can compare against what was actually stored.
	etagBefore := map[uint]string{}
	var before []Activity
	require.NoError(t, db.Order("id").Find(&before).Error)
	require.Len(t, before, 2)
	for _, r := range before {
		require.NotEmpty(t, r.ETag)
		etagBefore[r.ID] = r.ETag
	}

	require.NoError(t, db.Model(&Activity{}).Where("user_id = ?", user.ID).
		Update("title", "Renamed").Error)

	var rows []Activity
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.NotEmpty(t, r.ETag)
		assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), r.ETag, "bulk update must not rewrite ETags from an empty ID")
		assert.Equal(t, etagBefore[r.ID], r.ETag, "ETag must survive the bulk update unchanged")
		assert.Equal(t, int64(1), r.Revision, "bulk update on a zero-value receiver must not bump revisions")
	}
}
