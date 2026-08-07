package models

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newAuditTestDB builds a real migrated schema (CLAUDE.md backend trap 1) and
// registers the audit recorder against it so hooks persist events.
func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-test.db"))
	require.NoError(t, err)
	RegisterAuditDB(db)
	t.Cleanup(func() { AuditFlush() })
	return db
}

func countAuditEvents(t *testing.T, db *gorm.DB, entityType, entityID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&AuditEvent{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Count(&count).Error)
	return count
}

// TestAudit_EveryEntityCreateUpdateDeleteProducesOneEvent is the completeness
// check: each audited entity's create/update/delete fires exactly one event.
func TestAudit_EveryEntityCreateUpdateDeleteProducesOneEvent(t *testing.T) {
	db := newAuditTestDB(t)

	user := User{Username: "audituser", Password: "password123!A", Email: "audit@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Lastname = "Byron"
	require.NoError(t, db.Save(&contact).Error)
	require.NoError(t, db.Delete(&contact).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityContact, contact.VCardUID))

	note := Note{UserID: user.ID, ContactID: &contact.ID, Content: "note"}
	require.NoError(t, db.Create(&note).Error)
	note.Content = "edited"
	require.NoError(t, db.Save(&note).Error)
	require.NoError(t, db.Delete(&note).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityNote, uintToStr(note.ID)))

	activity := Activity{UserID: user.ID, Title: "a"}
	require.NoError(t, db.Create(&activity).Error)
	activity.Title = "b"
	require.NoError(t, db.Save(&activity).Error)
	require.NoError(t, db.Delete(&activity).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityActivity, activity.UUID))

	le := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "moved"}
	require.NoError(t, db.Create(&le).Error)
	le.Type = "graduated"
	require.NoError(t, db.Save(&le).Error)
	require.NoError(t, db.Delete(&le).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityLifeEvent, le.ID))

	gift := Gift{UserID: user.ID, EntityID: contact.VCardUID}
	require.NoError(t, db.Create(&gift).Error)
	require.NoError(t, db.Delete(&gift).Error)
	AuditFlush()
	assert.EqualValues(t, 2, countAuditEvents(t, db, AuditEntityGift, gift.ID))

	circle := Circle{UserID: user.ID, Name: "c"}
	require.NoError(t, db.Create(&circle).Error)
	circle.Name = "c2"
	require.NoError(t, db.Save(&circle).Error)
	require.NoError(t, db.Delete(&circle).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityCircle, circle.ID))

	tag := Tag{UserID: user.ID, Name: "t"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Delete(&tag).Error)
	AuditFlush()
	assert.EqualValues(t, 2, countAuditEvents(t, db, AuditEntityTag, tag.ID))

	household := Household{UserID: user.ID, Name: "h", Type: "family_unit"}
	require.NoError(t, db.Create(&household).Error)
	household.Name = "h2"
	require.NoError(t, db.Save(&household).Error)
	require.NoError(t, db.Delete(&household).Error)
	AuditFlush()
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityHousehold, household.ID))

	reminder := Reminder{UserID: user.ID, ContactID: &contact.ID, Message: "r", RemindAt: time.Now(), Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)
	require.NoError(t, db.Delete(&reminder).Error)
	AuditFlush()
	assert.EqualValues(t, 2, countAuditEvents(t, db, AuditEntityReminder, uintToStr(reminder.ID)))
}

// TestAudit_TableRejectsMutation pins the DB-level immutability trigger: an
// UPDATE against audit_events is rejected even though the app model has no
// update path (the trigger is the safety net that catches raw-SQL mistakes).
func TestAudit_TableRejectsMutation(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditmut", Password: "password123!A", Email: "auditmut@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "A"}
	require.NoError(t, db.Create(&contact).Error)
	AuditFlush()

	require.Error(t, db.Model(&AuditEvent{}).Where("entity_type = ?", AuditEntityContact).Update("operation", "create").Error,
		"audit_events must reject UPDATE")
}

// TestAudit_SecretFieldsNeverReachTheLog pins the deny-list: a snapshot whose
// JSON carries a deny-listed key (e.g. a password field) has it stripped.
func TestAudit_SecretFieldsNeverReachTheLog(t *testing.T) {
	raw := `{"firstname":"Ada","password":"hunter2","nested":{"totp_secret":"abc","ok":1},"api_token_hash":"x"}`
	redacted, err := redactJSON([]byte(raw))
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(redacted, &m))
	assert.NotContains(t, m, "password")
	assert.Equal(t, "Ada", m["firstname"])
	nested, ok := m["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, nested, "totp_secret")
	assert.Equal(t, float64(1), nested["ok"])
	assert.NotContains(t, m, "api_token_hash")
}

// TestAudit_UpdateEventStoresBeforeSnapshot checks that an update event's
// before_snapshot reflects the pre-update state (needed for undo).
func TestAudit_UpdateEventStoresBeforeSnapshot(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditsnap", Password: "password123!A", Email: "auditsnap@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Before", Lastname: "Name"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Firstname = "After"
	require.NoError(t, db.Save(&contact).Error)
	AuditFlush()

	var event AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		AuditEntityContact, contact.VCardUID, AuditOpUpdate).First(&event).Error)
	require.NotEmpty(t, event.BeforeSnapshot)
	var before Contact
	require.NoError(t, json.Unmarshal([]byte(event.BeforeSnapshot), &before))
	assert.Equal(t, "Before", before.Firstname, "the before snapshot must capture the pre-update firstname")
}

// TestAudit_HookFailureDoesNotRollBackTheRealWrite verifies the fire-and-forget
// contract: when the audit write fails (its session is broken), the real
// create still succeeds.
func TestAudit_HookFailureDoesNotRollBackTheRealWrite(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditfail", Password: "password123!A", Email: "auditfail@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Point the recorder at a session whose pool is closed, so every audit
	// write fails — while the app's own db stays healthy.
	brokenDB, err := database.InitDB(filepath.Join(t.TempDir(), "broken-audit.db"))
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	require.NoError(t, brokenSQL.Close())
	RegisterAuditDB(brokenDB)

	// The audit write now fails (logged, ignored); the contact must still save.
	contact := Contact{UserID: user.ID, Firstname: "Survivor"}
	require.NoError(t, db.Create(&contact).Error)

	// Restore a working session for the rest of the test process.
	RegisterAuditDB(db)
	AuditFlush()
}

func uintToStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
