package atrest_test

import (
	"strings"
	"testing"
	"time"

	"mycorrhizal/atrest"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file lives in the external test package (atrest_test) because it
// exercises the real model structs — models blank-imports atrest to register
// the serializers, so an in-package test would create an import cycle. Here
// both imports are fine: models → atrest (blank), atrest_test → both.

func setup(t *testing.T) (*gorm.DB, uint) {
	t.Helper()
	db := dbtest.New(t)

	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = byte(i)
	}
	require.NoError(t, atrest.Initialize(db, kek))
	t.Cleanup(atrest.ResetForTest)

	user := models.User{Username: "atrest-int", Password: "password123!A", Email: "atrest-int@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user.ID
}

func TestModelRoundTrip_EncryptedAtRest_DecryptedOnRead(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{
		UserID:          userID,
		Firstname:       "Alice",
		HowWeMet:        "met at a mycology conference",
		WorkInformation: "founder, Fungi Labs",
	}
	require.NoError(t, db.Create(&contact).Error)

	// The flat sensitive columns are ciphertext at rest.
	var raw string
	require.NoError(t, db.Raw("SELECT how_we_met FROM contacts WHERE id = ?", contact.ID).Scan(&raw).Error)
	require.Contains(t, raw, "encv1:", "how_we_met must be encrypted at rest")

	// A model read decrypts transparently.
	var loaded models.Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	require.Equal(t, "met at a mycology conference", loaded.HowWeMet)
	require.Equal(t, "founder, Fungi Labs", loaded.WorkInformation)
}

func TestModelRoundTrip_NeutralCardEncrypted(t *testing.T) {
	db, userID := setup(t)

	// Build the contact through the canonical neutral-model path so
	// BeforeSave keeps the Card we set (cardSetDirectly semantics).
	record := contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Full: "Alice Nakamoto"},
			PersonalInfo: []contactmodel.PersonalInfo{
				{Kind: "hobby", Value: "mycology"},
			},
		},
	}
	contact := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&contact, &record, "")
	require.NoError(t, db.Create(&contact).Error)

	var raw string
	require.NoError(t, db.Raw("SELECT card FROM contacts WHERE id = ?", contact.ID).Scan(&raw).Error)
	require.Contains(t, raw, "encv1:", "neutral card must be encrypted at rest")

	var loaded models.Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	require.Equal(t, "Alice Nakamoto", loaded.Card.Name.Full)
	require.Equal(t, "mycology", loaded.Card.PersonalInfo[0].Value)
}

