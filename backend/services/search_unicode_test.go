package services

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// I18N-02 (issue #485): the search side of Unicode normalization. These tests
// pin two things that are easy to conflate:
//
//  1. Stored data is NFC (guaranteed by the write boundary, contactmodel/
//     NormalizeRecord, + the startup backfill for pre-existing rows).
//  2. Search is diacritic- and case-insensitive for Latin-script text, and
//     folds NFD/NFC either direction, because the FTS5 unicode61 tokenizer
//     removes diacritics and folds ASCII case. Queries are NFC-folded
//     (NormalizeSearchTerm) so every comparison arm — FTS and the byte-based
//     LIKE arms — sees storage-form bytes.
//
// The "documented, not discovered" behaviors live in
// docs/development/unicode-search.md; each row here is a pin for one line of
// that table. Changing a pin means changing the doc, deliberately.

func newUnicodeUser(t *testing.T, db *gorm.DB, suffix string) models.User {
	t.Helper()
	u := models.User{Username: "uni-" + suffix, Password: "password123!A", Email: "uni-" + suffix + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func searchFirstnames(t *testing.T, db *gorm.DB, userID uint, term string) []string {
	t.Helper()
	res, err := Search(db, userID, term, 0, nil)
	require.NoError(t, err)
	out := make([]string, 0, len(res.Contacts))
	for _, c := range res.Contacts {
		out = append(out, c.Firstname+" "+c.Lastname)
	}
	return out
}

// TestSearch_CrossEncodingContact pins the issue's headline: "a contact
// stored in NFD is found by an NFC query and vice versa." Production stores
// NFC (see the write-boundary tests), so to exercise a pre-backfill NFD row
// this seeds the flat columns + indexed card bytes directly in NFD — exactly
// the state migration 000052's Go backfill exists to clean up — and asserts
// the FTS path resolves it from an NFC / ASCII / NFD query in both
// directions.
func TestSearch_CrossEncodingContact(t *testing.T) {
	db := newSearchDB(t)

	t.Run("stored NFD found by NFC and ASCII queries", func(t *testing.T) {
		user := newUnicodeUser(t, db, "nfdbynfc")
		// Decomposed "José" (e + U+0301) written straight into the columns,
		// bypassing ApplyRecordToContact (the state that can only exist
		// pre-backfill).
		c := models.Contact{UserID: user.ID, Firstname: "Jos\u0065\u0301", Lastname: "P\u0065\u0301rez"}
		require.NoError(t, db.Create(&c).Error)

		for _, term := range []string{"José", "Jos\u0065\u0301", "jose", "Pérez", "P\u0065\u0301rez", "perez"} {
			hits := searchFirstnames(t, db, user.ID, term)
			require.Len(t, hits, 1, "term %q must find the NFD-stored contact", term)
		}
	})

	t.Run("stored NFC found by NFD query", func(t *testing.T) {
		user := newUnicodeUser(t, db, "nfcbyndf")
		c := models.Contact{UserID: user.ID, Firstname: "José", Lastname: "Pérez"}
		require.NoError(t, db.Create(&c).Error)

		for _, term := range []string{"José", "Jos\u0065\u0301", "jose", "Pérez", "P\u0065\u0301rez"} {
			hits := searchFirstnames(t, db, user.ID, term)
			require.Len(t, hits, 1, "term %q must find the NFC-stored contact", term)
		}
	})
}

// TestSearch_UnicodeCaseAndDiacriticPins is the "documented, not discovered"
// table from docs/development/unicode-search.md, pinned against the real FTS5
// tokenizer of this repo's pure-Go driver. Each case seeds one contact and
// asserts which spellings of its name resolve and which deliberately do not.
func TestSearch_UnicodeCaseAndDiacriticPins(t *testing.T) {
	cases := []struct {
		label     string
		firstname string
		queries   map[string]bool // term -> must match?
	}{
		{
			label:     "Latin accent + case fold (García)",
			firstname: "García",
			queries:   map[string]bool{"garcia": true, "GARCÍA": true, "García": true, "GARCIa": true, "Garci\u0301a": true},
		},
		{
			label:     "Latin umlaut fold (Müller)",
			firstname: "Müller",
			queries:   map[string]bool{"muller": true, "MÜLLER": true, "Müller": true},
		},
		{
			label:     "Turkish dotted I folds to i (İstanbul)",
			firstname: "İstanbul",
			queries:   map[string]bool{"istanbul": true, "ISTANBUL": true, "\u0130stanbul": true},
		},
		{
			label:     "Turkish dotless ı stays distinct",
			firstname: "Istanbul",
			queries:   map[string]bool{"istanbul": true, "\u0131stanbul": false},
		},
		{
			label:     "German ß does not fold to ss",
			firstname: "Straße",
			queries:   map[string]bool{"straße": true, "strasse": false, "STRASSE": false},
		},
		{
			label:     "Greek all-caps not folded to lowercase",
			firstname: "Κωνσταντίνος",
			queries:   map[string]bool{"κωνσταντίνος": true, "ΚΩΝΣΤΑΝΤΙΝΟΣ": false},
		},
		{
			label:     "Cyrillic folds (observed tokenizer behavior)",
			firstname: "Екатерина",
			queries:   map[string]bool{"екатерина": true, "ЕКАТЕРИНА": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			db := newSearchDB(t)
			user := newUnicodeUser(t, db, "pin-"+strings.ReplaceAll(tc.label, " ", "-"))
			require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: tc.firstname, Lastname: "Q"}).Error)
			for term, want := range tc.queries {
				hits := searchFirstnames(t, db, user.ID, term)
				if want {
					assert.Len(t, hits, 1, "term %q should match %q", term, tc.firstname)
				} else {
					assert.Empty(t, hits, "term %q must NOT match %q", term, tc.firstname)
				}
			}
		})
	}
}

