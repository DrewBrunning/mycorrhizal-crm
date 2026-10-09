package services

import (
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
)

// searchTermFuzzSeeds cover the shapes NormalizeSearchTerm must survive: an
// ordinary term, FTS5 operator words and syntax characters the production
// ftsQuery path has to quote, a phone-shaped term (the separate phoneFTSMatch
// arm), the NUL byte that caused the historical "unterminated string" 500
// (issue #1460), an already-NFC vs decomposed Unicode pair, and an oversized
// term.
var searchTermFuzzSeeds = []string{
	"alice",
	"Ada Lovelace",
	`"quoted phrase"`,
	"a*b",
	"AND OR NOT NEAR",
	"(paren) -dash +plus",
	"800-555-1234",
	"+1 (800) 555-1234",
	"nul\x00byte",
	"caf\u00e9",  // NFC
	"cafe\u0301", // NFD, the same word
	strings.Repeat("x", 300),
	"",
}

// FuzzNormalizeSearchTerm fuzzes the search-term boundary (issue #1626).
// NormalizeSearchTerm folds a client term to NFC and strips NUL bytes before
// it reaches an FTS5 MATCH; the property is that its output is safe to hand
// to the production expression builder and then to a real FTS5 table:
//
//   - it is idempotent;
//   - no NUL survives (the byte that terminated the MATCH C string and
//     produced an "unterminated string" error — issue #1460);
//   - ContactFTSMatch(term) never produces a MATCH expression SQLite rejects.
//
// The DB is built once in the outer Fuzz function (once per fuzz worker
// process), not per input — a migrated-schema build per iteration would
// dominate the run.
func FuzzNormalizeSearchTerm(f *testing.F) {
	for _, seed := range searchTermFuzzSeeds {
		f.Add(seed)
	}

	db := dbtest.New(f)

	f.Fuzz(func(t *testing.T, data string) {
		term := NormalizeSearchTerm(data)
		if again := NormalizeSearchTerm(term); again != term {
			t.Fatalf("NormalizeSearchTerm not idempotent: %q -> %q -> %q", data, term, again)
		}
		if strings.IndexByte(term, 0) >= 0 {
			t.Fatalf("NormalizeSearchTerm left a NUL byte in %q", term)
		}

		// The production path (Search / applyContactSearch) builds the MATCH
		// expression through ContactFTSMatch, which quotes every token. Run
		// the result against a real FTS5 table and require no SQL error.
		expr, ok := ContactFTSMatch(term)
		if !ok {
			return
		}
		var n int64
		if err := db.Raw(
			"SELECT count(*) FROM contacts_fts WHERE contacts_fts MATCH ? AND contacts_fts.user_id = ?",
			expr, 0,
		).Scan(&n).Error; err != nil {
			t.Fatalf("MATCH %q (from term %q, input %q) errored: %v", expr, term, data, err)
		}
	})
}
