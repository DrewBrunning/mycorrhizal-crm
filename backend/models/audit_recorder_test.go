package models

import (
	"fmt"
	"sync"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mustUser(t *testing.T, db *gorm.DB, name string) User {
	t.Helper()
	u := User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func auditCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&AuditEvent{}).Count(&n).Error)
	return n
}

// TestAuditRecorder_TwoRecordersDoNotInterfere is the #1493 isolation proof:
// two recorders bound to two DBs, driven concurrently from parallel subtests,
// each see exactly (and only) their own events. With the old process-global
// recorder the second RegisterAuditDB redirected the first test's writes.
func TestAuditRecorder_TwoRecordersDoNotInterfere(t *testing.T) {
	const perDB = 15
	type env struct {
		db   *gorm.DB
		rec  AuditRecorder
		user User
	}
	mk := func(name string, opts ...AuditOption) env {
		db := dbtest.New(t)
		return env{db: db, rec: NewAuditRecorder(db, opts...), user: mustUser(t, db, name)}
	}
	a := mk("iso-a", WithSync())
	b := mk("iso-b") // async: exercises Flush too

	var wg sync.WaitGroup
	for _, e := range []env{a, b} {
		wg.Add(1)
		go func(e env) {
			defer wg.Done()
			for i := 0; i < perDB; i++ {
				RecordAuditEvent(e.db, AuditEntityAuth, fmt.Sprintf("%d", e.user.ID), AuditOpLogin, e.user.ID)
			}
		}(e)
	}
	wg.Wait()
	a.rec.Flush()
	b.rec.Flush()

	// user-create events don't exist (User is not an audited model), so each
	// DB holds exactly its own perDB login events.
	assert.EqualValues(t, perDB, auditCount(t, a.db))
	assert.EqualValues(t, perDB, auditCount(t, b.db))
	for _, e := range []env{a, b} {
		var foreign int64
		require.NoError(t, e.db.Model(&AuditEvent{}).Where("user_id <> ?", e.user.ID).Count(&foreign).Error)
		assert.Zero(t, foreign, "no event from the other recorder may land here")
		gaps, err := VerifyAuditChain(e.db)
		require.NoError(t, err)
		assert.Empty(t, gaps)
	}
	assert.NotSame(t, AuditRecorderFor(a.db), AuditRecorderFor(b.db))
}

// TestAuditRecorder_ParallelTestsHaveOwnRecorders runs many parallel subtests
// that each own a DB; any shared recorder state would cross-contaminate counts.
func TestAuditRecorder_ParallelTestsHaveOwnRecorders(t *testing.T) {
	for i := 0; i < 6; i++ {
		i := i
		t.Run(fmt.Sprintf("p%d", i), func(t *testing.T) {
			t.Parallel()
			db := newAuditTestDB(t)
			u := mustUser(t, db, fmt.Sprintf("par%d", i))
			for j := 0; j <= i; j++ {
				c := Contact{UserID: u.ID, Firstname: "P"}
				require.NoError(t, db.Create(&c).Error)
			}
			assert.EqualValues(t, i+1, auditCount(t, db))
		})
	}
}

// TestAuditRecorder_UnregisteredDBStillRecordsInTests pins item 4 of #1493: a
// test DB with no explicit recorder can no longer make audit assertions pass
// vacuously — the first audited write installs a sync recorder on that DB, so
// the event really exists.
func TestAuditRecorder_UnregisteredDBStillRecordsInTests(t *testing.T) {
	db := dbtest.New(t)
	require.Nil(t, AuditRecorderFor(db), "precondition: no recorder yet")
	u := mustUser(t, db, "lazy")
	c := Contact{UserID: u.ID, Firstname: "Lazy"}
	require.NoError(t, db.Create(&c).Error)

	assert.NotNil(t, AuditRecorderFor(db))
	assert.EqualValues(t, 1, countAuditEvents(t, db, AuditEntityContact, c.VCardUID))

	// RecordAuditEvent on a DB without a recorder does the same.
	db2 := dbtest.New(t)
	u2 := mustUser(t, db2, "lazy2")
	RecordAuditEvent(db2, AuditEntityAuth, "x", AuditOpLogin, u2.ID)
	assert.EqualValues(t, 1, auditCount(t, db2))
}

