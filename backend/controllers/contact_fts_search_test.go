package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// T85 (docs/fork-plan/tickets/129-T85-contacts-list-fts-search.md): GET
// /contacts?search= gains an FTS5 clause, OR-ed onto the existing LIKE
// clause, so it composes with the list's circle/archived/sort/cursor
// filters. These tests run against database.InitDB (not AutoMigrate) — the
// contacts_fts virtual table + triggers live in the hand-written migration
// SQL (000007, widened by 000010/000020) and do not exist under GORM's
// derived schema.

// ftsRealRouter builds a real-migrated-schema router with GET /contacts
// wired, for T85's FTS-composition tests.
func ftsRealRouter(t *testing.T, dbName string) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), dbName))
	require.NoError(t, err)

	user := models.User{Username: "fts-" + dbName, Password: "password123!A", Email: "fts-" + dbName + "@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.GET("/contacts", GetContacts)
	return db, router, user
}

// ftsSearch issues one GET /contacts?<query> and returns the decoded
// contacts array.
func ftsSearch(t *testing.T, router *gin.Engine, query string) []map[string]any {
	t.Helper()
	items, _ := ftsSearchPage(t, router, query)
	return items
}

// ftsSearchPage is ftsSearch plus the response's next_cursor, for the
// pagination test.
func ftsSearchPage(t *testing.T, router *gin.Engine, query string) (items []map[string]any, next string) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	raw, _ := body["contacts"].([]any)
	items = make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		items = append(items, r.(map[string]any))
	}
	next, _ = body["next_cursor"].(string)
	return items, next
}

func firstnamesOf(items []map[string]any) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["firstname"].(string)
	}
	return out
}

// TestGetContactsSearch_FTSOnlyMatch pins the core of T85: a contact
// findable only through the new FTS5 clause is returned. `org` is indexed
// into contacts_fts (migration 000007) but was never part of
// applyContactSearch's LIKE clause, so a search on org text is a match the
// LIKE-only implementation could never have produced before this ticket.
func TestGetContactsSearch_FTSOnlyMatch(t *testing.T) {
	db, router, user := ftsRealRouter(t, "fts-only.db")

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Priya", Lastname: "Nakamura", Organization: "Globex Dynamics",
	}).Error)
	// A second contact that must NOT match.
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Omar", Lastname: "Reyes"}).Error)

	items := ftsSearch(t, router, "search=Globex")
	require.Len(t, items, 1, "org text is only reachable via the new FTS clause")
	assert.Equal(t, "Priya", items[0]["firstname"])
}

// TestGetContactsSearch_LIKEOnlyMatchStillWorks pins the ticket's central
// trap: OR-ing in FTS must be strictly additive. FTS is token-prefix, so a
// substring in the middle of a token ("xand" inside "Alexander") matches via
// LIKE but never via FTS MATCH — this must still return the contact exactly
// as it did before T85.
func TestGetContactsSearch_LIKEOnlyMatchStillWorks(t *testing.T) {
	db, router, user := ftsRealRouter(t, "like-only.db")

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alexander", Lastname: "Popov"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Omar", Lastname: "Reyes"}).Error)

	items := ftsSearch(t, router, "search=xand")
	require.Len(t, items, 1, "a mid-token substring must still match via the LIKE clause")
	assert.Equal(t, "Alexander", items[0]["firstname"])
}

// TestGetContactsSearch_ComposesWithCircleArchivedAndSort proves search=
// narrows the same row set circle=/include_archived/sort=name already
// narrow, rather than replacing the list machinery: three org-matching
// contacts are split across circle membership and archive state, and one
// request combining all four params returns exactly the intersection, in
// name order.
func TestGetContactsSearch_ComposesWithCircleArchivedAndSort(t *testing.T) {
	db, router, user := ftsRealRouter(t, "composes.db")

	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)

	inCircleActive := models.Contact{UserID: user.ID, Firstname: "Bella", Lastname: "Ortiz", Organization: "Initech Holdings"}
	notInCircle := models.Contact{UserID: user.ID, Firstname: "Adam", Lastname: "Kessler", Organization: "Initech Holdings"}
	inCircleArchived := models.Contact{UserID: user.ID, Firstname: "Carl", Lastname: "Ibsen", Organization: "Initech Holdings", Archived: true}
	require.NoError(t, db.Create(&inCircleActive).Error)
	require.NoError(t, db.Create(&notInCircle).Error)
	require.NoError(t, db.Create(&inCircleArchived).Error)

	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: inCircleActive.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: inCircleArchived.VCardUID}).Error)

	// search + circle + sort, archived excluded by default: only Bella.
	items := ftsSearch(t, router, "search=Initech&circle=Friends&sort=name&order=asc")
	require.Len(t, items, 1)
	assert.Equal(t, "Bella", items[0]["firstname"])

	// Adding include_archived=true pulls Carl in too, still scoped to the
	// circle and still name-ordered. sort=name orders by sort_name (lastname
	// first): Ibsen sorts before Ortiz, so Carl comes first despite his
	// firstname sorting after Bella's.
	items = ftsSearch(t, router, "search=Initech&circle=Friends&sort=name&order=asc&include_archived=true")
	require.Len(t, items, 2)
	assert.Equal(t, []string{"Carl", "Bella"}, firstnamesOf(items), "name order (by lastname) must hold across the FTS-matched, circle-scoped set")
}

