package models

import (
	"errors"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1547: the async recorder must persist an event only if the audited
// write's transaction commits.

func asyncAuditDB(t *testing.T) (*gorm.DB, AuditRecorder, User) {
	t.Helper()
	db := dbtest.New(t, dbtest.WithAsyncAudit())
	u := mustUser(t, db, "tx-user")
	// Trigger the lazy recorder install, then grab it.
	RecordAuditEvent(db, AuditEntityAuth, "seed", AuditOpLogin, u.ID)
	rec := AuditRecorderFor(db)
	require.NotNil(t, rec)
	rec.Flush()
	base := auditCount(t, db)
	require.EqualValues(t, 1, base)
	return db, rec, u
}

func TestAuditRecorder_AsyncRolledBackTransactionLeavesNoEvent(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&Contact{UserID: u.ID, Firstname: "Ghost"}).Error)
		return errors.New("boom")
	})
	require.Error(t, err)
	rec.Flush()
	assert.EqualValues(t, 1, auditCount(t, db), "rolled-back write must leave no audit row")
}

func TestAuditRecorder_AsyncCommittedTransactionRecordsExactlyOneEvent(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	c := Contact{UserID: u.ID, Firstname: "Real"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return tx.Create(&c).Error }))
	rec.Flush()
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, c.VCardUID))
	assert.EqualValues(t, 2, auditCount(t, db))
	gaps, err := VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps)
}

func TestAuditRecorder_AsyncImplicitTransactionStillRecords(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	c := Contact{UserID: u.ID, Firstname: "Implicit"}
	require.NoError(t, db.Create(&c).Error)
	rec.Flush()
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, c.VCardUID))
}

func TestAuditRecorder_AsyncFailedImplicitWriteLeavesNoEvent(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	// A BeforeSave-passing row whose INSERT fails (duplicate unique vcard_uid):
	// the hook never reaches AfterCreate, and nothing may be recorded.
	first := Contact{UserID: u.ID, Firstname: "Dup"}
	require.NoError(t, db.Create(&first).Error)
	rec.Flush()
	before := auditCount(t, db)
	dup := Contact{UserID: u.ID, Firstname: "Dup2", VCardUID: first.VCardUID}
	require.Error(t, db.Create(&dup).Error)
	rec.Flush()
	assert.Equal(t, before, auditCount(t, db))
}

func TestAuditRecorder_AsyncUpdateAndDeleteRollBackToo(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	c := Contact{UserID: u.ID, Firstname: "Before"}
	require.NoError(t, db.Create(&c).Error)
	rec.Flush()
	before := auditCount(t, db)

	_ = db.Transaction(func(tx *gorm.DB) error {
		c.Firstname = "After"
		require.NoError(t, tx.Save(&c).Error)
		require.NoError(t, tx.Delete(&c).Error)
		return errors.New("boom")
	})
	rec.Flush()
	assert.Equal(t, before, auditCount(t, db))
}

func TestAuditRecorder_AsyncNestedTransactionRollbackDropsOnlyInnerEvents(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	outer := Contact{UserID: u.ID, Firstname: "Outer"}
	inner := Contact{UserID: u.ID, Firstname: "Inner"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&outer).Error)
		_ = tx.Transaction(func(tx2 *gorm.DB) error {
			require.NoError(t, tx2.Create(&inner).Error)
			return errors.New("inner boom")
		})
		return nil
	}))
	rec.Flush()
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, outer.VCardUID))
	assert.EqualValues(t, 0, countAuditEvents(t, db, AuditEntityContact, inner.VCardUID), "rolled-back savepoint must drop its events")
}

func TestAuditRecorder_AsyncNestedTransactionCommitKeepsBoth(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	outer := Contact{UserID: u.ID, Firstname: "Outer"}
	inner := Contact{UserID: u.ID, Firstname: "Inner"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&outer).Error)
		return tx.Transaction(func(tx2 *gorm.DB) error { return tx2.Create(&inner).Error })
	}))
	rec.Flush()
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, outer.VCardUID))
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, inner.VCardUID))
}

func TestAuditRecorder_AsyncManualBeginRollbackAndCommit(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	ghost := Contact{UserID: u.ID, Firstname: "Ghost"}
	require.NoError(t, tx.Create(&ghost).Error)
	require.NoError(t, tx.Rollback().Error)

	tx = db.Begin()
	require.NoError(t, tx.Error)
	real := Contact{UserID: u.ID, Firstname: "Real"}
	require.NoError(t, tx.Create(&real).Error)
	require.NoError(t, tx.Commit().Error)
	rec.Flush()
	assert.EqualValues(t, 0, countAuditEvents(t, db, AuditEntityContact, ghost.VCardUID))
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, real.VCardUID))
}

func TestAuditRecorder_AsyncRecordAuditEventOutsideTransactionUnchanged(t *testing.T) {
	db, rec, u := asyncAuditDB(t)
	RecordAuditEvent(db, AuditEntityAuth, "x", AuditOpLogin, u.ID)
	rec.Flush()
	assert.EqualValues(t, 2, auditCount(t, db))
}

func TestNewAuditRecorder_AsyncWrapsPoolSoDBStillWorks(t *testing.T) {
	db := dbtest.New(t)
	rec := NewAuditRecorder(db)
	u := mustUser(t, db, "wrapped")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	c := Contact{UserID: u.ID, Firstname: "W"}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return tx.Create(&c).Error }))
	rec.Flush()
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, c.VCardUID))
}