func TestAuditRecorder_DisableAuditDropsEvents(t *testing.T) {
	db := dbtest.New(t)
	DisableAudit(db)
	u := mustUser(t, db, "quiet")
	require.NoError(t, db.Create(&Contact{UserID: u.ID, Firstname: "Quiet"}).Error)
	RecordAuditEvent(db, AuditEntityAuth, "x", AuditOpLogin, u.ID)
	assert.Zero(t, auditCount(t, db))

	rec := AuditRecorderFor(db)
	require.NotNil(t, rec)
	assert.Equal(t, auditPluginName, rec.Name())
	assert.NoError(t, rec.Initialize(db))
	rec.Flush()

	// Re-arming replaces the no-op recorder.
	NewAuditRecorder(db, WithSync())
	RecordAuditEvent(db, AuditEntityAuth, "x", AuditOpLogin, u.ID)
	assert.EqualValues(t, 1, auditCount(t, db))
}

func TestAuditRecorder_NilAndBareDBAreSafe(t *testing.T) {
	assert.Nil(t, AuditRecorderFor(nil))
	assert.Nil(t, AuditRecorderFor(&gorm.DB{}))
	assert.NotPanics(t, func() {
		RecordAuditEvent(nil, AuditEntityAuth, "x", AuditOpLogin, 1)
		RecordAuditEvent(&gorm.DB{}, AuditEntityAuth, "x", AuditOpLogin, 1)
		installAuditRecorder(nil, noopAuditRecorder{})
		installAuditRecorder(&gorm.DB{}, noopAuditRecorder{})
	})
	// A recorder with no DB records nothing, sync or async.
	assert.NotPanics(t, func() {
		(&auditLogger{sync: true}).Record(nil, AuditEntityAuth, "x", AuditOpLogin, 1, "")
		(&auditLogger{}).Record(nil, AuditEntityAuth, "x", AuditOpLogin, 1, "")
	})
}

// TestAuditRecorder_InstallInitialisesNilPluginMap covers a Config whose
// Plugins map was never initialised.
func TestAuditRecorder_InstallInitialisesNilPluginMap(t *testing.T) {
	db := dbtest.New(t)
	db.Config.Plugins = nil
	NewAuditRecorder(db, WithSync())
	assert.NotNil(t, AuditRecorderFor(db))

}

// TestAuditRecorder_SyncJoinsTheWritersTransaction documents the sync-mode
// contract: the event commits (or rolls back) with the audited write, and
// every Session/Transaction derived from the DB resolves the same recorder.
func TestAuditRecorder_SyncJoinsTheWritersTransaction(t *testing.T) {
	db := newAuditTestDB(t)
	u := mustUser(t, db, "txn")

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		assert.Same(t, AuditRecorderFor(db), AuditRecorderFor(tx))
		return tx.Create(&Contact{UserID: u.ID, Firstname: "Committed"}).Error
	}))
	assert.EqualValues(t, 1, auditCount(t, db))

	_ = db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&Contact{UserID: u.ID, Firstname: "RolledBack"}).Error)
		return fmt.Errorf("abort")
	})
	assert.EqualValues(t, 1, auditCount(t, db), "rolled-back write takes its sync audit row with it")

	sess := db.Session(&gorm.Session{NewDB: true})
	require.NoError(t, sess.Create(&Contact{UserID: u.ID, Firstname: "Session"}).Error)
	assert.EqualValues(t, 2, auditCount(t, db))
}

// TestAuditRecorder_FailedWritesAreCounted makes the #1471 failure mode
// observable: an audit insert that violates the FK is counted, not just
// logged.
func TestAuditRecorder_FailedWritesAreCounted(t *testing.T) {
	db := dbtest.New(t)
	rec := NewAuditRecorder(db, WithSync()).(*auditLogger)
	u := mustUser(t, db, "fails")
	RecordAuditEvent(db, AuditEntityAuth, "ok", AuditOpLogin, u.ID)
	assert.Zero(t, rec.FailedWrites())
	RecordAuditEvent(db, AuditEntityAuth, "bad", AuditOpLogin, 987654)
	assert.EqualValues(t, 1, rec.FailedWrites())
}

func TestAuditRecorder_RecomputeWithoutRecorderStillWorks(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, RecomputeAuditChain(db))
	assert.NoError(t, RecomputeAuditChain(nil))
}
