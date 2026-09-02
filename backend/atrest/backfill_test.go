package atrest

import (
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// createUser inserts a bare users row via raw SQL (importing models here would
// create an import cycle: models blank-imports atrest to register serializers).
func createUser(t *testing.T, db *gorm.DB, username string) uint {
	t.Helper()
	res := db.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), ?, ?, ?)",
		username, "password123!A", username+"@example.com",
	)
	require.NoError(t, res.Error)
	require.Equal(t, int64(1), res.RowsAffected)
	var uid uint
	require.NoError(t, db.Table("users").Select("id").Where("username = ?", username).Scan(&uid).Error)
	return uid
}

// realDB opens a real migrated SQLite DB in a temp dir (the repo's
// "test against the real migrated schema, not AutoMigrate" rule — CLAUDE.md
// backend trap 1) and arms at-rest encryption with a test master key.
func realDB(t *testing.T) (*gorm.DB, []byte) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db := dbtest.NewAt(t, dbPath)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	require.NoError(t, Initialize(db, kek))
	t.Cleanup(ResetForTest)
	return db, kek
}

func TestInitialize_CreatesWrappedDEK(t *testing.T) {
	db, _ := realDB(t)

	var n int64
	require.NoError(t, db.Table("data_encryption_keys").Count(&n).Error)
	require.Equal(t, int64(1), n, "Initialize must seed exactly one wrapped DEK")

	// Round-trip through the armed engine.
	ct, err := Encrypt("hello")
	require.NoError(t, err)
	got, err := Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, "hello", got)
}

func TestInitialize_WrongKeyFailsClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db := dbtest.NewAt(t, dbPath)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	require.NoError(t, Initialize(db, kek))

	// Re-init with a different master key: the wrapped DEK cannot be
	// unwrapped → fail closed at boot.
	wrong := make([]byte, keySize)
	wrong[0] = 0xFF
	err := Initialize(db, wrong)
	require.Error(t, err, "a wrong master key must fail closed at initialize time")
	require.Contains(t, err.Error(), "wrong DATA_ENCRYPTION_KEY")
}

func TestInitialize_PersistsAcrossRestart(t *testing.T) {
	// Simulate a restart: the DEK row survives, and re-initializing with the
	// same key decrypts rows written before the restart.
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db := dbtest.NewAt(t, dbPath)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	require.NoError(t, Initialize(db, kek))

	ct, err := Encrypt("written-before-restart")
	require.NoError(t, err)
	require.NoError(t, db.Table("contacts").Create(map[string]interface{}{
		"user_id": 1, "firstname": "Alice", "card": ct,
	}).Error)

	// "Restart": fresh Initialize on the same file.
	ResetForTest()
	require.NoError(t, Initialize(db, kek))

	var stored string
	require.NoError(t, db.Table("contacts").Select("card").Where("firstname = ?", "Alice").Scan(&stored).Error)
	got, err := Decrypt(stored)
	require.NoError(t, err)
	require.Equal(t, "written-before-restart", got)
}

func TestRotateMasterKey_RewrapsWithoutTouchingData(t *testing.T) {
	db, oldKEK := realDB(t)

	ct, err := Encrypt("payload-that-must-not-change")
	require.NoError(t, err)

	newKEK := make([]byte, keySize)
	for i := range newKEK {
		newKEK[i] = byte(i) ^ 0x5A
	}
	require.NoError(t, RotateMasterKey(db, oldKEK, newKEK))

	// Data still decrypts under the new master key.
	got, err := Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, "payload-that-must-not-change", got, "rotation must not alter payloads")

	// The payload bytes themselves are byte-identical (rotation only rewraps
	// the DEK row, never re-encrypts data).
	var wrapped struct {
		DEK []byte `gorm:"column:wrapped_dek"`
	}
	require.NoError(t, db.Table("data_encryption_keys").Select("wrapped_dek").Scan(&wrapped).Error)
	require.NotEmpty(t, wrapped.DEK)

	// A restart under the new key works.
	ResetForTest()
	require.NoError(t, Initialize(db, newKEK))
	got, err = Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, "payload-that-must-not-change", got)
}

func TestRotateMasterKey_WrongOldKeyFails(t *testing.T) {
	db, oldKEK := realDB(t)

	wrong := make([]byte, keySize)
	wrong[0] = 0xFF
	err := RotateMasterKey(db, wrong, oldKEK)
	require.Error(t, err, "rotation with the wrong old key must fail closed")
}

