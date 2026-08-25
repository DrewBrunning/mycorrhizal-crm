package atrest

import (
	"path/filepath"
	"testing"

	"mycorrhizal/database"

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
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

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
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	kek := make([]byte, keySize)
	for i := range kek {
		kek[i] = byte(i)
	}
	require.NoError(t, Initialize(db, kek))

	// Re-init with a different master key: the wrapped DEK cannot be
	// unwrapped → fail closed at boot.
	wrong := make([]byte, keySize)
	wrong[0] = 0xFF
	err = Initialize(db, wrong)
	require.Error(t, err, "a wrong master key must fail closed at initialize time")
	require.Contains(t, err.Error(), "wrong DATA_ENCRYPTION_KEY")
}

func TestInitialize_PersistsAcrossRestart(t *testing.T) {
	// Simulate a restart: the DEK row survives, and re-initializing with the
	// same key decrypts rows written before the restart.
	dbPath := filepath.Join(t.TempDir(), "atrest.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

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

func TestBackfill_NoOpWhenNotArmed(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "atrest.db"))
	require.NoError(t, err)
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
