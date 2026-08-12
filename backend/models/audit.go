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

// Audit operation tokens stored on AuditEvent.Operation.
const (
	AuditOpCreate = "create"
	AuditOpUpdate = "update"
	AuditOpDelete = "delete"
)

// AuditEntityType tokens stored on AuditEvent.EntityType.
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
)

// AuditEvent is one immutable create/update/delete record for an audited
// entity (T18, docs/fork-plan/tickets/34-T18-audit-trail.md), feeding undo,
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
	BeforeSnapshot string `gorm:"column:before_snapshot;type:text" json:"before_snapshot,omitempty"`
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
// were therefore never captured by the audit trail — see T82 (docs/fork-plan/
// tickets/126-T82-audit-snapshots-miss-nested-contact-data.md).
type auditSnapshotProvider interface {
	auditSnapshot() any
}

// auditLogger is the package-level recorder that persists audit events
// fire-and-forget from GORM hooks: a separate goroutine on its own session
// (never the hook's transaction), so an audit failure can never roll back the
// real write. The DB is registered at startup (RegisterAuditDB); until then —
// e.g. AutoMigrate-based unit tests — hooks skip silently, which is what keeps
// audit wiring out of every existing test.
type auditLogger struct {
	mu sync.RWMutex
	db *gorm.DB
	wg sync.WaitGroup
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
func AuditFlush() {
	auditRecorder.wg.Wait()
}

// record queues an audit event for async persistence. snapshotJSON is the
// redacted before-state ("" for creates). Never returns an error and never
// blocks the hook beyond launching the goroutine — see the package doc.
func (a *auditLogger) record(entityType, entityID, operation string, userID uint, snapshotJSON string) {
	a.mu.RLock()
	db := a.db
	a.mu.RUnlock()
	if db == nil {
		// Not registered (e.g. AutoMigrate test DBs). Skip silently so hooks
		// never disturb writes in environments without an audit session.
		return
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		event := AuditEvent{
			EntityType:     entityType,
			EntityID:       entityID,
			Operation:      operation,
			UserID:         userID,
			BeforeSnapshot: snapshotJSON,
		}
		if err := db.Create(&event).Error; err != nil {
			logger.Warn().Err(err).
				Str("entity_type", entityType).Str("entity_id", entityID).Str("operation", operation).
				Msg("audit: failed to persist audit event (real write is unaffected)")
		}
	}()
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
