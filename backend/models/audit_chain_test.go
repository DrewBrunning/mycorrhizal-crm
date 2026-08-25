package models

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newChainTestDB is newAuditTestDB with a user created for the events.
func newChainTestDB(t *testing.T) (*gorm.DB, User) {
	t.Helper()
	db := newAuditTestDB(t)
	user := User{Username: "chain", Password: "password123!A", Email: "chain@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

// chainEventCount returns the number of audit rows, oldest first.
func chainEventCount(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&AuditEvent{}).Count(&n).Error)
	return int(n)
}

// TestAuditChain_RecorderAppendsLinkedChain is the core contract: events
// recorded through the real hooks chain up (prev_hash of row i+1 == hash of
// row i), the head row uses the genesis prev_hash, and VerifyAuditChain is
// clean.
func TestAuditChain_RecorderAppendsLinkedChain(t *testing.T) {
	db, user := newChainTestDB(t)

	// Drive events through the real recorder path (entity hooks).
	contact := Contact{UserID: user.ID, Firstname: "Chain", Lastname: "Link"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Lastname = "Linked"
	require.NoError(t, db.Save(&contact).Error)
	require.NoError(t, db.Delete(&contact).Error)
	RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLoginFailed, user.ID)
	AuditFlush()

	n := chainEventCount(t, db)
	require.GreaterOrEqual(t, n, 5, "create+update+delete+login+login_failed")

	var events []AuditEvent
	require.NoError(t, db.Order("id asc").Find(&events).Error)

	// Head row uses the genesis prev_hash.
	assert.Equal(t, auditChainGenesis, events[0].PrevHash)
	for i := 1; i < len(events); i++ {
		assert.Equal(t, events[i-1].Hash, events[i].PrevHash,
			"event %d must link to event %d's hash", events[i].ID, events[i-1].ID)
		assert.NotEmpty(t, events[i].Hash, "every event must carry a hash")
	}
	for i, e := range events {
		prev := auditChainGenesis
		if i > 0 {
			prev = events[i-1].Hash
		}
		assert.Equal(t, AuditChainHash(prev, &e), e.Hash, "stored hash must be reproducible")
	}

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "an untouched recorder-built chain must verify clean")
}

// TestAuditChain_ConcurrentAppendsDoNotFork verifies that many concurrent
// records serialize onto one linear chain (no duplicate prev_hash, verify
// clean).
func TestAuditChain_ConcurrentAppendsDoNotFork(t *testing.T) {
	db, user := newChainTestDB(t)

	const n = 40
	for i := 0; i < n; i++ {
		go RecordAuditEvent(AuditEntityAuth, fmt.Sprintf("u%d", i), AuditOpLogin, user.ID)
	}
	AuditFlush()

	var events []AuditEvent
	require.NoError(t, db.Order("id asc").Find(&events).Error)
	require.Equal(t, n, len(events))

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "concurrent appends must chain linearly")
}

// TestAuditChain_DetectsContentEdit drops the immutability trigger (as an
// attacker with raw DB access would), edits a row's content, and asserts the
// verifier flags exactly that row as a hash mismatch.
func TestAuditChain_DetectsContentEdit(t *testing.T) {
	db, user := newChainTestDB(t)
	for i := 0; i < 3; i++ {
		RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	}
	AuditFlush()

	require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS audit_events_no_update").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE audit_events SET entity_id = 'tampered' WHERE id = (SELECT id FROM audit_events ORDER BY id ASC LIMIT 1)")
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed'); END").Error)

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.Len(t, gaps, 1, "a content edit must produce exactly one (first) gap")
	assert.Contains(t, gaps[0].Message, "hash mismatch")
	assert.EqualValues(t, 1, gaps[0].EventID)
}

// TestAuditChain_DetectsDeletion deletes a middle row and asserts the
// verifier flags the row after the hole as a prev_hash mismatch.
func TestAuditChain_DetectsDeletion(t *testing.T) {
	db, user := newChainTestDB(t)
	for i := 0; i < 3; i++ {
		RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	}
	AuditFlush()

	require.NoError(t, db.Exec("DELETE FROM audit_events WHERE id = (SELECT id FROM audit_events ORDER BY id ASC LIMIT 1)").Error)

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Contains(t, gaps[0].Message, "prev_hash mismatch")
	assert.EqualValues(t, 2, gaps[0].EventID, "the row following the deletion must be the first gap")
}

