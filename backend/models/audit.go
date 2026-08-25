package models

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// Audit operation tokens stored on AuditEvent.Operation. The first three are
// entity CRUD; the rest are the auth/admin lifecycle vocabulary added by issue
// #381 (ASVS V7.3). The set is pinned by migration 000034's CHECK constraint —
// a token added here without a migration is a silent INSERT failure.
const (
	AuditOpCreate         = "create"
	AuditOpUpdate         = "update"
	AuditOpDelete         = "delete"
	AuditOpLogin          = "login"
	AuditOpLoginFailed    = "login_failed"
	AuditOpRegister       = "register"
	AuditOpPasswordChange = "password_change"
	AuditOpPasswordReset  = "password_reset"
	AuditOpTOTPEnable     = "totp_enable"
	AuditOpTOTPDisable    = "totp_disable"
	AuditOpRecoveryRegen  = "recovery_regenerate"
	AuditOpRevoke         = "revoke"
	AuditOpRoleChange     = "role_change"
)

// AuditEntityType tokens stored on AuditEvent.EntityType. The first group are
// entity CRUD; the "user"/"auth"/"api_token" group are the auth/admin
// lifecycle entities added by issue #381. Mirrored by hand in the frontend
// (frontend/src/api/audit.ts) and openapi.yaml's AuditEvent enum — see
// CLAUDE.md frontend trap #4.
const (
	AuditEntityContact   = "contact"
	AuditEntityNote      = "note"
	AuditEntityActivity  = "activity"
	AuditEntityLifeEvent = "life_event"
	AuditEntityGift      = "gift"
	AuditEntityCircle    = "circle"
	AuditEntityTag       = "tag"
	AuditEntityHousehold = "household"
	AuditEntityReminder  = "reminder"

	AuditEntityUser     = "user"
	AuditEntityAuth     = "auth"
	AuditEntityAPIToken = "api_token"
)

// AuditEvent is one immutable create/update/delete record for an audited
// entity (T18, T18), feeding undo,
// sync, and debugging. Append-only by construction: it has no update/delete
// receiver methods, and migration 000016's BEFORE UPDATE / BEFORE DELETE
// triggers hard-reject any such write at the DB level.
//
// Hard-delete (no deleted_at) per T26: system-generated, not user-authored —
// rows are removed only by the retention purge job (AUDIT_RETENTION_DAYS).
// BeforeSnapshot holds redacted JSON of the pre-update/pre-delete state.
type AuditEvent struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	EntityType string `gorm:"not null;index" json:"entity_type"`
	EntityID   string `gorm:"not null;index" json:"entity_id"`
	Operation  string `gorm:"not null" json:"operation"`
	UserID     uint   `gorm:"not null;index" json:"-"`
	// BeforeSnapshot is redacted JSON (auditDenyList applied). Empty for
	// create events.
	BeforeSnapshot string `gorm:"column:before_snapshot;type:text;serializer:encrypted" json:"before_snapshot,omitempty"`
	// Hash is the SHA-256 of (prev_hash || canonical event content); PrevHash
	// is the Hash of the preceding row ("" for the head of the chain). Together
	// they make the log tamper-evident (issue #381): VerifyAuditChain
	// recomputes the chain and flags any insert/delete/reorder/edit. Both are
	// maintained by the recorder at insert and by RecomputeAuditChain (startup
	// backfill + retention purge re-link). The hash is computed over the
	// logical (decrypted) BeforeSnapshot value — GORM's serializer decrypts it
	// transparently on read, so the chain stays valid whether or not at-rest
	// encryption (issue #380) is armed.
	Hash     string `gorm:"not null;default:''" json:"hash"`
	PrevHash string `gorm:"not null;default:''" json:"prev_hash"`
}

// auditDenyList is the field-name deny-list applied to every audit snapshot:
// credentials and secrets must never become a secondary copy in the audit
// log. Checked case-insensitively at any depth (see redactJSON).
var auditDenyList = map[string]bool{
	"password":             true,
	"totpsecret":           true,
	"apitokenhash":         true,
	"tokenhash":            true,
	"passwordencrypted":    true,
	"gotifytokenencrypted": true,
	"oidccode":             true,
	"oidcstate":            true,
	"authorization":        true,
	"cookie":               true,
	"jwtsecretkey":         true,
	"resendapikey":         true,
	"smtppassword":         true,
}

