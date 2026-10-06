package main

import (
	"testing"

	"mycorrhizal/atrest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1489: against a database whose contact cards are encrypted at rest
// (every production instance) the doctor must arm the same key the server uses,
// or it reports every contact as malformed.
func TestDoctor_EncryptedCardsAreReadWithTheServersKey(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "doctor-atrest-test-secret-key-32+chars-long")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Cleanup(atrest.ResetForTest)

	path := migratedDBFile(t, func(db *gorm.DB) {
		kek, err := atrest.EncryptionKey()
		require.NoError(t, err)
		require.NoError(t, atrest.Initialize(db, kek))
		seedUserContact(t, db) // saved through the serializer: card is ciphertext
		var raw string
		require.NoError(t, db.Raw("SELECT card FROM contacts LIMIT 1").Scan(&raw).Error)
		require.Contains(t, raw, "encv1:", "the fixture must really be encrypted")
		atrest.ResetForTest()
	})

	code, out, _ := run(t, "-db", path)
	assert.Equal(t, 0, code, out)
	assert.Contains(t, out, "result: OK")
}

func TestDoctor_BadEncryptionKeyIsAnOperationalError(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "not-base64-32-bytes")
	path := migratedDBFile(t, nil)
	code, _, errs := run(t, "-db", path)
	assert.Equal(t, 2, code)
	assert.Contains(t, errs, "at-rest master key")
}