// TestAuditChain_DetectsInsertion inserts a forged row in the middle and
// asserts the verifier flags it.
func TestAuditChain_DetectsInsertion(t *testing.T) {
	db, user := newChainTestDB(t)
	for i := 0; i < 3; i++ {
		RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	}
	AuditFlush()

	forged := AuditEvent{
		EntityType: AuditEntityAuth,
		EntityID:   "forged",
		Operation:  AuditOpLogin,
		UserID:     user.ID,
		Hash:       "deadbeef",
	}
	require.NoError(t, db.Create(&forged).Error)

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.EqualValues(t, forged.ID, gaps[0].EventID, "the inserted row itself must be the first gap")
	assert.Contains(t, gaps[0].Message, "hash mismatch")
}

// TestAuditChain_BackfillLegacyRows covers pre-000033 rows: events written
// directly (no hash) are backfilled by RecomputeAuditChain, the chain becomes
// valid, and the immutability trigger is left in force.
func TestAuditChain_BackfillLegacyRows(t *testing.T) {
	db, user := newChainTestDB(t)

	// Rows written the way pre-000033 code wrote them: direct Create, no hash.
	legacy := []AuditEvent{
		{EntityType: AuditEntityContact, EntityID: "a", Operation: AuditOpCreate, UserID: user.ID, BeforeSnapshot: "{}"},
		{EntityType: AuditEntityContact, EntityID: "a", Operation: AuditOpUpdate, UserID: user.ID, BeforeSnapshot: `{"x":1}`},
		{EntityType: AuditEntityNote, EntityID: "7", Operation: AuditOpDelete, UserID: user.ID, BeforeSnapshot: `{"y":2}`},
	}
	for _, e := range legacy {
		require.NoError(t, db.Create(&e).Error)
	}

	// Not backfilled yet: the verifier says "backfill pending".
	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Contains(t, gaps[0].Message, "backfill pending")

	require.NoError(t, RecomputeAuditChain(db))

	gaps, err = VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "after backfill the chain must verify clean")

	// Idempotent: a second run is a no-op and the trigger still rejects UPDATE.
	require.NoError(t, RecomputeAuditChain(db))
	require.Error(t, db.Model(&AuditEvent{}).Where("entity_id = ?", "a").Update("entity_id", "b").Error,
		"the immutability trigger must still reject UPDATE after backfill")
}

// TestAuditChain_RecomputeRelinksAfterPurge deletes the head of a valid chain
// (the retention purge's sanctioned DELETE) and asserts that RecomputeAuditChain
// re-links the survivors into a clean chain, while verify WITHOUT the recompute
// reports the break.
func TestAuditChain_RecomputeRelinksAfterPurge(t *testing.T) {
	db, user := newChainTestDB(t)
	for i := 0; i < 3; i++ {
		RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	}
	AuditFlush()

	require.NoError(t, db.Exec("DELETE FROM audit_events WHERE id < (SELECT MAX(id) FROM audit_events)").Error)

	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.NotEmpty(t, gaps, "a purge without re-link must be detected")

	require.NoError(t, RecomputeAuditChain(db))
	gaps, err = VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "recompute must re-link the survivors into a clean chain")

	// The new head row uses the genesis prev_hash (the old head is gone).
	var head AuditEvent
	require.NoError(t, db.Order("id asc").First(&head).Error)
	assert.Equal(t, auditChainGenesis, head.PrevHash)
}

// TestAuditChain_VerifyIsReadOnly asserts VerifyAuditChain never repairs or
// writes: after verification, a tampered row is still flagged on the second
// run too (no laundering).
func TestAuditChain_VerifyIsReadOnly(t *testing.T) {
	db, user := newChainTestDB(t)
	RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	RecordAuditEvent(AuditEntityAuth, "alice", AuditOpLogin, user.ID)
	AuditFlush()

	require.NoError(t, db.Exec("DROP TRIGGER IF EXISTS audit_events_no_update").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE audit_events SET operation = 'delete' WHERE id = (SELECT MAX(id) FROM audit_events)")
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed'); END").Error)

	gaps1, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.NotEmpty(t, gaps1)
	gaps2, err := VerifyAuditChain(db)
	require.NoError(t, err)
	require.NotEmpty(t, gaps2, "verification must not launder a tampered chain")
}
