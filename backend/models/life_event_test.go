package models

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"mycorrhizal/contactmodel"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLifeEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &LifeEvent{}))
	return db
}

func TestLifeEventBeforeCreateGeneratesUUID(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeGraduated}
	require.NoError(t, db.Create(&event).Error)

	assert.NotEmpty(t, event.ID)
}

func TestLifeEventBeforeCreatePreservesExplicitID(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeMoved}
	require.NoError(t, db.Create(&event).Error)

	assert.Equal(t, "explicit-id", event.ID)
}

// A year-only PartialDate ("known only to a year", per §91.6) and a
// multi-entry RelatedEntityIDs list must both round-trip through a real
// save/reload exactly as stored -- not just compile, per the WP-83 lesson
// that only a real AutoMigrate-backed save/reload catches column/serializer
// mismatches.
func TestLifeEventPartialDateAndRelatedEntityIDsRoundTrip(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	subject := Contact{UserID: user.ID, Firstname: "Alice"}
	spouse := Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&subject).Error)
	require.NoError(t, db.Create(&spouse).Error)

	year := 2024
	event := LifeEvent{
		UserID:           user.ID,
		EntityID:         subject.VCardUID,
		Type:             LifeEventTypeMarried,
		Date:             &contactmodel.PartialDate{Year: &year},
		Source:           LifeEventSourceUser,
		RelatedEntityIDs: []string{spouse.VCardUID},
	}
	require.NoError(t, db.Create(&event).Error)

	var reloaded LifeEvent
	require.NoError(t, db.First(&reloaded, "id = ?", event.ID).Error)

	require.NotNil(t, reloaded.Date)
	require.NotNil(t, reloaded.Date.Year)
	assert.Equal(t, 2024, *reloaded.Date.Year)
	assert.Nil(t, reloaded.Date.Month)
	assert.Equal(t, []string{spouse.VCardUID}, reloaded.RelatedEntityIDs)
}

func TestLifeEventETagGeneratedOnCreateAndPersists(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeGraduated}
	require.NoError(t, db.Create(&event).Error)

	require.NotEmpty(t, event.ETag)
	// LifeEvent has a UUID string PK, so its ETag is derived from that, not
	// a numeric ID.
	assert.Regexp(t, regexp.MustCompile(`^e-[0-9a-f-]+-\d+$`), event.ETag)
	assert.Equal(t, fmt.Sprintf("e-%s-%d", event.ID, event.UpdatedAt.Unix()), event.ETag)

	var reloaded LifeEvent
	require.NoError(t, db.First(&reloaded, "id = ?", event.ID).Error)
	assert.Equal(t, event.ETag, reloaded.ETag, "ETag must be persisted, not just set in memory")
}

func TestLifeEventETagChangesOnUpdate(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeGraduated}
	require.NoError(t, db.Create(&event).Error)
	firstETag := event.ETag

	future := time.Now().Add(10 * time.Second)
	require.NoError(t, db.Model(&event).Updates(map[string]any{"description": "updated", "updated_at": future}).Error)

	assert.NotEqual(t, firstETag, event.ETag, "updating a LifeEvent must change its ETag")
	assert.Equal(t, fmt.Sprintf("e-%s-%d", event.ID, future.Unix()), event.ETag)
}

func TestLifeEventETagSaveDoesNotLoop(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: LifeEventTypeGraduated}
	require.NoError(t, db.Create(&event).Error)
	etag := event.ETag

	// AfterSave must use UpdateColumn (which skips hooks), not a nested Save
	// — otherwise these re-saves would recurse forever.
	require.NoError(t, db.Save(&event).Error)
	require.NoError(t, db.Save(&event).Error)

	assert.Equal(t, etag, event.ETag, "a save that does not change UpdatedAt must not rewrite the ETag")
}

// TestLifeEventETagBulkRepointOnZeroValueReceiverDoesNotCorrupt pins the
// zero-value-receiver guard in LifeEvent.AfterSave, mirroring contact merge's
// bulk entity_id repoint (contact_merge_service.go): a bulk
// Model(&LifeEvent{}).Where(...).Update fires the hook with no primary key,
// and a naive hook would widen its UpdateColumn to every row in the table
// (writing "e--<ts>"). The ETags must be left alone.
func TestLifeEventETagBulkRepointOnZeroValueReceiverDoesNotCorrupt(t *testing.T) {
	db := setupLifeEventTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	alice := Contact{UserID: user.ID, Firstname: "Alice"}
	bob := Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	loser := LifeEvent{UserID: user.ID, EntityID: alice.VCardUID, Type: LifeEventTypeGraduated}
	other := LifeEvent{UserID: user.ID, EntityID: bob.VCardUID, Type: LifeEventTypeMoved}
	require.NoError(t, db.Create(&loser).Error)
	require.NoError(t, db.Create(&other).Error)
	loserETag, otherETag := loser.ETag, other.ETag

	require.NoError(t, db.Model(&LifeEvent{}).Where("entity_id = ? AND user_id = ?", alice.VCardUID, user.ID).
		Update("entity_id", bob.VCardUID).Error)

	var rows []LifeEvent
	require.NoError(t, db.Order("created_at").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.NotEmpty(t, r.ETag)
		assert.Regexp(t, regexp.MustCompile(`^e-[0-9a-f-]+-\d+$`), r.ETag, "bulk repoint must not rewrite ETags from an empty ID")
	}
	assert.Contains(t, []string{loserETag, otherETag}, rows[0].ETag, "ETags must survive the bulk repoint unchanged")
	assert.Contains(t, []string{loserETag, otherETag}, rows[1].ETag, "ETags must survive the bulk repoint unchanged")
}
