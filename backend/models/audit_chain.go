package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Tamper-evident audit chain (issue #381, ASVS V7.3).
//
// Every AuditEvent carries Hash = SHA-256(prev_hash || canonical content) and
// PrevHash = the preceding row's Hash ("" for the head of the chain). A
// verifier recomputes the whole chain in id order: any insertion, deletion,
// reorder, or edit breaks the linkage at exactly the first affected row, which
// is what VerifyAuditChain reports.
//
// The chain detects tampering; it does not prevent it (an attacker with full
// DB write access can always recompute the chain, same as they can drop the
// immutability trigger). The two sanctioned writers — the recorder's insert
// path and RecomputeAuditChain (startup backfill + retention-purge re-link) —
// are the only code that ever produces a hash, and they serialize on the
// recorder's chainMu so a live append can never interleave with a recompute.

// auditChainGenesis is the PrevHash of the first row. Empty string is chosen
// over a fixed constant so the chain is deterministic and trivially inspectable.
const auditChainGenesis = ""

// AuditChainGap describes the first broken link in the audit chain.
type AuditChainGap struct {
	// EventID is the id of the row where the chain first broke.
	EventID uint   `json:"event_id"`
	Message string `json:"message"`
}

// auditChainContent is the canonical, hashable serialization of an event's
// stored content. Field order and tags are part of the hash's definition —
// changing them invalidates every existing chain, so treat this struct as
// frozen.
type auditChainContent struct {
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	Operation      string `json:"operation"`
	UserID         uint   `json:"user_id"`
	BeforeSnapshot string `json:"before_snapshot"`
	CreatedAt      string `json:"created_at"`
}

// chainContent builds the canonical content bytes. CreatedAt is normalized to
// UTC microseconds so the hash is identical whether it is computed at insert
// (from an in-memory struct) or at verification (from a row re-read from
// SQLite, whose DATETIME precision is driver-dependent).
func (e *AuditEvent) chainContent() auditChainContent {
	return auditChainContent{
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		Operation:      e.Operation,
		UserID:         e.UserID,
		BeforeSnapshot: e.BeforeSnapshot,
		CreatedAt:      e.CreatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
	}
}

// AuditChainHash computes the SHA-256 chain hash for an event given its
// predecessor's hash. Deterministic: the same event state always yields the
// same hash, at write time and at verify time.
func AuditChainHash(prevHash string, e *AuditEvent) string {
	content, err := json.Marshal(e.chainContent())
	if err != nil {
		// The content struct only holds strings and a uint — unmarshallable is
		// not reachable; a panic here would take the recorder goroutine down.
		return ""
	}
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte{0}) // delimiter; prevHash is "" or exactly 64 hex chars
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// RecomputeAuditChain rewrites hash/prev_hash for every audit row, in id
// order, so the chain is complete and consistent. It is used for:
//
//   - the one-time backfill after migration 000034 (rows carry hash=” until
//     this runs — the server does it at startup, cmd/audit-backfill does it for
//     migrate-up-only workflows);
//   - re-linking after the retention purge, whose sanctioned DELETE of old rows
//     breaks the chain at the head.
//
// Idempotent and write-free when the chain is already consistent (a fresh boot
// touches nothing). Because the immutability trigger blocks UPDATE, it drops
// and re-creates that trigger around its writes, inside a single transaction —
// exactly the pattern migration 000034 itself uses. It takes the recorder's
// chainMu so it can never interleave with a live append.
//
// NOTE: this is a maintenance operation, not a verifier. It recalculates
// hashes for rows that already carry one, so running it against a tampered
// chain would "repair" the tampering. It is deliberately never called by
// VerifyAuditChain.
func RecomputeAuditChain(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	auditRecorder.chainMu.Lock()
	defer auditRecorder.chainMu.Unlock()

	var events []AuditEvent
	if err := db.Order("id asc").Find(&events).Error; err != nil {
		return fmt.Errorf("audit chain: read events: %w", err)
	}
	if len(events) == 0 {
		return nil
	}

	type chainUpdate struct {
		id       uint
		prevHash string
		hash     string
	}
	var updates []chainUpdate
	prev := auditChainGenesis
	for _, e := range events {
		hash := AuditChainHash(prev, &e)
		if e.PrevHash != prev || e.Hash != hash {
			updates = append(updates, chainUpdate{id: e.ID, prevHash: prev, hash: hash})
		}
		prev = hash
	}
	if len(updates) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DROP TRIGGER IF EXISTS audit_events_no_update").Error; err != nil {
			return fmt.Errorf("audit chain: drop immutability trigger: %w", err)
		}
		for _, u := range updates {
			if err := tx.Model(&AuditEvent{}).Where("id = ?", u.id).
				Updates(map[string]any{"hash": u.hash, "prev_hash": u.prevHash}).Error; err != nil {
				return fmt.Errorf("audit chain: update event %d: %w", u.id, err)
			}
		}
		if err := tx.Exec("CREATE TRIGGER IF NOT EXISTS audit_events_no_update " +
			"BEFORE UPDATE ON audit_events BEGIN " +
			"SELECT RAISE(ABORT, 'audit_events is append-only: UPDATE is not allowed'); END").Error; err != nil {
			return fmt.Errorf("audit chain: recreate immutability trigger: %w", err)
		}
		return nil
	})
}

// VerifyAuditChain recomputes the hash chain and returns the FIRST gap, or nil
// when the chain is intact. It is read-only and never repairs anything. A gap
// message distinguishes:
//
//   - "backfill pending": a row still carries hash=” (migration 000034 applied
//     but RecomputeAuditChain has not run yet);
//   - "prev_hash mismatch": the row's stored PrevHash differs from the
//     predecessor's recomputed hash — a row was deleted, inserted, or had its
//     id/order changed;
//   - "hash mismatch": the row's stored Hash differs from a fresh
//     computation — its content was edited after recording.
func VerifyAuditChain(db *gorm.DB) ([]AuditChainGap, error) {
	if db == nil {
		return nil, nil
	}
	var events []AuditEvent
	if err := db.Order("id asc").Find(&events).Error; err != nil {
		return nil, fmt.Errorf("audit chain: read events: %w", err)
	}
	if len(events) == 0 {
		return nil, nil
	}

	prev := auditChainGenesis
	for _, e := range events {
		if e.Hash == "" {
			return []AuditChainGap{{EventID: e.ID, Message: fmt.Sprintf(
				"event %d has no hash (chain backfill pending: run the server once or `make audit-backfill`)", e.ID)}}, nil
		}
		if e.PrevHash != prev {
			return []AuditChainGap{{EventID: e.ID, Message: fmt.Sprintf(
				"event %d prev_hash mismatch: stored %q, expected %q (a row was deleted, inserted, or reordered)", e.ID, e.PrevHash, prev)}}, nil
		}
		expected := AuditChainHash(prev, &e)
		if e.Hash != expected {
			return []AuditChainGap{{EventID: e.ID, Message: fmt.Sprintf(
				"event %d hash mismatch: stored %q, expected %q (content was modified after recording)", e.ID, e.Hash, expected)}}, nil
		}
		prev = e.Hash
	}
	return nil, nil
}