// TestGetContactsSearch_CursorPagesWithNoDuplicatesOrSkips pages a
// search-narrowed result set with a small limit and asserts every matching
// row is returned exactly once — the ticket's core cursor-composition
// requirement, since FTS is used as a filter under the existing (updated_at,
// id) order, never as a competing rank.
func TestGetContactsSearch_CursorPagesWithNoDuplicatesOrSkips(t *testing.T) {
	db, router, user := ftsRealRouter(t, "cursor-paging.db")

	names := []string{"Aiden", "Blair", "Casey", "Devon", "Eden", "Farah"}
	for _, name := range names {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: name, Organization: "Umbrella Robotics"}).Error)
	}
	// A non-matching contact interleaved in id order must never appear.
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zed", Organization: "Nomatch"}).Error)

	var seen []string
	next := ""
	for page := 0; ; page++ {
		q := "search=Umbrella&limit=2"
		if next != "" {
			q += "&cursor=" + next
		}
		items, n := ftsSearchPage(t, router, q)
		seen = append(seen, firstnamesOf(items)...)
		if n == "" {
			break
		}
		next = n
		if page > 10 {
			t.Fatal("search-narrowed cursor pagination did not terminate")
		}
	}

	assert.ElementsMatch(t, names, seen, "every matching contact returned exactly once, no drops or duplicates")
}

// TestGetContactsSearch_CrossUserContactNeverReturned pins the highest-risk
// part of the ticket: the FTS subquery's own contacts_fts.user_id scope,
// stacked with the outer query's existing user_id scope.
func TestGetContactsSearch_CrossUserContactNeverReturned(t *testing.T) {
	db, router, _ := ftsRealRouter(t, "cross-user.db")

	other := models.User{Username: "fts-other", Password: "password123!A", Email: "fts-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Sneaky", Organization: "ZzyzxCorp"}).Error)

	items := ftsSearch(t, router, "search=ZzyzxCorp")
	assert.Empty(t, items, "a contact owned by another user must never be returned via the FTS clause")
}

// TestGetContactsSearch_SoftDeletedContactNeverReturned pins that
// contacts_fts (which indexes soft-deleted rows until the trigger removes
// them) cannot be trusted to filter deletion on its own — the outer query's
// GORM soft-delete scope must still apply.
func TestGetContactsSearch_SoftDeletedContactNeverReturned(t *testing.T) {
	db, router, user := ftsRealRouter(t, "soft-deleted.db")

	contact := models.Contact{UserID: user.ID, Firstname: "Ghost", Organization: "Wraithworks"}
	require.NoError(t, db.Create(&contact).Error)

	items := ftsSearch(t, router, "search=Wraithworks")
	require.Len(t, items, 1, "findable before deletion")

	require.NoError(t, db.Delete(&contact).Error)
	items = ftsSearch(t, router, "search=Wraithworks")
	assert.Empty(t, items, "a soft-deleted contact must not be findable via the FTS clause")
}

// TestGetContactsSearch_OneCharacterTermUnchanged pins the two-rune gate
// (matching services.Search's own gate): below two runes the FTS clause
// never runs, so a one-character search behaves exactly as it did before
// T85. Two contacts isolate the two clauses from each other: "Gordon"'s
// firstname is only ever LIKE-reachable; "Zoe"'s organization is only ever
// FTS-reachable (org is not part of the LIKE clause) and shares no
// characters with "Zoe" itself, so a match on it can only come from FTS.
func TestGetContactsSearch_OneCharacterTermUnchanged(t *testing.T) {
	db, router, user := ftsRealRouter(t, "one-char.db")

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Gordon"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zoe", Organization: "Globex Dynamics"}).Error)

	// At one rune, the FTS clause must be gated off entirely: "G" matches
	// Gordon via the ordinary LIKE clause (unchanged from before T85) but
	// must NOT reach Zoe through her org, even though "G" is a prefix of the
	// org token "globex".
	items := ftsSearch(t, router, "search=G")
	require.Len(t, items, 1, "a one-character term must behave exactly as it did before T85")
	assert.Equal(t, "Gordon", items[0]["firstname"])

	// At two runes, the FTS clause activates: "Gl" is not a substring of
	// either firstname (so LIKE alone matches nothing), but it IS a prefix
	// of the org token "globex" — reaching Zoe only through the new clause.
	items = ftsSearch(t, router, "search=Gl")
	require.Len(t, items, 1, "a two-character term must activate the FTS clause")
	assert.Equal(t, "Zoe", items[0]["firstname"])
}

