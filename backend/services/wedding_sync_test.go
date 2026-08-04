package services

import (
	"path/filepath"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Tests for the wedding-anniversary <-> married-LifeEvent sync
// (wedding_sync.go). Real migrated schema per CLAUDE.md trap 1 — these
// exercise the actual `life_events` and `contacts` columns.

func setupWeddingSyncDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "wedding-sync.db"))
	require.NoError(t, err)
	return db
}

func createWeddingSyncUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{Username: "wedding-tester", Password: "password123!A", Email: "wedding@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createWeddingSyncContact(t *testing.T, db *gorm.DB, user models.User, anniversaries []contactmodel.Anniversary) models.Contact {
	t.Helper()
	rec := &contactmodel.Record{
		Card: contactmodel.Card{
			UID: uuid.NewString(),
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Ada"},
			}},
			Anniversaries: anniversaries,
			// A canary to prove the LifeEvent-side sync never clobbers other
			// card data (CLAUDE.md traps 2/3).
			PersonalInfo: []contactmodel.PersonalInfo{{Kind: "hobby", Value: "chess"}},
		},
	}
	contact := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&contact, rec, "")
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func partialDate(y, m, d int) *contactmodel.PartialDate {
	return &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
}

func marriedEvent(t *testing.T, db *gorm.DB, uid string, userID uint) *models.LifeEvent {
	t.Helper()
	var event models.LifeEvent
	err := db.Where("entity_id = ? AND user_id = ? AND type = ?", uid, userID, models.LifeEventTypeMarried).First(&event).Error
	if err != nil {
		t.Fatalf("married life event not found: %v", err)
	}
	return &event
}

func TestSyncWeddingFromCard_CreatesMarriedLifeEvent(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, []contactmodel.Anniversary{
		{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: partialDate(2009, 8, 16)}},
	})

	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, partialDate(2009, 8, 16)))

	event := marriedEvent(t, db, contact.VCardUID, user.ID)
	assert.NotNil(t, event.Date)
	assert.Equal(t, 2009, *event.Date.Year)
	assert.Equal(t, 8, *event.Date.Month)
	assert.Equal(t, 16, *event.Date.Day)
	assert.Equal(t, models.LifeEventSourceUser, event.Source)
}

func TestSyncWeddingFromCard_UpdatesExistingLifeEventDate(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, []contactmodel.Anniversary{
		{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: partialDate(2009, 8, 16)}},
	})
	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, partialDate(2009, 8, 16)))

	// The card's anniversary moves; the life event follows.
	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, partialDate(2010, 9, 1)))

	event := marriedEvent(t, db, contact.VCardUID, user.ID)
	assert.Equal(t, 2010, *event.Date.Year)
	assert.Equal(t, 9, *event.Date.Month)
	assert.Equal(t, 1, *event.Date.Day)
}

func TestSyncWeddingFromCard_ClearsDateButKeepsNarrative(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, nil)

	require.NoError(t, db.Create(&models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMarried,
		Date: partialDate(2009, 8, 16), Description: "eloped in Prague",
	}).Error)

	// Anniversary removed -> the date clears, the narrative survives.
	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, nil))

	event := marriedEvent(t, db, contact.VCardUID, user.ID)
	assert.Nil(t, event.Date)
	assert.Equal(t, "eloped in Prague", event.Description)
}

func TestSyncWeddingFromCard_RemovesEmptyLifeEvent(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, nil)

	require.NoError(t, db.Create(&models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMarried,
		Date: partialDate(2009, 8, 16),
	}).Error)

	// Date-only event with no narrative is an empty shell once the date goes.
	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, nil))

	var count int64
	require.NoError(t, db.Model(&models.LifeEvent{}).Where("entity_id = ? AND user_id = ? AND type = ?", contact.VCardUID, user.ID, models.LifeEventTypeMarried).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSyncWeddingFromLifeEvent_SetsAnniversary(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, nil)

	require.NoError(t, SyncWeddingFromLifeEvent(db, user.ID, contact.VCardUID, partialDate(2009, 8, 16)))

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	date := WeddingDateFromCard(&reloaded.Card)
	require.NotNil(t, date)
	assert.Equal(t, 2009, *date.Year)
	assert.Equal(t, 8, *date.Month)
	assert.Equal(t, 16, *date.Day)
}

func TestSyncWeddingFromLifeEvent_PreservesOtherCardData(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, []contactmodel.Anniversary{
		{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: partialDate(2009, 8, 16)}},
	})

	// The life event's date changes; the anniversary follows, the hobby stays.
	require.NoError(t, SyncWeddingFromLifeEvent(db, user.ID, contact.VCardUID, partialDate(2011, 5, 20)))

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	rec := models.RecordForContact(&reloaded, "", db)
	date := WeddingDateFromCard(&rec.Card)
	require.NotNil(t, date)
	assert.Equal(t, 2011, *date.Year)
	require.Len(t, rec.Card.PersonalInfo, 1)
	assert.Equal(t, "chess", rec.Card.PersonalInfo[0].Value)
}

func TestSyncWeddingFromLifeEvent_ClearsAnniversary(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, []contactmodel.Anniversary{
		{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: partialDate(2009, 8, 16)}},
	})

	require.NoError(t, SyncWeddingFromLifeEvent(db, user.ID, contact.VCardUID, nil))

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Nil(t, WeddingDateFromCard(&reloaded.Card))
}

func TestSyncWedding_Idempotent_NoDuplicateLifeEvent(t *testing.T) {
	db := setupWeddingSyncDB(t)
	user := createWeddingSyncUser(t, db)
	contact := createWeddingSyncContact(t, db, user, nil)

	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, partialDate(2009, 8, 16)))
	require.NoError(t, SyncWeddingFromCard(db, user.ID, contact.VCardUID, partialDate(2009, 8, 16)))

	var count int64
	require.NoError(t, db.Model(&models.LifeEvent{}).Where("entity_id = ? AND user_id = ? AND type = ?", contact.VCardUID, user.ID, models.LifeEventTypeMarried).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
