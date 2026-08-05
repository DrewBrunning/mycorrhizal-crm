package services

import (
	"fmt"
	"mycorrhizal/models"
	"strings"

	"gorm.io/gorm"
)

// Full-text search over contacts, notes, and interactions (T11 / WP-86,
// docs/fork-plan/tickets/24-T11-search-fts5.md; addresses added by T38,
// docs/fork-plan/tickets/47-T38-search-address-fields.md). Backed by FTS5
// virtual tables kept in sync by triggers (migrations 000007 + 000010); the
// index is derived data and can be rebuilt at any time via
// RebuildSearchIndex.
//
// Contact addresses are searchable through the denormalized
// contacts.addresses_flat column (maintained by Contact.BeforeSave and
// backfilled by migration 000010), indexed into contacts_fts like the other
// flat fields — the ticket's lowest-friction option, consistent with how the
// rest of the index works.
//
// Scoping: each FTS row carries user_id (UNINDEXED), and every query filters
// on it — the highest-risk correctness rule of the ticket, pinned by
// TestSearch_CrossUserReturnsNothing and TestSearch_CrossUserAddressDoesNotLeak.
// The base-table JOIN also re-scopes and re-filters soft-deleted rows
// (defense-in-depth on top of the triggers).

// SearchResult is the grouped response: matched contacts, notes, and
// interactions (activities). Query and any resolved relation synonym are
// echoed so a client can show "matched relationship: sibling".
type SearchResult struct {
	Query            string              `json:"query"`
	ResolvedRelation string              `json:"resolved_relation,omitempty"`
	Contacts         []SearchContactHit  `json:"contacts"`
	Notes            []SearchNoteHit     `json:"notes"`
	Activities       []SearchActivityHit `json:"activities"`
}

// SearchContactHit is a matched contact (the slim ContactSummary shape) plus
// a snippet of the matched field.
type SearchContactHit struct {
	models.ContactSummary
	Snippet string `json:"snippet,omitempty"`
}

// SearchNoteHit is a matched note with its owning contact's name when one is
// attached, plus a snippet of the matched content.
type SearchNoteHit struct {
	models.Note
	ContactName string `json:"contact_name,omitempty"`
	Snippet     string `json:"snippet"`
}

// SearchActivityHit is a matched interaction (activity) plus a snippet.
type SearchActivityHit struct {
	models.Activity
	Snippet string `json:"snippet"`
}

const (
	searchDefaultLimit = 10
	searchMaxLimit     = 50
)

// ResolveSearchSynonym reports whether the whole search term resolves to a
// relation token through the type registry ("mom"/"mother" → parent_of,
// "brother" → sibling_of) — T11's synonym consumer, also used by the graph
// traversal relation filter (services/graph_traversal.go).
func ResolveSearchSynonym(term string) (string, bool) {
	return models.MatchLegacyRelationType(term)
}

// ftsQuery turns a free-text term into a safe FTS5 MATCH expression: each
// whitespace-separated token is quoted (escaping embedded quotes) and given a
// prefix-match suffix, joined by implicit AND. This prevents FTS5 syntax
// errors from user input (quotes, operators) while still prefix-matching
// ("alice" finds "alicia").
func ftsQuery(term string) string {
	tokens := strings.Fields(term)
	if len(tokens) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"*`)
	}
	return strings.Join(quoted, " ")
}

