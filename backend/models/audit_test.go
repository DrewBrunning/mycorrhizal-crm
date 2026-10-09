package models

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newAuditTestDB builds a real migrated schema (CLAUDE.md backend trap 1) with
// its own synchronous audit recorder, so hooks persist events inline and the
// test reads them with no flush. The recorder is bound to this DB alone: there
// is nothing to unregister and no state shared with any other test.
func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	NewAuditRecorder(db, WithSync())
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
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityContact, contact.VCardUID))

	note := Note{UserID: user.ID, ContactID: &contact.ID, Content: "note"}
	require.NoError(t, db.Create(&note).Error)
	note.Content = "edited"
	require.NoError(t, db.Save(&note).Error)
	require.NoError(t, db.Delete(&note).Error)
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityNote, uintToStr(note.ID)))

	activity := Activity{UserID: user.ID, Title: "a"}
	require.NoError(t, db.Create(&activity).Error)
	activity.Title = "b"
	require.NoError(t, db.Save(&activity).Error)
	require.NoError(t, db.Delete(&activity).Error)
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityActivity, activity.UUID))

	le := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "moved"}
	require.NoError(t, db.Create(&le).Error)
	le.Type = "graduated"
	require.NoError(t, db.Save(&le).Error)
	require.NoError(t, db.Delete(&le).Error)
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityLifeEvent, le.ID))

	gift := Gift{UserID: user.ID, EntityID: contact.VCardUID}
	require.NoError(t, db.Create(&gift).Error)
	require.NoError(t, db.Delete(&gift).Error)
	assert.EqualValues(t, 2, countAuditEvents(t, db, AuditEntityGift, gift.ID))

	circle := Circle{UserID: user.ID, Name: "c"}
	require.NoError(t, db.Create(&circle).Error)
	circle.Name = "c2"
	require.NoError(t, db.Save(&circle).Error)
	require.NoError(t, db.Delete(&circle).Error)
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityCircle, circle.ID))

	tag := Tag{UserID: user.ID, Name: "t"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Delete(&tag).Error)
	assert.EqualValues(t, 2, countAuditEvents(t, db, AuditEntityTag, tag.ID))

	household := Household{UserID: user.ID, Name: "h", Type: "family_unit"}
	require.NoError(t, db.Create(&household).Error)
	household.Name = "h2"
	require.NoError(t, db.Save(&household).Error)
	require.NoError(t, db.Delete(&household).Error)
	assert.EqualValues(t, 3, countAuditEvents(t, db, AuditEntityHousehold, household.ID))

	reminder := Reminder{UserID: user.ID, ContactID: &contact.ID, Message: "r", RemindAt: time.Now(), Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)
	require.NoError(t, db.Delete(&reminder).Error)
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

	require.ErrorContains(t, db.Model(&AuditEvent{}).Where("entity_type = ?", AuditEntityContact).Update("operation", "create").Error,
		"audit_events is append-only: UPDATE is not allowed",
		"audit_events must reject UPDATE")
}

// TestAudit_SecretFieldsNeverReachTheLog pins the deny-list: a snapshot whose
// JSON carries a deny-listed key (e.g. a password field) has it stripped.
func TestAudit_SecretFieldsNeverReachTheLog(t *testing.T) {
	t.Parallel()
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

	var event AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		AuditEntityContact, contact.VCardUID, AuditOpUpdate).First(&event).Error)
	require.NotEmpty(t, event.BeforeSnapshot)
	var before Contact
	require.NoError(t, json.Unmarshal([]byte(event.BeforeSnapshot), &before))
	assert.Equal(t, "Before", before.Firstname, "the before snapshot must capture the pre-update firstname")
}

// TestAudit_BeforeSnapshotQueryFailureIsLogged pins the fix for a silent
// swallow: auditBeforeSave's pre-update re-query used to drop any error on
// the floor, leaving state.before == "" with no trace — indistinguishable
// from a legitimate "no prior state". A transient failure there (a
// busy-timeout under real contention; a bogus/stale entityID here, which
// exercises the identical err != nil branch) must now be logged so it can't
// masquerade as a normal update and silently starve a before-snapshot
// consumer like reach-out detection.
func TestAudit_BeforeSnapshotQueryFailureIsLogged(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditsnapfail", Password: "password123!A", Email: "auditsnapfail@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Before"}
	require.NoError(t, db.Create(&contact).Error)

	buf := &bytes.Buffer{}
	oldLogger := logger.Logger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	t.Cleanup(func() {
		logger.Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})

	tx := db.Session(&gorm.Session{})
	auditBeforeSave[Contact](tx, AuditEntityContact, contact.ID+999999, false)

	state, ok := tx.Statement.Context.Value(auditStateKey).(*auditState)
	require.True(t, ok)
	assert.Empty(t, state.before, "a failed re-query must still leave before empty, not a stale/wrong snapshot")
	assert.Contains(t, buf.String(), "audit: failed to load pre-update state for before-snapshot")
	assert.Contains(t, buf.String(), `"entity_type":"contact"`)
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
	brokenDB := dbtest.New(t)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	require.NoError(t, brokenSQL.Close())
	rec := &auditLogger{db: brokenDB}
	installAuditRecorder(db, rec)

	// The audit write now fails (logged, ignored); the contact must still save.
	contact := Contact{UserID: user.ID, Firstname: "Survivor"}
	require.NoError(t, db.Create(&contact).Error)

	rec.Flush()
	assert.EqualValues(t, 1, rec.FailedWrites(), "the audit write failed (and was counted), the real write did not")
}

// TestAudit_HookGuardsAndMarshalFailures covers the defensive early returns the
// hook helpers take when a hook fires without a usable statement (a nil tx), on
// a zero-identity bulk-hook model, or when a snapshot cannot be marshaled. The
// refactor in issue #1493 moved the recorder out of audit.go; these branches
// stayed behind and would otherwise go untested there.
func TestAudit_HookGuardsAndMarshalFailures(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditguards", Password: "password123!A", Email: "auditguards@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// A nil tx is a no-op for both hooks (some unit tests call them directly).
	auditAfterSave(nil, AuditEntityContact, "1", user.ID)
	auditAfterDelete(nil, AuditEntityContact, "1", user.ID, &Contact{})

	// Zero-identity bulk-hook models are skipped, not recorded.
	tx := db.Session(&gorm.Session{})
	auditAfterSave(tx, AuditEntityContact, "0", user.ID)
	auditAfterDelete(tx, AuditEntityContact, "0", user.ID, &Contact{})

	// An unmarshalable snapshot makes auditAfterDelete give up silently.
	auditAfterDelete(tx, AuditEntityContact, "1", user.ID, make(chan int))

	// The redaction helpers surface their own marshal/unmarshal errors.
	_, err := redactJSON([]byte("{not json"))
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)

	_, err = redactedJSON(make(chan int))
	var unsupportedErr *json.UnsupportedTypeError
	require.ErrorAs(t, err, &unsupportedErr)
}

func uintToStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
