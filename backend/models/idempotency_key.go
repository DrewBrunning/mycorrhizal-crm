package models

import "time"

// IdempotencyKey is one client-supplied idempotency token and the stored
// outcome of the request that first used it (issue #459, CON-04 —
// docs/adrs/0010-idempotency-keys.md).
//
// A client that cannot tolerate an ambiguous failure (the write landed, the
// response was lost) attaches `Idempotency-Key: <opaque>` to a non-idempotent
// POST. The first request runs the handler and records its response here; every
// later request with the same (UserID, Key) replays ResponseStatus /
// ResponseBody verbatim without re-running the handler — one row, one webhook,
// one push, however many times the client retries.
//
// User-scoped operational bookkeeping, hard-delete (no DeletedAt), immutable
// except for the pending -> completed transition. Same lifecycle shape as
// JobRun / SystemEvent; unlike ImportRun it *does* carry a retention purge
// (PurgeExpiredIdempotencyKeys), because a keyed POST is common, not rare.
//
// Every field carries an explicit gorm:"column:..." tag — GORM's name
// derivation disagrees with the hand-written migration SQL for acronyms/IDs and
// AutoMigrate-based tests cannot see it (CLAUDE.md backend trap #1).
type IdempotencyKey struct {
	ID     uint `gorm:"column:id;primarykey" json:"id"`
	UserID uint `gorm:"column:user_id;not null;index" json:"user_id"`

	// Key is the opaque client-supplied token. Unique per user
	// (idx_idempotency_keys_user_key) — that index is what makes the
	// claim-the-key INSERT race-safe.
	Key string `gorm:"column:idempotency_key;not null" json:"idempotency_key"`

	Method string `gorm:"column:method;not null" json:"method"`
	Path   string `gorm:"column:path;not null" json:"path"`

	// RequestFingerprint is a hash of method + path + body. A key replayed
	// with a different request is rejected 422 rather than replaying the wrong
	// stored response.
	RequestFingerprint string `gorm:"column:request_fingerprint;not null" json:"-"`

	// State is "pending" between the INSERT that claims the key and the UPDATE
	// that records the response, then "completed". A concurrent retry that
	// finds a pending row is told 409, not allowed to re-run the handler.
	State string `gorm:"column:state;not null;default:pending" json:"state"`

	ResponseStatus int    `gorm:"column:response_status;not null;default:0" json:"response_status"`
	ResponseBody   string `gorm:"column:response_body;not null;default:''" json:"-"`

	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the table so a rename of the struct can't silently repoint it.
func (IdempotencyKey) TableName() string { return "idempotency_keys" }

// Idempotency-key state values.
const (
	IdempotencyStatePending   = "pending"
	IdempotencyStateCompleted = "completed"
)