// Search runs a full-text query across the user's contacts, notes, and
// interactions. Empty/whitespace queries return an empty result. A query
// shorter than two characters also returns empty (avoiding a noisy index
// scan for one-character terms, matching the frontend's debounce gate).
//
// When householdID is non-nil, contact results are restricted to members of
// that household ("everyone in the Smith household", T11's household-scoped
// half) — the caller is responsible for verifying the household belongs to
// the user.
func Search(db *gorm.DB, userID uint, term string, limit int, householdID *string) (*SearchResult, error) {
	result := &SearchResult{
		Query:      strings.TrimSpace(term),
		Contacts:   []SearchContactHit{},
		Notes:      []SearchNoteHit{},
		Activities: []SearchActivityHit{},
	}
	term = strings.TrimSpace(term)
	if term == "" || len([]rune(term)) < 2 {
		return result, nil
	}

	if limit < 1 {
		limit = searchDefaultLimit
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	// Synonym resolution: if the whole term is a relation synonym, echo the
	// canonical token so clients can offer relationship-aware results (the
	// graph traversal relation filter is where it actually filters).
	if token, ok := ResolveSearchSynonym(term); ok {
		result.ResolvedRelation = token
	}

	match := ftsQuery(term)

	// Optional household scope: restrict contact hits to members of one
	// household (T11's "everyone in the Smith household").
	householdClause := ""
	householdArgs := []interface{}{}
	if householdID != nil {
		householdClause = " AND c.vcard_uid IN (SELECT member_vcard_uid FROM household_members WHERE household_id = ? AND user_id = ?)"
		householdArgs = append(householdArgs, *householdID, userID)
	}

	// Contacts: FTS hit → JOIN the live row (re-scoping + re-filtering
	// soft-deletes defensively), build the slim summary shape.
	var contactHits []struct {
		models.Contact
		Snippet string
	}
	contactArgs := append([]interface{}{match, userID, userID}, householdArgs...)
	contactArgs = append(contactArgs, limit)
	err := db.Raw(`
		SELECT c.*, snippet(contacts_fts, 0, '…', '…', '…', 20) AS snippet
		FROM contacts_fts
		JOIN contacts c ON c.id = contacts_fts.rowid
		WHERE contacts_fts MATCH ?
		  AND contacts_fts.user_id = ?
		  AND c.user_id = ?
		  AND c.deleted_at IS NULL`+householdClause+`
		ORDER BY contacts_fts.rank
		LIMIT ?`,
		contactArgs...,
	).Scan(&contactHits).Error
	if err != nil {
		return nil, fmt.Errorf("search contacts: %w", err)
	}
	for i := range contactHits {
		hit := SearchContactHit{
			ContactSummary: models.NewContactSummary(&contactHits[i].Contact),
			Snippet:        contactHits[i].Snippet,
		}
		result.Contacts = append(result.Contacts, hit)
	}

	// Notes: FTS hit → JOIN the live note + its contact name (direct FK).
	var noteHits []struct {
		models.Note
		ContactName string
		Snippet     string
	}
	err = db.Raw(`
		SELECT n.*, COALESCE(c.firstname || ' ' || c.lastname, '') AS contact_name,
		       snippet(notes_fts, 0, '…', '…', '…', 20) AS snippet
		FROM notes_fts
		JOIN notes n ON n.id = notes_fts.rowid
		LEFT JOIN contacts c ON c.id = n.contact_id
		WHERE notes_fts MATCH ?
		  AND notes_fts.user_id = ?
		  AND n.user_id = ?
		  AND n.deleted_at IS NULL
		ORDER BY notes_fts.rank
		LIMIT ?`,
		match, userID, userID, limit,
	).Scan(&noteHits).Error
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	for i := range noteHits {
		hit := SearchNoteHit{
			Note:        noteHits[i].Note,
			ContactName: strings.TrimSpace(noteHits[i].ContactName),
			Snippet:     noteHits[i].Snippet,
		}
		result.Notes = append(result.Notes, hit)
	}

	// Activities (interactions): FTS hit → JOIN the live activity.
	var activityHits []struct {
		models.Activity
		Snippet string
	}
	err = db.Raw(`
		SELECT a.*, snippet(activities_fts, 0, '…', '…', '…', 20) AS snippet
		FROM activities_fts
		JOIN activities a ON a.id = activities_fts.rowid
		WHERE activities_fts MATCH ?
		  AND activities_fts.user_id = ?
		  AND a.user_id = ?
		  AND a.deleted_at IS NULL
		ORDER BY activities_fts.rank
		LIMIT ?`,
		match, userID, userID, limit,
	).Scan(&activityHits).Error
	if err != nil {
		return nil, fmt.Errorf("search activities: %w", err)
	}
	for i := range activityHits {
		hit := SearchActivityHit{
			Activity: activityHits[i].Activity,
			Snippet:  activityHits[i].Snippet,
		}
		result.Activities = append(result.Activities, hit)
	}

	return result, nil
}

// RebuildSearchIndex truncates the three FTS virtual tables and re-inserts
// every live row from the base tables, including the address text in
// contacts.addresses_flat. Idempotent and re-runnable — call it after any
// bulk data change that bypassed the triggers (raw SQL migrations,
// backfills), and to make pre-existing contacts' addresses searchable after
// migration 000010. Runs in one transaction so a failure cannot leave a
// half-built index.
func RebuildSearchIndex(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range []string{
			"DELETE FROM contacts_fts",
			"DELETE FROM notes_fts",
			"DELETE FROM activities_fts",
			`INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat)
			 SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat
			 FROM contacts WHERE deleted_at IS NULL`,
			`INSERT INTO notes_fts(rowid, user_id, content)
			 SELECT id, user_id, content
			 FROM notes WHERE deleted_at IS NULL`,
			`INSERT INTO activities_fts(rowid, user_id, title, description, location)
			 SELECT id, user_id, title, description, location
			 FROM activities WHERE deleted_at IS NULL`,
		} {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("rebuild search index: %w", err)
			}
		}
		return nil
	})
}
