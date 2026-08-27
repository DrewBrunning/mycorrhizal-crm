package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActivityAndLifeEventETag_RealMigratedSchema is the real-DB check for
// T12a. The AutoMigrate
// test DBs used elsewhere (setupActivityTestDB / setupLifeEventTestDB)
// derive their schema from the same Go struct tags the application code
// uses, so they cannot catch a GORM column-tag mismatch against the real
// migration SQL — the `e_tag` vs `etag` bug class that shipped broken for
// ContactSyncLink.ETag. Only a database.InitDB-migrated DB, which applies
// migration 000041's real `etag` column, can.
//
// It proves the ticket's three assertions against the real schema:
//  1. a new Activity gets an ETag;
//  2. updating it changes the ETag;
//  3. LifeEvent — served too (its ETag concern was entirely unaddressed,
//     and T12b will serve both Interaction and LifeEvent as CalDAV) — has
//     the identical behavior, with its ETag derived from the UUID string PK.
func TestActivityAndLifeEventETag_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "etag-real.db")
	db := dbtest.NewAt(t, dbPath)

	user := User{Username: "realdbtester", Password: "password123!A", Email: "realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// --- Activity (uint PK, gorm.Model) ---
	activity := Activity{UserID: user.ID, Title: "Coffee", Date: time.Now(), Type: InteractionTypeMeal}
	require.NoError(t, db.Create(&activity).Error)
	require.NotEmpty(t, activity.ETag, "assertion 1: a new Activity gets an ETag")
	assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), activity.ETag)
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, activity.UpdatedAt.Unix()), activity.ETag)

	var reloadedActivity Activity
	require.NoError(t, db.First(&reloadedActivity, activity.ID).Error)
	assert.Equal(t, activity.ETag, reloadedActivity.ETag, "ETag must persist through the real 'etag' column")

	// Update with a bumped updated_at (GORM overwrites UpdatedAt with
	// time.Now() on every struct save, which can land in the same Unix
	// second as the create — an explicit updated_at in the update map makes
	// the ETag change deterministic).
	future := time.Now().Add(10 * time.Second)
	require.NoError(t, db.Model(&activity).Updates(map[string]any{"title": "Dinner", "updated_at": future}).Error)
	assert.NotEqual(t, reloadedActivity.ETag, activity.ETag, "assertion 2: updating an Activity changes its ETag")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", activity.ID, future.Unix()), activity.ETag)

	var reloadedActivity2 Activity
	require.NoError(t, db.First(&reloadedActivity2, activity.ID).Error)
	assert.Equal(t, activity.ETag, reloadedActivity2.ETag, "the updated ETag must persist")

	// --- LifeEvent (UUID string PK) ---
	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeGraduated}
	require.NoError(t, db.Create(&event).Error)
	require.NotEmpty(t, event.ETag, "a new LifeEvent gets an ETag")
	assert.Regexp(t, regexp.MustCompile(`^e-[0-9a-f-]+-\d+$`), event.ETag)
	assert.Equal(t, fmt.Sprintf("e-%s-%d", event.ID, event.UpdatedAt.Unix()), event.ETag)

	var reloadedEvent LifeEvent
	require.NoError(t, db.First(&reloadedEvent, "id = ?", event.ID).Error)
	assert.Equal(t, event.ETag, reloadedEvent.ETag, "LifeEvent ETag must persist through the real 'etag' column")

	eventFuture := time.Now().Add(10 * time.Second)
	require.NoError(t, db.Model(&event).Updates(map[string]any{"description": "updated", "updated_at": eventFuture}).Error)
	assert.NotEqual(t, reloadedEvent.ETag, event.ETag, "updating a LifeEvent changes its ETag")
	assert.Equal(t, fmt.Sprintf("e-%s-%d", event.ID, eventFuture.Unix()), event.ETag)

	var reloadedEvent2 LifeEvent
	require.NoError(t, db.First(&reloadedEvent2, "id = ?", event.ID).Error)
	assert.Equal(t, event.ETag, reloadedEvent2.ETag, "the updated LifeEvent ETag must persist")
}

// TestLifeEventCategory_RealMigratedSchema is T36's real-DB check for the
// same column-tag-mismatch trap this file's doc comment describes: an
// AutoMigrate-backed test can't catch LifeEvent.Category's `gorm:"column:
// category"` tag disagreeing with migration 000011's actual `category`
// column, because AutoMigrate would happily derive its own schema from
// whatever the tag says. Only a database.InitDB-migrated DB proves the two
// agree.
func TestLifeEventCategory_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "category-real.db")
	db := dbtest.NewAt(t, dbPath)

	user := User{Username: "categorytester", Password: "password123!A", Email: "category@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{
		UserID:   user.ID,
		EntityID: contact.VCardUID,
		Type:     LifeEventTypeMoved,
		Category: LifeEventCategoryHomeLiving,
	}
	require.NoError(t, db.Create(&event).Error)

	var reloaded LifeEvent
	require.NoError(t, db.First(&reloaded, "id = ?", event.ID).Error)
	assert.Equal(t, LifeEventCategoryHomeLiving, reloaded.Category,
		"Category must persist through the real migrated 'category' column")

	// And directly through the raw column, ruling out GORM masking a tag
	// mismatch by deriving its own (wrong) schema.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	var raw string
	require.NoError(t, sqlDB.QueryRow("SELECT category FROM life_events WHERE id = ?", event.ID).Scan(&raw))
	assert.Equal(t, LifeEventCategoryHomeLiving, raw)
}
