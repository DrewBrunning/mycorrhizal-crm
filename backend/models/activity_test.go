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

	require.NotEmpty(t, activity.ETag)
	assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), activity.ETag)
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, activity.UpdatedAt.Unix()), activity.ETag)

	var reloaded Activity
	require.NoError(t, db.First(&reloaded, activity.ID).Error)
	assert.Equal(t, activity.ETag, reloaded.ETag, "ETag must be persisted, not just set in memory")
}

func TestActivityETagChangesOnUpdate(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	firstETag := activity.ETag

	future := time.Now().Add(10 * time.Second)
	require.NoError(t, db.Model(&activity).Updates(map[string]any{"title": "Dinner", "updated_at": future}).Error)

	assert.NotEqual(t, firstETag, activity.ETag, "updating an Activity must change its ETag")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, future.Unix()), activity.ETag)
}

func TestActivityETagSaveDoesNotLoop(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	etag := activity.ETag

	// AfterSave must use UpdateColumn (which skips hooks), not a nested Save
	// — otherwise these re-saves would recurse forever.
	require.NoError(t, db.Save(&activity).Error)
	require.NoError(t, db.Save(&activity).Error)

	assert.Equal(t, etag, activity.ETag, "a save that does not change UpdatedAt must not rewrite the ETag")
}

// TestActivityETagBulkUpdateOnZeroValueReceiverDoesNotCorrupt pins the
// zero-value-receiver guard in Activity.AfterSave: a bulk
// Model(&Activity{}).Where(...).Update on a zero-value receiver fires the
// hook with no primary key, and a naive hook would widen its UpdateColumn to
// every row in the table (writing "e-0-..."). The ETag must be left alone.
func TestActivityETagBulkUpdateOnZeroValueReceiverDoesNotCorrupt(t *testing.T) {
	db := setupActivityTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	one := Activity{UserID: user.ID, Title: "One", Date: time.Now()}
	two := Activity{UserID: user.ID, Title: "Two", Date: time.Now()}
	require.NoError(t, db.Create(&one).Error)
	require.NoError(t, db.Create(&two).Error)

	// Capture the ETags the create-time hook assigned, so the assertion below
	// can compare against what was actually stored.
	//
	// Deliberately NOT re-derived from UpdatedAt after the update: the bulk
	// Update legitimately bumps UpdatedAt while the hook (correctly) leaves the
	// ETag alone, so `fmt.Sprintf("e-%d-%d", r.ID, r.UpdatedAt.Unix())` only
	// matches when the create and the update land in the same wall-clock
	// second. That made this test flaky under parallel package load — it failed
	// whenever the two straddled a second boundary.
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
		assert.NotEqual(t, fmt.Sprintf("e-0-%d", r.UpdatedAt.Unix()), r.ETag,
			"an ETag derived from a zero ID means the hook fired on the zero-value receiver")
	}
}