func TestRotateMasterKey_MissingArgs(t *testing.T) {
	db, kek := realDB(t)

	require.Error(t, RotateMasterKey(nil, kek, kek), "nil db must be rejected")
	require.Error(t, RotateMasterKey(db, nil, kek), "nil old key must be rejected")
	require.Error(t, RotateMasterKey(db, kek, nil), "nil new key must be rejected")
}

func TestRotateMasterKey_NoDEKYet(t *testing.T) {
	// A real migrated DB (so data_encryption_keys exists) that has never had
	// Initialize called on it — the "rotate before the server has ever
	// booted with a key" operator mistake.
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db := dbtest.NewAt(t, dbPath)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	err := RotateMasterKey(db, kek, kek)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no wrapped DEK found")
}

func TestRotateMasterKey_MissingDEKTableFailsClosed(t *testing.T) {
	// Same "never migrated" scenario as TestInitialize_MissingDEKTableFailsClosed,
	// hitting RotateMasterKey's own read instead of loadOrCreateDEK's.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	err = RotateMasterKey(db, kek, kek)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rotate read DEK")
}

func TestLoadOrCreateDEK_PersistFails(t *testing.T) {
	// A real migrated (writable-schema) DB flipped read-only at the SQLite
	// level — simulates a locked/read-only DB file, a real operator failure
	// mode distinct from "table missing". query_only rejects the INSERT
	// while still allowing the preceding SELECT to succeed.
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db := dbtest.NewAt(t, dbPath)
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	_, err := loadOrCreateDEK(db, kek)
	require.Error(t, err)
	require.Contains(t, err.Error(), "persist wrapped DEK")
}

func TestRotateMasterKey_PersistFails(t *testing.T) {
	db, oldKEK := realDB(t)
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)

	newKEK := make([]byte, keySize)
	for i := range newKEK {
		newKEK[i] = byte(i) ^ 0x5A
	}
	err := RotateMasterKey(db, oldKEK, newKEK)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rotate persist")
}

func TestInitialize_MissingDEKTableFailsClosed(t *testing.T) {
	// A DB with none of this project's hand-written migrations applied (no
	// data_encryption_keys table at all) — Initialize must surface the read
	// error rather than silently treating "table missing" as "no key yet".
	t.Cleanup(ResetForTest)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	err = Initialize(db, kek)
	require.Error(t, err)
	require.Contains(t, err.Error(), "data_encryption_keys")
}

func TestBackfill_NilDBErrors(t *testing.T) {
	err := Backfill(nil)
	require.Error(t, err)
}