// auditSnapshotProvider is implemented by an audited entity whose before-
// snapshot must be marshaled from something other than json.Marshal(&model).
// Contact implements it to include its nested Card/CRM/Passthrough columns,
// which are json:"-" on the struct (so the REST API serves the nested model
// through ContactRecordResponse rather than leaking the storage shape) and
// were therefore never captured by the audit trail — see T82.
type auditSnapshotProvider interface {
	auditSnapshot() any
}

// auditLogger is the package-level recorder that persists audit events
// fire-and-forget from GORM hooks: a separate goroutine on its own session
// (never the hook's transaction), so an audit failure can never roll back the
// real write. The DB is registered at startup (RegisterAuditDB); until then —
// e.g. AutoMigrate-based unit tests — hooks skip silently, which is what keeps
// audit wiring out of every existing test.
//
// chainMu serializes the read-prev-hash + insert sequence so concurrent audit
// writes (each hook spawns its own goroutine) append to the hash chain in a
// deterministic id order instead of forking it. The standalone chain
// maintenance operations (RecomputeAuditChain) take the same lock, so they can
// never interleave with a live append.
type auditLogger struct {
	mu      sync.RWMutex
	db      *gorm.DB
	wg      sync.WaitGroup
	chainMu sync.Mutex
}

var auditRecorder = &auditLogger{}

// RegisterAuditDB points the audit recorder at a standalone DB session used
// for fire-and-forget audit writes. Called from database.InitDB after the
// connection is opened.
func RegisterAuditDB(db *gorm.DB) {
	auditRecorder.mu.Lock()
	auditRecorder.db = db
	auditRecorder.mu.Unlock()
}

// AuditFlush blocks until every in-flight audit write has completed. Used by
// tests (to read audit rows deterministically) and safe to call at shutdown.
//
// It holds a.mu across the Wait so a concurrent record() can never Add to the
// WaitGroup while Wait is running: the WaitGroup contract forbids an Add that
// starts at a zero counter concurrently with Wait (a data race that can lose a
// wakeup at shutdown). Serializing Add and Wait on a.mu makes every Add that
// began before the flush happen-before the Wait, which is what makes the drain
// total rather than best-effort.
func AuditFlush() {
	auditRecorder.mu.Lock()
	defer auditRecorder.mu.Unlock()
	auditRecorder.wg.Wait()
}

// record queues an audit event for async persistence. snapshotJSON is the
// redacted before-state ("" for creates). Never returns an error and never
// blocks the hook beyond launching the goroutine — see the package doc.
//
// The chain append (read previous row's hash, compute this row's, insert) is
// serialized on chainMu so concurrent events chain in id order.
func (a *auditLogger) record(entityType, entityID, operation string, userID uint, snapshotJSON string) {
	// The db-nil check and wg.Add(1) run atomically under a.mu so an Add can
	// never start while AuditFlush's Wait is in progress (see AuditFlush for
	// the WaitGroup-contract reasoning). The goroutine it spawns only touches
	// chainMu and the DB session, never a.mu, so holding the lock across the
	// drain cannot deadlock.
	a.mu.Lock()
	db := a.db
	if db == nil {
		// Not registered (e.g. AutoMigrate test DBs). Skip silently so hooks
		// never disturb writes in environments without an audit session.
		a.mu.Unlock()
		return
	}
	a.wg.Add(1)
	a.mu.Unlock()

	go func() {
		defer a.wg.Done()

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
			logger.Warn().Err(err).
				Str("entity_type", entityType).Str("entity_id", entityID).Str("operation", operation).
				Msg("audit: failed to persist audit event (real write is unaffected)")
		}
	}()
}

// RecordAuditEvent appends an auth/admin lifecycle event to the audit log
// (issue #381): login success/failure, registration, password change/reset,
// TOTP enable/disable, recovery-code regeneration, API-token create/revoke,
// and admin user operations. These are pure append-only records — no
// before-snapshot (undo only supports entity updates) and nothing secret, so
// the deny-list has nothing to strip. entityID is the affected subject: a
// username/email for auth events, a numeric user or token id for the rest.
func RecordAuditEvent(entityType, entityID, operation string, userID uint) {
	auditRecorder.record(entityType, entityID, operation, userID, "")
}