func TestSearchStillWorks_WithEncryptionArmed(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice", HowWeMet: "met at conference"}
	require.NoError(t, db.Create(&contact).Error)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "talked about mycology"}
	require.NoError(t, db.Create(&note).Error)

	// RebuildSearchIndex reads base columns directly via SQL — the FTS columns
	// (firstname, note content) are NOT encrypted, so this must still work.
	require.NoError(t, atrest.Backfill(db))

	// Search by an unencrypted FTS column still finds the contact.
	var count int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM contacts_fts
		JOIN contacts c ON c.id = contacts_fts.rowid
		WHERE contacts_fts MATCH 'Alice' AND contacts_fts.user_id = ?`, userID).Scan(&count).Error)
	require.Equal(t, int64(1), count)

	// Notes FTS (content stays plaintext by design).
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM notes_fts WHERE notes_fts MATCH 'mycology' AND user_id = ?`, userID).Scan(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestExportStillWorks_RecordForContactDecrypts(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice", HowWeMet: "met at conference"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, atrest.Backfill(db))

	// RecordForContact reads the decrypted Card/HowWeMet through the model.
	record := models.RecordForContact(&contact, "", db)
	require.NotNil(t, record)
	given := ""
	for _, comp := range record.Card.Name.Components {
		if comp.Kind == "given" {
			given = comp.Value
		}
	}
	require.Equal(t, "Alice", given, "RecordForContact must read the decrypted neutral card")
	// The flat how_we_met field is available on the model (decrypted).
	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	require.Equal(t, "met at conference", reloaded.HowWeMet)
}

func TestAuditSnapshotEncrypted_UndoStillWorks(t *testing.T) {
	db, userID := setup(t)

	// A real audit event written through the model path (the audit recorder
	// does db.Create(&AuditEvent{...})).
	event := models.AuditEvent{
		EntityType:     "contact",
		EntityID:       "vcard-undo",
		Operation:      "update",
		UserID:         userID,
		BeforeSnapshot: `{"firstname":"Alice","how_we_met":"old text"}`,
	}
	require.NoError(t, db.Create(&event).Error)

	var raw string
	require.NoError(t, db.Raw("SELECT before_snapshot FROM audit_events WHERE id = ?", event.ID).Scan(&raw).Error)
	require.Contains(t, raw, "encv1:", "audit before_snapshot must be encrypted at rest")

	var loaded models.AuditEvent
	require.NoError(t, db.First(&loaded, event.ID).Error)
	require.Contains(t, loaded.BeforeSnapshot, "old text", "undo must read back the decrypted snapshot")
}

func TestBackfillAfterRawInsert_ModelReadRoundTrips(t *testing.T) {
	db, userID := setup(t)

	// Pre-backfill plaintext row written via raw SQL (bypasses serializer).
	require.NoError(t, db.Exec(
		"INSERT INTO contacts (user_id, firstname, how_we_met, card) VALUES (?, 'Legacy', 'old plaintext', ?)",
		userID, `{"kind":"card"}`).Error)

	require.NoError(t, atrest.Backfill(db))

	var raw string
	require.NoError(t, db.Raw("SELECT how_we_met FROM contacts WHERE firstname = 'Legacy'").Scan(&raw).Error)
	require.Contains(t, raw, "encv1:", "backfill must encrypt pre-existing plaintext")

	var loaded models.Contact
	require.NoError(t, db.Where("firstname = ?", "Legacy").First(&loaded).Error)
	require.Equal(t, "old plaintext", loaded.HowWeMet, "backfilled value must decrypt to original")
}

func TestReminderAndLifeEventEncrypted(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	reminder := models.Reminder{UserID: userID, ContactID: &contact.ID, Message: "call about mycology", RemindAt: time.Now().Add(24 * time.Hour), Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)

	// The contact needs a VCardUID before the LifeEvent can reference it.
	require.NotEmpty(t, contact.VCardUID, "BeforeCreate must have generated a VCardUID")
	lifeEvent := models.LifeEvent{UserID: userID, EntityID: contact.VCardUID, Description: "started mycology club", Type: "started_a_hobby"}
	require.NoError(t, db.Create(&lifeEvent).Error)

	var rawReminder string
	require.NoError(t, db.Raw("SELECT message FROM reminders WHERE id = ?", reminder.ID).Scan(&rawReminder).Error)
	require.Contains(t, rawReminder, "encv1:", "reminder message must be encrypted at rest")

	var rawLifeEvent string
	require.NoError(t, db.Raw("SELECT description FROM life_events WHERE id = ?", lifeEvent.ID).Scan(&rawLifeEvent).Error)
	require.Contains(t, rawLifeEvent, "encv1:", "life event description must be encrypted at rest")
}

func TestSearchService_WorksWithEncryptionArmed(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice", HowWeMet: "met at conference"}
	require.NoError(t, db.Create(&contact).Error)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "talked about mycology"}
	require.NoError(t, db.Create(&note).Error)
	activity := models.Activity{UserID: userID, Contacts: []models.Contact{contact}, Title: "coffee", Description: "discussed mushroom taxonomy", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)

	require.NoError(t, atrest.Backfill(db))

	// The real search service: raw `SELECT c.*` into a Contact-shaped struct
	// (serializers decrypt on scan) joined against the FTS index (which stays
	// plaintext). Finding the contact by an unencrypted flat field proves
	// neither the encryption nor the FTS contract broke.
	res, err := services.Search(db, userID, "Alice", 10, nil)
	require.NoError(t, err)
	require.Len(t, res.Contacts, 1, "search must find the contact by FTS flat field")

	// Notes FTS (content deliberately plaintext).
	res, err = services.Search(db, userID, "mycology", 10, nil)
	require.NoError(t, err)
	require.Len(t, res.Notes, 1)

	// Activities FTS (title/description deliberately plaintext).
	res, err = services.Search(db, userID, "mushroom", 10, nil)
	require.NoError(t, err)
	require.Len(t, res.Activities, 1)
}

func TestModelRead_CorruptedPlainCiphertext_FailsClosed(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice", HowWeMet: "met at a conference"}
	require.NoError(t, db.Create(&contact).Error)

	// Corrupt the "encrypted" (plain-string) serializer's stored ciphertext
	// directly, bypassing the serializer — simulates DB corruption or a
	// truncated write.
	require.NoError(t, db.Exec(
		"UPDATE contacts SET how_we_met = 'encv1:main:not-valid-base64!!!' WHERE id = ?", contact.ID).Error)

	var loaded models.Contact
	err := db.First(&loaded, contact.ID).Error
	require.Error(t, err, "a corrupted encrypted column must fail the read closed, not surface garbage")
}

func TestModelRead_CorruptedJSONCiphertext_FailsClosed(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// Corrupt the "encryptedjson" (neutral card) serializer's stored
	// ciphertext directly.
	require.NoError(t, db.Exec(
		"UPDATE contacts SET card = 'encv1:main:not-valid-base64!!!' WHERE id = ?", contact.ID).Error)

	var loaded models.Contact
	err := db.First(&loaded, contact.ID).Error
	require.Error(t, err, "a corrupted encrypted JSON column must fail the read closed, not surface garbage")
}

func TestModelRead_CardDecryptsToInvalidJSON_FailsClosed(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// Valid ciphertext, but the plaintext it decrypts to is not valid JSON —
	// a corruption one layer deeper than a malformed/tampered ciphertext
	// (e.g. the card column pointed at the wrong logical row's bytes).
	ct, err := atrest.Encrypt("not valid json{{{")
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", ct, contact.ID).Error)

	var loaded models.Contact
	err = db.First(&loaded, contact.ID).Error
	require.Error(t, err, "a card column that decrypts to invalid JSON must fail the read, not silently zero out")
}

func TestNotesContentStaysPlaintext_FTSContract(t *testing.T) {
	db, userID := setup(t)

	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "this content must stay searchable"}
	require.NoError(t, db.Create(&note).Error)

	// notes.content is FTS-indexed and deliberately NOT encrypted.
	var raw string
	require.NoError(t, db.Raw("SELECT content FROM notes WHERE id = ?", note.ID).Scan(&raw).Error)
	require.False(t, strings.HasPrefix(raw, "encv1:"), "notes.content is FTS-indexed and stays plaintext by design")
	require.Equal(t, "this content must stay searchable", raw)
}