func TestBackfill_MalformedColumnSpecErrors(t *testing.T) {
	db, _ := realDB(t)

	// EncryptedColumns is always well-formed in real code (pinned by
	// TestEncryptedColumns_MatchesSerializerTags), but Backfill must still
	// fail loudly rather than skip silently if a future entry is ever
	// malformed — restore the real list after.
	orig := EncryptedColumns
	EncryptedColumns = []string{"no-dot-here"}
	t.Cleanup(func() { EncryptedColumns = orig })

	err := Backfill(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed encrypted-column spec")
}

func TestBackfillColumn_NoSuchTableIsNoOp(t *testing.T) {
	db, _ := realDB(t)
	// A table that genuinely doesn't exist in this schema — the "disabled
	// feature / fresh DB" case the isNoSuchTable guard exists for.
	require.NoError(t, backfillColumn(db, "no_such_table_at_all", "value"))
}

func TestBackfillColumn_OtherSQLErrorPropagates(t *testing.T) {
	db, _ := realDB(t)
	// contacts exists but this column doesn't — a real SQL error distinct
	// from "no such table" must propagate, not be swallowed as a no-op.
	err := backfillColumn(db, "contacts", "no_such_column_at_all")
	require.Error(t, err)
}

func TestBackfillColumn_UpdateFailurePropagates(t *testing.T) {
	db, _ := realDB(t)
	require.NoError(t, db.Table("contacts").Create(map[string]interface{}{
		"user_id": 1, "firstname": "Alice", "how_we_met": "plaintext",
	}).Error)

	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	err := backfillColumn(db, "contacts", "how_we_met")
	require.Error(t, err, "a write failure mid-backfill must surface, not be swallowed")
}

func TestBackfill_EncryptsPlaintextRows_PreservesRowCounts(t *testing.T) {
	db, _ := realDB(t)

	userID := createUser(t, db, "atrest-bf")

	// Plaintext rows written before encryption existed (raw insert bypasses
	// the serializer — this is exactly the pre-backfill state).
	contacts := []map[string]interface{}{
		{"user_id": userID, "firstname": "Bob", "how_we_met": "met at the market", "card": `{"kind":"card"}`},
		{"user_id": userID, "firstname": "Cara", "how_we_met": "college roommate", "card": `{"kind":"card"}`},
	}
	require.NoError(t, db.Table("contacts").Create(contacts).Error)

	before, err := countContacts(t, db, userID)
	require.NoError(t, err)
	require.Equal(t, int64(2), before)

	require.NoError(t, Backfill(db))

	after, err := countContacts(t, db, userID)
	require.NoError(t, err)
	require.Equal(t, before, after, "backfill must preserve row counts (issue #380 verify)")

	// The stored values are now ciphertext, and decrypt back to the originals.
	var rows []struct {
		HowWeMet string
		Card     string
	}
	require.NoError(t, db.Table("contacts").
		Select("how_we_met, card").
		Where("user_id = ?", userID).
		Order("firstname ASC").
		Scan(&rows).Error)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.Contains(t, r.HowWeMet, ciphertextPrefix, "plaintext must be encrypted after backfill")
		require.Contains(t, r.Card, ciphertextPrefix)
		hm, err := Decrypt(r.HowWeMet)
		require.NoError(t, err)
		require.NotEqual(t, "", hm)
		card, err := Decrypt(r.Card)
		require.NoError(t, err)
		require.Contains(t, card, "kind")
	}

	// Idempotent: running again changes nothing (nothing left to encrypt).
	require.NoError(t, Backfill(db))
	again, err := countContacts(t, db, userID)
	require.NoError(t, err)
	require.Equal(t, before, again)
}

func TestBackfill_AlreadyEncryptedRowsUntouched(t *testing.T) {
	db, _ := realDB(t)
	userID := createUser(t, db, "atrest-bf2")

	// One row written through the armed serializer (encrypted on write), one
	// raw plaintext row (pre-backfill state).
	require.NoError(t, db.Table("contacts").Create(map[string]interface{}{
		"user_id": userID, "firstname": "Dan", "how_we_met": "through work",
	}).Error)

	encHowWeMet, err := Encrypt("through work")
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE contacts SET how_we_met = ? WHERE firstname = ?", encHowWeMet, "Dan").Error)

	require.NoError(t, db.Table("contacts").Create(map[string]interface{}{
		"user_id": userID, "firstname": "Eve", "how_we_met": "plaintext row",
	}).Error)

	require.NoError(t, Backfill(db))

	var eveHowWeMet string
	require.NoError(t, db.Table("contacts").Select("how_we_met").Where("firstname = ?", "Eve").Scan(&eveHowWeMet).Error)
	got, err := Decrypt(eveHowWeMet)
	require.NoError(t, err)
	require.Equal(t, "plaintext row", got, "backfilled value decrypts to original")

	var danHowWeMet string
	require.NoError(t, db.Table("contacts").Select("how_we_met").Where("firstname = ?", "Dan").Scan(&danHowWeMet).Error)
	got, err = Decrypt(danHowWeMet)
	require.NoError(t, err)
	require.Equal(t, "through work", got, "already-encrypted row is not double-encrypted")
}

// insertAuditEvent writes one audit_events row via raw SQL, bypassing the
// serializer — the pre-backfill state of an instance that already had audit
// history when at-rest encryption was introduced. Returns the new row's id.
func insertAuditEvent(t *testing.T, db *gorm.DB, userID uint, beforeSnapshot string) int64 {
	t.Helper()
	res := db.Exec(
		"INSERT INTO audit_events "+
			"(created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash) "+
			"VALUES (datetime('now'), datetime('now'), 'contact', 'urn:uuid:audit-fixture', 'update', ?, ?, '', '')",
		userID, beforeSnapshot,
	)
	require.NoError(t, res.Error)
	require.Equal(t, int64(1), res.RowsAffected)
	var id int64
	require.NoError(t, db.Table("audit_events").
		Select("id").Where("user_id = ?", userID).Order("id DESC").Limit(1).Scan(&id).Error)
	return id
}

