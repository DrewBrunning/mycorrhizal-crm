package models

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// Audit operation tokens stored on AuditEvent.Operation. The first three are
// entity CRUD; the rest are the auth/admin lifecycle vocabulary added by issue
// #381 (ASVS V7.3), plus password_reset_requested added by issue #411. The
// set is pinned by migration 000034/000035's CHECK constraint — a token
// added here without a migration is a silent INSERT failure.
const (
	AuditOpCreate         = "create"
	AuditOpUpdate         = "update"
	AuditOpDelete         = "delete"
	AuditOpLogin          = "login"
	AuditOpLoginFailed    = "login_failed"
	AuditOpRegister       = "register"
	AuditOpPasswordChange = "password_change"
	AuditOpPasswordReset  = "password_reset"
	// AuditOpPasswordResetRequested fires when a reset is requested for a
	// known account (issue #411) -- distinct from AuditOpPasswordReset, which
	// fires only once the token is actually confirmed. Never recorded for an
	// unknown email: RequestPasswordReset returns before this point, so the
	// audit trail itself can't be used to enumerate accounts.
	AuditOpPasswordResetRequested = "password_reset_requested"
	AuditOpTOTPEnable             = "totp_enable"
	AuditOpTOTPDisable            = "totp_disable"
	AuditOpRecoveryRegen          = "recovery_regenerate"
	// Issue #593: passkey enrollment / removal (entity = the user).
	AuditOpWebAuthnRegister = "webauthn_register" //nolint:gosec // G101 false positive: an audit operation token, not a credential
	AuditOpWebAuthnRevoke   = "webauthn_revoke"   //nolint:gosec // G101 false positive: an audit operation token, not a credential
	AuditOpRevoke           = "revoke"
	AuditOpRoleChange       = "role_change"
	// AuditOpTwoFactorAdminReset fires when an admin resets another user's
	// second factor (issue #592) -- distinct from AuditOpTOTPDisable, which
	// fires only on the self-service path where the caller proves they still
	// have a live code. This is the operator-side response to a user locked
	// out of their own account (TOTP device and recovery codes both lost).
	AuditOpTwoFactorAdminReset = "two_factor_admin_reset"
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
	// AuditEntityFeed is the private Atom feed credential (issue #382, ADR
	// 0030). Create/rotate/revoke record under it, mirroring the API-token
	// events.
	AuditEntityFeed = "feed"
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

// RecordAuditEvent appends an auth/admin lifecycle event to the audit log
// (issue #381): login success/failure, registration, password change/reset,
// TOTP enable/disable, recovery-code regeneration, API-token create/revoke,
// and admin user operations. These are pure append-only records — no
// before-snapshot (undo only supports entity updates) and nothing secret, so
// the deny-list has nothing to strip. entityID is the affected subject: a
// username/email for auth events, a numeric user or token id for the rest.
//
// db is the handle the caller already holds; the recorder is resolved from it
// (see AuditRecorderFor), never from a package variable (issue #1493).
func RecordAuditEvent(db *gorm.DB, entityType, entityID, operation string, userID uint) {
	recordAudit(db, entityType, entityID, operation, userID, "")
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

// auditContextKey is an unexported key type so no other package's
// context.WithValue can collide with the audit state (staticcheck SA1029).
type auditContextKey string

const auditStateKey auditContextKey = "mycorrhizal:audit:state"

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
		if err := tx.Session(&gorm.Session{NewDB: true}).Where("id = ?", entityID).First(&old).Error; err != nil {
			// A failed re-query (busy-timeout, transient I/O) must not pass
			// silently: it leaves state.before == "", which auditAfterSave
			// then persists as a genuine "no prior state" update event —
			// indistinguishable from a create — and any consumer relying on
			// that before-snapshot (e.g. reach-out detection) silently sees
			// no change at all. Logging surfaces that gap instead of letting
			// it masquerade as normal.
			logger.Warn().Err(err).
				Str("entity_type", entityType).Any("entity_id", entityID).
				Msg("audit: failed to load pre-update state for before-snapshot")
		} else if raw, err := redactedJSONForAudit(&old); err == nil {
			state.before = raw
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
	if skipZeroIdentityAudit(entityType, entityID, userID, AuditOpCreate) {
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
	recordAudit(tx, entityType, entityID, op, userID, before)
}

// skipZeroIdentityAudit reports whether an audit hook fired for a zero-value
// model (empty/"0" entity id, or user_id 0) and must not record anything
// (issue #1471). A bulk Where(...).Delete(&Model{}) or Model(&M{}).Where(...)
// .Update(...) fires the model's hooks once on a zero-value receiver rather
// than per row; the resulting event can never satisfy audit_events.user_id's
// FK and only produces a "failed to persist audit event" warning that buries
// real failures. Cascade children are deliberately not undoable, so no
// per-child event is lost. Centralised here so a new audited model cannot
// reintroduce the bug.
func skipZeroIdentityAudit(entityType, entityID string, userID uint, op string) bool {
	if entityID != "" && entityID != "0" && userID != 0 {
		return false
	}
	logger.Debug().
		Str("entity_type", entityType).Str("entity_id", entityID).Str("operation", op).
		Msg("audit: skipping event for zero-identity (bulk-hook) model")
	return true
}

// auditAfterDelete fires the delete audit event with a redacted snapshot of
// the row being deleted (the model still holds its values in AfterDelete).
func auditAfterDelete(tx *gorm.DB, entityType, entityID string, userID uint, model any) {
	if tx == nil {
		return
	}
	if skipZeroIdentityAudit(entityType, entityID, userID, AuditOpDelete) {
		return
	}
	raw, err := redactedJSONForAudit(model)
	if err != nil {
		return
	}
	recordAudit(tx, entityType, entityID, AuditOpDelete, userID, raw)
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