// auditState is carried across a single save's hook chain (BeforeSave →
// AfterSave) via the shared *gorm.Statement.Context. GORM's InstanceSet is
// unreliable here: getInstance() clones the statement, so the stored value
// never survives to AfterSave in this GORM version. The statement context is
// the same object across the chain, so a pointer stored there is visible to
// AfterSave.
type auditState struct {
	isNew  bool
	before string
}

const auditStateKey = "mycorrhizal:audit:state"

// auditBeforeSave is the shared BeforeSave hook helper: it marks whether this
// save is a create or an update and, for updates, captures the pre-update
// snapshot. generic is the entity type so the old row can be re-queried for
// the snapshot. A nil tx (some unit tests invoke hooks directly) is skipped.
func auditBeforeSave[T any](tx *gorm.DB, entityType string, entityID any, isNew bool) {
	if tx == nil || tx.Statement == nil {
		return
	}
	state := &auditState{isNew: isNew}
	if !isNew {
		var old T
		if err := tx.Session(&gorm.Session{NewDB: true}).Where("id = ?", entityID).First(&old).Error; err == nil {
			if raw, err := redactedJSONForAudit(&old); err == nil {
				state.before = raw
			}
		}
	}
	tx.Statement.Context = context.WithValue(tx.Statement.Context, auditStateKey, state)
}

// auditAfterSave fires the create/update audit event. Called from each
// audited model's AfterSave hook.
func auditAfterSave(tx *gorm.DB, entityType, entityID string, userID uint) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return
	}
	state, _ := tx.Statement.Context.Value(auditStateKey).(*auditState)
	op := AuditOpUpdate
	before := ""
	if state != nil {
		if state.isNew {
			op = AuditOpCreate
		} else {
			before = state.before
		}
	}
	auditRecorder.record(entityType, entityID, op, userID, before)
}

// auditAfterDelete fires the delete audit event with a redacted snapshot of
// the row being deleted (the model still holds its values in AfterDelete).
func auditAfterDelete(tx *gorm.DB, entityType, entityID string, userID uint, model any) {
	if tx == nil {
		return
	}
	raw, err := redactedJSONForAudit(model)
	if err != nil {
		return
	}
	auditRecorder.record(entityType, entityID, AuditOpDelete, userID, raw)
}

// redactedJSONForAudit marshals a model's before-snapshot, honoring the
// auditSnapshotProvider interface so an entity whose JSON tags omit data
// (Contact's json:"-" Card/CRM/Passthrough, T82) can supply a purpose-built
// snapshot shape. Falls back to plain json.Marshal for every other entity.
func redactedJSONForAudit(v interface{}) (string, error) {
	if p, ok := v.(auditSnapshotProvider); ok {
		return redactedJSON(p.auditSnapshot())
	}
	return redactedJSON(v)
}

// redactJSON walks a parsed JSON document and returns it with every key whose
// name (case-insensitively) is in auditDenyList removed, at any depth. Non-
// object values pass through untouched.
func redactJSON(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	redactValue(v)
	return json.Marshal(v)
}

func redactValue(v interface{}) {
	switch node := v.(type) {
	case map[string]interface{}:
		for key := range node {
			if auditDenyList[strings.ReplaceAll(strings.ToLower(key), "_", "")] {
				delete(node, key)
				continue
			}
			redactValue(node[key])
		}
	case []interface{}:
		for _, item := range node {
			redactValue(item)
		}
	}
}

// redactedJSON marshals v and applies the deny-list redaction. Errors (an
// unserializable model) yield an empty string — the audit event is still
// recorded, just without a snapshot, rather than failing the write.
func redactedJSON(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	redacted, err := redactJSON(raw)
	if err != nil {
		return "", err
	}
	return string(redacted), nil
}

// compile-time guard: AuditEvent has no Update/Delete receiver methods by
// construction (none are defined); the DB triggers are the safety net.
