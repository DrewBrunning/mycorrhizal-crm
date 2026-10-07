package models

import (
	"sync"
	"sync/atomic"
	"time"

	"mycorrhizal/internal/auditwire"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// Injected audit recorder (issue #1493).
//
// The recorder used to be a process-global singleton (`var auditRecorder`)
// swapped by RegisterAuditDB, so every test shared one recorder, had to undo
// its registration, and could pass vacuously when it forgot to register. It is
// now an object bound to a *gorm.DB: NewAuditRecorder installs it on that DB's
// gorm.Config.Plugins map, and the audit hooks resolve it from the `tx` they
// are handed. gorm.Config is shared by every Session/Transaction derived from
// the DB, so the hooks always find the recorder of the DB they are writing
// through and never another instance's. Production wires exactly one recorder
// in embedded.Start; each test owns its own.

// auditPluginName is the key the recorder is stored under in
// gorm.Config.Plugins. It is a lookup key, not a registration with GORM's
// plugin machinery (Initialize is a no-op).
const auditPluginName = "mycorrhizal:audit_recorder"

// AuditRecorder persists audit events. It implements gorm.Plugin only so it
// can live in gorm.Config.Plugins; NewAuditRecorder installs it.
type AuditRecorder interface {
	gorm.Plugin
	// Record appends one event. tx is the handle of the write being audited
	// (a sync recorder inserts through it so the row joins the same
	// transaction; an async recorder ignores it).
	Record(tx *gorm.DB, entityType, entityID, operation string, userID uint, snapshotJSON string)
	// Flush blocks until every in-flight write has completed (a no-op for a
	// sync recorder, whose writes are already done when Record returns).
	Flush()
}

// AuditOption configures NewAuditRecorder.
type AuditOption func(*auditLogger)

// WithSync makes Record persist inline instead of in a goroutine, so a test
// can read audit rows with no Flush and no ordering assumptions. The insert
// goes through the audited write's own handle, hence joins its transaction
// (an async insert from another connection would wait on that transaction's
// SQLite write lock forever). Production keeps the default async mode, where
// an audit failure can never roll back the real write.
func WithSync() AuditOption { return func(a *auditLogger) { a.sync = true } }

// auditPluginsMu guards every read/write of gorm.Config.Plugins performed by
// this file (GORM itself only touches that map from db.Use at setup time).
var auditPluginsMu sync.RWMutex

// auditLogger is the concrete recorder. Async mode (default) persists on a
// separate goroutine on its own session (never the hook's transaction), so an
// audit failure can never roll back the real write.
//
// chainMu serializes the read-prev-hash + insert sequence so concurrent audit
// writes append to the hash chain in a deterministic id order instead of
// forking it. RecomputeAuditChain takes the same lock, so it can never
// interleave with a live append.
type auditLogger struct {
	mu      sync.RWMutex
	db      *gorm.DB
	sync    bool
	wg      sync.WaitGroup
	chainMu sync.Mutex
	failed  atomic.Int64
}

// NewAuditRecorder builds a recorder bound to db and installs it as db's audit
// recorder, replacing any previous one. Hooks fired through db (or any Session
// / Transaction derived from it) record through it.
func NewAuditRecorder(db *gorm.DB, opts ...AuditOption) AuditRecorder {
	a := &auditLogger{db: db}
	for _, o := range opts {
		o(a)
	}
	installAuditRecorder(db, a)
	return a
}

func installAuditRecorder(db *gorm.DB, rec AuditRecorder) {
	if db == nil || db.Config == nil {
		return
	}
	auditPluginsMu.Lock()
	defer auditPluginsMu.Unlock()
	setAuditRecorderLocked(db, rec)
}

// setAuditRecorderLocked stores rec; the caller holds auditPluginsMu.
func setAuditRecorderLocked(db *gorm.DB, rec AuditRecorder) {
	if db.Plugins == nil {
		db.Plugins = map[string]gorm.Plugin{}
	}
	db.Plugins[auditPluginName] = rec
}

// AuditRecorderFor returns the recorder installed on db, or nil.
func AuditRecorderFor(db *gorm.DB) AuditRecorder {
	if db == nil || db.Config == nil {
		return nil
	}
	auditPluginsMu.RLock()
	defer auditPluginsMu.RUnlock()
	rec, _ := db.Plugins[auditPluginName].(AuditRecorder)
	return rec
}

// DisableAudit installs a recorder that drops every event, for a test that
// deliberately writes audited rows without wanting audit side effects (e.g.
// seeding before the action under test). It is explicit so that "no recorder"
// can never be a silent default; see recordAudit.
func DisableAudit(db *gorm.DB) { installAuditRecorder(db, noopAuditRecorder{}) }

type noopAuditRecorder struct{}

func (noopAuditRecorder) Name() string                                          { return auditPluginName }
func (noopAuditRecorder) Initialize(*gorm.DB) error                             { return nil }
func (noopAuditRecorder) Record(*gorm.DB, string, string, string, uint, string) {}
func (noopAuditRecorder) Flush()                                                {}

// Name and Initialize satisfy gorm.Plugin.
func (a *auditLogger) Name() string              { return auditPluginName }
func (a *auditLogger) Initialize(*gorm.DB) error { return nil }

// Flush blocks until every in-flight audit write has completed. Safe to call
// at shutdown.
//
// It holds a.mu across the Wait so a concurrent Record can never Add to the
// WaitGroup while Wait is running: the WaitGroup contract forbids an Add that
// starts at a zero counter concurrently with Wait (a data race that can lose a
// wakeup at shutdown). Serializing Add and Wait on a.mu makes every Add that
// began before the flush happen-before the Wait, which is what makes the drain
// total rather than best-effort.
func (a *auditLogger) Flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.wg.Wait()
}