// TestGetContactsSearch_SpecialCharactersDoNotError is the contacts-list
// counterpart to services.TestSearch_SpecialCharactersDoNotError, and it
// exists because T85 put user input on a path to FTS5 MATCH that it had
// never reached before. An unescaped quote or bare operator is an FTS5
// syntax error, which on THIS endpoint is a 500 on the main contact list —
// the one `search=` has three live consumers on (web list, web AppBar
// autocomplete, Android list), versus /search's single one. ftsQuery's
// quoting is what prevents it; this pins that the protection actually
// reaches this caller rather than being assumed from the other one.
func TestGetContactsSearch_SpecialCharactersDoNotError(t *testing.T) {
	db, router, user := ftsRealRouter(t, "special-chars.db")

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Quick", Lastname: "Fox"}).Error)

	inputs := []string{
		`say "hi"`,         // embedded quotes
		`it's`,             // apostrophe
		`AND OR NOT`,       // FTS5 boolean operators, bare
		`NEAR(quick, fox)`, // FTS5 operator syntax
		`quick*`,           // user-supplied wildcard
		`a "b" c`,          // quotes mid-phrase
		`-exclude`,         // negation operator
		`"`,                // lone quote
		`*`,                // lone wildcard
		`"unclosed`,        // unbalanced quote
		`firstname:Quick`,  // FTS5 column-filter syntax
		`^caret`,           // initial-token operator
		`{a}`,              // column-group syntax
		`100%`,             // LIKE wildcard, harmless but URL-escaped
		`a+b`,              // '+' must survive URL encoding intact
		`x&y`,              // '&' must not truncate the query string
		`( ) - . /`,        // phone punctuation with no digits at all
	}
	for _, in := range inputs {
		req, _ := http.NewRequest("GET", "/contacts?search="+url.QueryEscape(in), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "input %q must not error: %s", in, w.Body.String())
	}
}

// TestGetContactsSearch_NameSortedCursorPagesSearchedSet pages a
// search-narrowed set under sort=name — the (sort_name, id) cursor rather
// than the default (updated_at, id) one. This is the pairing T86 will
// actually ship on, since T77 makes name the web default, and it is the
// case the ticket's "FTS as a filter, not a ranker" decision is really
// about: the FTS clause must narrow the row set without disturbing the
// name cursor's total order.
func TestGetContactsSearch_NameSortedCursorPagesSearchedSet(t *testing.T) {
	db, router, user := ftsRealRouter(t, "name-cursor.db")

	// Created in non-alphabetical order so a correct result cannot come from
	// insertion order alone.
	lastnames := []string{"Ibsen", "Ortiz", "Adler", "Zhang", "Meyer", "Crane"}
	for _, ln := range lastnames {
		require.NoError(t, db.Create(&models.Contact{
			UserID: user.ID, Firstname: "Sam", Lastname: ln, Organization: "Umbrella Robotics",
		}).Error)
	}
	// A non-matching contact whose sort_name falls in the middle of the run
	// must never appear on any page.
	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Nope", Lastname: "Nadir", Organization: "Nomatch",
	}).Error)

	var seen []string
	next := ""
	for page := 0; ; page++ {
		q := "search=Umbrella&sort=name&order=asc&limit=2"
		if next != "" {
			q += "&cursor=" + next
		}
		items, n := ftsSearchPage(t, router, q)
		for _, it := range items {
			seen = append(seen, it["lastname"].(string))
		}
		if n == "" {
			break
		}
		next = n
		if page > 10 {
			t.Fatal("name-sorted search pagination did not terminate")
		}
	}

	assert.Equal(t, []string{"Adler", "Crane", "Ibsen", "Meyer", "Ortiz", "Zhang"}, seen,
		"the name cursor must walk the FTS-narrowed set in sort_name order, exactly once each")
}
