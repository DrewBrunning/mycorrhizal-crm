package models

import "time"

// DismissedDuplicatePair is T93's permanent "not a duplicate" memory
// (T93): one
// row per dismissed duplicate-pair candidate, identified by the ORDERED
// (user_id, uid_low, uid_high) triple of Contact.VCardUIDs. Ordering means
// (A,B) and (B,A) can never both be stored.
//
// uint primary key, hard-delete per /CLAUDE.md trap #7 (T26): a join-shaped
// row whose identity IS its natural-key composite unique index. A dismissal
// is not user-authored content with an undo button — it is a permanent,
// deterministic fact, and deleting either contact sweeps its rows in
// DeleteContact's cascade checklist (backend/controllers/contact_controller.go
// deleteContactAssociations) and DeleteUser's sweep.
//
// GORM tag priorities mirror the migration's composite unique index column
// order (user_id, uid_low, uid_high) exactly, so an AutoMigrate-derived schema
// (test-only) and the real migration SQL agree.
type DismissedDuplicatePair struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID  uint   `gorm:"not null;index;uniqueIndex:idx_dismissed_duplicate_pairs_unique,priority:1" json:"-"`
	UIDLow  string `gorm:"not null;uniqueIndex:idx_dismissed_duplicate_pairs_unique,priority:2" json:"uid_low"`
	UIDHigh string `gorm:"not null;uniqueIndex:idx_dismissed_duplicate_pairs_unique,priority:3" json:"uid_high"`
}

// DuplicatePairReason is one tier of the duplicate scan that two contacts
// matched on. A pair can match on several (the response carries the full set,
// not the first hit — a pair matching on email AND phone is a much stronger
// candidate than one matching on name alone).
type DuplicatePairReason string

const (
	// DuplicateReasonEmail — both contacts carry the same email (flat `email`
	// column, case-insensitive).
	DuplicateReasonEmail DuplicatePairReason = "email"
	// DuplicateReasonName — both contacts share the same exact
	// firstname+lastname (case-insensitive). The false-positive tier: pairs a
	// father and son, pairs two unrelated people with a common name. That is
	// why reasons/confidence and persistent dismissal exist.
	DuplicateReasonName DuplicatePairReason = "name"
	// DuplicateReasonPhone — both contacts carry a phone that reduces to the
	// same PhoneKey (last-10 digits, T68), read from the phones_normalized
	// column so every number is scanned, not just the flat primary — the thing
	// DetectDuplicate (import_service.go) cannot do.
	DuplicateReasonPhone DuplicatePairReason = "phone"
)

// DuplicatePair is one candidate pair returned by GET /contacts/duplicates.
// A and B are plain ContactSummaries — the same DTO the list endpoint
// returns, so the web client renders rows with existing components. Reasons
// is the full set of tiers that matched (never the first one only).
// Confidence is a 0-1 heuristic for how likely the pair really is a
// duplicate — strictly a function of which tiers matched; the UI sorts by it.
type DuplicatePair struct {
	A          ContactSummary `json:"a"`
	B          ContactSummary `json:"b"`
	Reasons    []string       `json:"reasons"`
	Confidence float64        `json:"confidence"`
}

// DuplicatePairsResponse is the GET /contacts/duplicates response. Pairs has
// no omitempty so an empty scan serializes as `[]`, never null (CLAUDE.md
// frontend trap #8 — a required TS field would otherwise crash the client).
type DuplicatePairsResponse struct {
	Pairs []DuplicatePair `json:"pairs"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// DuplicateDismissalInput is the request body for POST /contacts/duplicates/
// dismiss: the two contacts of a pair, identified by Contact.VCardUID (the
// stable identity the dismissal table keys on). The controller orders them
// into uid_low/uid_high so (A,B) and (B,A) are the same dismissal.
type DuplicateDismissalInput struct {
	UIDA string `json:"uid_a" validate:"required,uuid4"`
	UIDB string `json:"uid_b" validate:"required,uuid4"`
}