// auditNoUpdateTriggerExists reports whether the append-only immutability
// trigger is present on audit_events.
func auditNoUpdateTriggerExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?",
		auditEventsNoUpdateTrigger,
	).Scan(&n).Error)
	return n == 1
}

// TestBackfill_AuditEventsBeforeSnapshot_EncryptsThroughImmutabilityTrigger is
// the regression for the upgrade crash-loop: atrest.Backfill's in-place UPDATE
// of a pre-existing plaintext audit_events.before_snapshot was rejected by the
// append-only trigger (SQLite error 1811), so main.go's logger.Fatal fired on
// every boot of any instance that had audit history. Backfill must drop the
// trigger around its own writes and put it back.
func TestBackfill_AuditEventsBeforeSnapshot_EncryptsThroughImmutabilityTrigger(t *testing.T) {
	db, _ := realDB(t)
	userID := createUser(t, db, "atrest-audit")

	require.True(t, auditNoUpdateTriggerExists(t, db),
		"precondition: the real migrated schema ships the append-only trigger")

	const plaintext = `{"firstname":"Bob","lastname":"Vance"}`
	id := insertAuditEvent(t, db, userID, plaintext)

	var before int64
	require.NoError(t, db.Table("audit_events").Count(&before).Error)

	require.NoError(t, Backfill(db),
		"backfill must encrypt audit history, not abort on the append-only trigger")

	var after int64
	require.NoError(t, db.Table("audit_events").Count(&after).Error)
	require.Equal(t, before, after, "backfill only UPDATEs existing rows — the count is preserved")

	var stored string
	require.NoError(t, db.Table("audit_events").
		Select("before_snapshot").Where("id = ?", id).Scan(&stored).Error)
	require.Contains(t, stored, ciphertextPrefix,
		"the plaintext snapshot must be encrypted at rest after backfill")
	got, err := Decrypt(stored)
	require.NoError(t, err)
	require.Equal(t, plaintext, got, "the encrypted snapshot decrypts back to the original")

	require.True(t, auditNoUpdateTriggerExists(t, db),
		"the immutability trigger must be back in place after backfill")
	tamper := db.Exec("UPDATE audit_events SET before_snapshot = 'tampered' WHERE id = ?", id).Error
	require.Error(t, tamper, "audit_events must reject UPDATEs again after the backfill")
	require.Contains(t, tamper.Error(), "append-only")

	// Idempotent: a second run finds nothing to encrypt and still leaves the
	// trigger intact.
	require.NoError(t, Backfill(db))
	require.True(t, auditNoUpdateTriggerExists(t, db))
}

// TestBackfillColumn_AuditEventsWriteBlocked_LeavesTriggerInPlace guards the
// rollback-safety property: a backfill of audit_events that cannot write (here
// the connection is read-only) surfaces the error and leaves the append-only
// trigger untouched — the drop happens inside the same transaction as the
// UPDATEs, so nothing that fails the transaction can leave the table mutable.
func TestBackfillColumn_AuditEventsWriteBlocked_LeavesTriggerInPlace(t *testing.T) {
	db, _ := realDB(t)
	userID := createUser(t, db, "atrest-audit-ro")
	insertAuditEvent(t, db, userID, `{"firstname":"Cara"}`)

	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	err := backfillColumn(db, auditEventsTable, "before_snapshot")
	require.NoError(t, db.Exec("PRAGMA query_only = OFF").Error)

	require.Error(t, err, "a write failure during the audit backfill must surface, not be swallowed")
	require.True(t, auditNoUpdateTriggerExists(t, db),
		"a failed audit backfill must leave the immutability trigger in place")
}

func TestBackfill_NoOpWhenNotArmed(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, Initialize(db, nil)) // no key → pass-through

	require.NoError(t, db.Table("contacts").Create(map[string]interface{}{
		"user_id": 1, "firstname": "Fay", "how_we_met": "stays plaintext",
	}).Error)
	require.NoError(t, Backfill(db))

	var stored string
	require.NoError(t, db.Table("contacts").Select("how_we_met").Where("firstname = ?", "Fay").Scan(&stored).Error)
	require.Equal(t, "stays plaintext", stored, "unarmed backfill must be a no-op")
}

func countContacts(t *testing.T, db *gorm.DB, userID uint) (int64, error) {
	t.Helper()
	var n int64
	err := db.Table("contacts").Where("user_id = ?", userID).Count(&n).Error
	return n, err
}
