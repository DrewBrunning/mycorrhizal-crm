package services

import (
	"testing"

	"mycorrhizal/atrest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1489: the INV-D8 check reads contacts.card with raw SQL, so with
// at-rest encryption armed (every production deployment) it saw ciphertext and
// reported EVERY contact as "not valid JSON". It was found by running the
// current code over data a real v1.0.0 instance wrote.
func TestDataIntegrity_INV_D8_EncryptedCardIsNotMalformed(t *testing.T) {
	db, cfg := integrityTestDB(t)
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = byte(0x11 + i)
	}
	require.NoError(t, atrest.Initialize(db, kek))
	t.Cleanup(atrest.ResetForTest)

	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")

	sealed, err := atrest.Encrypt(`{"name":{"components":[{"kind":"given","value":"A"}]}}`)
	require.NoError(t, err)
	require.Contains(t, sealed, "encv1:")
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", sealed, c.ID).Error)

	r := runDataChecks(t, db, cfg)
	_, flagged := findingFor(r, "canonical_record.invalid_json", u.ID)
	assert.False(t, flagged, "a valid encrypted card must not be reported as invalid JSON: %+v", r.Findings)

	// An empty-object card behind encryption is still "no card", not malformed.
	emptySealed, err := atrest.Encrypt(`{}`)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", emptySealed, c.ID).Error)
	r = runDataChecks(t, db, cfg)
	_, flagged = findingFor(r, "canonical_record.invalid_json", u.ID)
	assert.False(t, flagged)

	// Ciphertext the key cannot open is still a violation.
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", "encv1:main:bm90LXJlYWwtY2lwaGVydGV4dA", c.ID).Error)
	r = runDataChecks(t, db, cfg)
	_, flagged = findingFor(r, "canonical_record.invalid_json", u.ID)
	assert.True(t, flagged, "unopenable ciphertext must stay a violation")
}