// FailedWrites reports how many audit inserts this recorder has failed to
// persist (each also logs a WARN). Lets a test assert audit writes are not
// failing silently (the #1471 failure mode).
func (a *auditLogger) FailedWrites() int64 { return a.failed.Load() }

// Record queues (async) or performs (sync) one audit append. snapshotJSON is
// the redacted before-state ("" for creates). Never returns an error and never
// blocks the hook beyond launching the goroutine.
func (a *auditLogger) Record(tx *gorm.DB, entityType, entityID, operation string, userID uint, snapshotJSON string) {
	if a.sync {
		db := a.db
		if tx != nil && tx.Statement != nil {
			db = tx.Session(&gorm.Session{NewDB: true})
		}
		if db == nil {
			return
		}
		a.persist(db, entityType, entityID, operation, userID, snapshotJSON)
		return
	}

	// The db-nil check and wg.Add(1) run atomically under a.mu so an Add can
	// never start while Flush's Wait is in progress. The goroutine only
	// touches chainMu and the DB session, never a.mu, so holding the lock
	// across the drain cannot deadlock.
	a.mu.Lock()
	db := a.db
	if db == nil {
		a.mu.Unlock()
		return
	}
	a.wg.Add(1)
	a.mu.Unlock()

	// Tracked by fireandforget too, so a test harness's drain (internal/dbtest
	// waits on it before closing the DB) covers in-flight audit writes the
	// same way it covers webhook deliveries and session touches (issue #703).
	fireandforget.Run(func() {
		defer a.wg.Done()
		a.persist(db, entityType, entityID, operation, userID, snapshotJSON)
	})
}

// persist appends one event to the hash chain. The chain append (read previous
// row's hash, compute this row's, insert) is serialized on chainMu.
func (a *auditLogger) persist(db *gorm.DB, entityType, entityID, operation string, userID uint, snapshotJSON string) {
	a.chainMu.Lock()
	defer a.chainMu.Unlock()

	event := AuditEvent{
		EntityType:     entityType,
		EntityID:       entityID,
		Operation:      operation,
		UserID:         userID,
		BeforeSnapshot: snapshotJSON,
		// Explicit UTC timestamp so the chain hash is deterministic at
		// insert time (the row can never be updated afterwards — the
		// immutability trigger and the chain would both reject it).
		// Truncated to microseconds to match what SQLite round-trips and
		// what chainContent hashes, so write-time and verify-time
		// computations always agree.
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	prev := ""
	var last struct{ Hash string }
	if err := db.Model(&AuditEvent{}).Order("id desc").Limit(1).Scan(&last).Error; err != nil {
		logger.Warn().Err(err).Msg("audit: failed to read last chain hash, appending from genesis")
	} else {
		prev = last.Hash
	}
	event.PrevHash = prev
	event.Hash = AuditChainHash(prev, &event)

	if err := db.Create(&event).Error; err != nil {
		a.failed.Add(1)
		logger.Warn().Err(err).
			Str("entity_type", entityType).Str("entity_id", entityID).Str("operation", operation).
			Msg("audit: failed to persist audit event (real write is unaffected)")
	}
}

// recordAudit resolves db's recorder and records through it.
//
// With no recorder installed the event is skipped silently — production
// (embedded.Start installs one right after the DB opens) has no other
// no-recorder window than the startup path before wiring, and a DB opened
// without one (a scratch restore-drill copy, a CLI tool) deliberately records
// nothing. The one exception is a DB a test harness explicitly armed with an
// auditwire.Default marker (internal/dbtest does, for every DB it hands out):
// there a silent no-op would make audit assertions pass vacuously, so a
// recorder is installed on first use, bound to this DB alone — synchronous by
// default, or the production-shaped async recorder when the marker asks for
// it. A test that wants no audit calls DisableAudit(db).
func recordAudit(db *gorm.DB, entityType, entityID, operation string, userID uint, snapshotJSON string) {
	rec := AuditRecorderFor(db)
	if rec == nil {
		marker := auditwire.For(db)
		if marker == nil {
			return
		}
		auditPluginsMu.Lock()
		if cur, ok := db.Plugins[auditPluginName].(AuditRecorder); ok {
			rec = cur // lost a race with a concurrent first write
		} else {
			if marker.Async {
				rec = &auditLogger{db: marker.Root()}
			} else {
				rec = &auditLogger{sync: true}
			}
			setAuditRecorderLocked(db, rec)
		}
		auditPluginsMu.Unlock()
	}
	rec.Record(db, entityType, entityID, operation, userID, snapshotJSON)
}