// TestSearch_NDFNoteContentStillFindable pins that the notes/activities arms
// of /search keep working for decomposed stored text without any stored-data
// normalization (only contact text is normalized on write). The FTS5
// tokenizer folds NFD/NFC at index and query time alike, so an NFC query
// finds an NFD note body.
func TestSearch_NDFNoteContentStillFindable(t *testing.T) {
	db := newSearchDB(t)
	user := newUnicodeUser(t, db, "ndfnote")

	c := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&models.Note{
		UserID: user.ID, ContactID: &c.ID, Content: "met for caf\u0065\u0301 and \u00e9tude",
	}).Error)

	res, err := Search(db, user.ID, "café", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Notes, 1, "NFC query finds NFD note content via the FTS tokenizer")
	assert.Equal(t, "Ada Lovelace", res.Notes[0].ContactName)
}

// TestNormalizeSearchTerm pins the query-side fold: NFD in becomes NFC out,
// NFC passes through untouched, and the empty term is unchanged (callers gate
// on it before running a MATCH).
func TestNormalizeSearchTerm(t *testing.T) {
	assert.Equal(t, "José", NormalizeSearchTerm("Jos\u0065\u0301"), "NFD query folds to NFC")
	assert.Equal(t, "José Pérez", NormalizeSearchTerm("Jos\u0065\u0301 P\u0065\u0301rez"))
	assert.Equal(t, "José", NormalizeSearchTerm("José"), "already-NFC query is an identity")
	assert.Equal(t, "", NormalizeSearchTerm(""))
	assert.Equal(t, "abc", NormalizeSearchTerm("a\x00b\x00c"), "NUL bytes are stripped (issue #1460)")
	if !norm.NFC.IsNormalString(NormalizeSearchTerm("Garc\u0069\u0301a")) {
		t.Fatal("NormalizeSearchTerm output must be NFC")
	}
}

// TestSearch_NulByteTermDoesNotError is the regression for issue #1460: a
// Schemathesis stateful sequence sent a `/search?q=` term containing a NUL
// byte and got a 500 DATABASE_ERROR. FTS5 hands its query parser a C string,
// so the NUL terminated the MATCH expression mid-phrase and SQLite reported
// "unterminated string" — a server error on input the API accepts. The term
// is stripped of NUL before any arm runs, so the request succeeds and the
// non-NUL tokens still search normally. Control bytes other than NUL are
// harmless to FTS5 and must not error either.
func TestSearch_NulByteTermDoesNotError(t *testing.T) {
	db := newSearchDB(t)
	user := newUnicodeUser(t, db, "nulbyte")
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}).Error)

	// The exact shape from the nightly report: NUL first, plus controls and
	// an invalid UTF-8 sequence, wrapped around a real token.
	nulTerm := "\x00W(\x02æQ!\u00c3)5ü\xf3\x87\x97\xaeD"
	_, err := Search(db, user.ID, nulTerm, 26, nil)
	require.NoError(t, err, "a NUL byte in the term must not 500 the search endpoint")

	// A term whose only non-NUL text still matches must keep matching after
	// the NUL is stripped.
	hits := searchFirstnames(t, db, user.ID, "Ada\x00")
	require.Len(t, hits, 1, "stripping the NUL leaves the searchable token intact")

	// Every other C0 control byte tokenizes without error, so the fix is
	// narrowly scoped to NUL rather than "strip all controls".
	for b := 0; b <= 0x1f; b++ {
		if b == 0 {
			continue
		}
		_, err := Search(db, user.ID, "a"+string(rune(b))+"b", 0, nil)
		require.NoError(t, err, "control byte 0x%02x must not error", b)
	}
}

// TestNormalizeRecordIsIdentityOverManifestShape is a cheap canary that the
// neutral normalizer never mangles the exotic-but-NFC shapes the canonical
// fixture carries (Devanagari with intrinsic combining marks, ZWJ emoji): run
// it over a full Record built via the public model and require DeepEqual.
func TestNormalizeRecordIsIdentityOverManifestShape(t *testing.T) {
	// The manifest's priya record is the densest combining-mark case in the
	// corpus; ZWJ families are the classic "normalization must not split
	// grapheme clusters" trap. Both are already NFC.
	for _, given := range []string{"प्रिया", "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"} {
		rec := &contactmodel.Record{Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
		}}
		got := contactmodel.NormalizeRecord(rec)
		if got.Card.Name.Components[0].Value != given {
			t.Errorf("NormalizeRecord changed %q -> %q", given, got.Card.Name.Components[0].Value)
		}
	}
}
