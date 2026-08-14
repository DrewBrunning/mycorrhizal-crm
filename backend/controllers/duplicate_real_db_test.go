package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDuplicatePairs_RealMigratedSchema is T93's
// (docs/fork-plan/tickets/137-T93-duplicate-scan-endpoint-and-review.md)
// end-to-end round trip against the real migrated schema (database.InitDB —
// /CLAUDE.md backend trap #1): detection across all three tiers (incl. the
// T68 country-code phone case and a shared NON-primary number, which
// DetectDuplicate cannot see), cross-user isolation, archived inclusion,
// the no-contact-info name-tier exclusion, dismissal persistence across
// scans, idempotent dismissal, phantom-uid rejection, and dismissal cleanup
// on contact delete.
func TestDuplicatePairs_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "duplicates-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "dupuser", Password: "password123!A", Email: "dupuser@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: filepath.Join(t.TempDir(), "photos"), AttachmentsDir: filepath.Join(t.TempDir(), "attachments")})
		c.Next()
	})
	router.GET("/contacts/duplicates", GetDuplicatePairs)
	router.POST("/contacts/duplicates/dismiss", middleware.ValidateJSONMiddleware(&models.DuplicateDismissalInput{}), DismissDuplicatePair)
	router.DELETE("/contacts/:id", DeleteContact)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	scan := func() []models.DuplicatePair {
		w := doJSON("GET", "/contacts/duplicates", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body models.DuplicatePairsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Pairs
	}
	pairHas := func(pairs []models.DuplicatePair, idA, idB uint) *models.DuplicatePair {
		for i := range pairs {
			p := &pairs[i]
			if (p.A.ID == idA && p.B.ID == idB) || (p.A.ID == idB && p.B.ID == idA) {
				return p
			}
		}
		return nil
	}

	// --- seed one duplicate per tier ---------------------------------------
	// Email dupe (same case-insensitive email, different names).
	e1 := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Adams", Email: "alice@example.com"}
	require.NoError(t, db.Create(&e1).Error)
	e2 := models.Contact{UserID: user.ID, Firstname: "Alison", Lastname: "Baker", Email: "ALICE@example.com"}
	require.NoError(t, db.Create(&e2).Error)

	// Name dupe (exact firstname+lastname, different emails).
	n1 := models.Contact{UserID: user.ID, Firstname: "Jane", Lastname: "Doe", Email: "jane1@example.com"}
	require.NoError(t, db.Create(&n1).Error)
	n2 := models.Contact{UserID: user.ID, Firstname: "Jane", Lastname: "Doe", Email: "jane2@example.com"}
	require.NoError(t, db.Create(&n2).Error)

	// Phone dupe across country code + punctuation (the T68 case).
	p1 := models.Contact{UserID: user.ID, Firstname: "Carl", Lastname: "Cook", Phones: []models.ContactPhone{{Value: "+18005551234"}}}
	require.NoError(t, db.Create(&p1).Error)
	p2 := models.Contact{UserID: user.ID, Firstname: "Cindy", Lastname: "Cole", Phones: []models.ContactPhone{{Value: "(800) 555-1234"}}}
	require.NoError(t, db.Create(&p2).Error)

	// Non-primary phone dupe: DetectDuplicate (flat primary only) cannot see
	// this; the phones_normalized-backed tier can.
	q1 := models.Contact{UserID: user.ID, Firstname: "Dan", Lastname: "Dunn", Phones: []models.ContactPhone{{Value: "1112223333"}, {Value: "9998887777"}}}
	require.NoError(t, db.Create(&q1).Error)
	q2 := models.Contact{UserID: user.ID, Firstname: "Dora", Lastname: "Diaz", Phones: []models.ContactPhone{{Value: "9998887777"}}}
	require.NoError(t, db.Create(&q2).Error)

	// Two name-identical contacts with NO contact info — must NOT be paired
	// by the name tier (pets / relationship stubs / "Mum" entries).
	x1 := models.Contact{UserID: user.ID, Firstname: "Mum", Lastname: "Smith"}
	require.NoError(t, db.Create(&x1).Error)
	x2 := models.Contact{UserID: user.ID, Firstname: "Mum", Lastname: "Smith"}
	require.NoError(t, db.Create(&x2).Error)

	// Archived contact + its duplicate — included but flagged.
	r1 := models.Contact{UserID: user.ID, Firstname: "Eve", Lastname: "Ernst", Email: "eve@example.com", Archived: true}
	require.NoError(t, db.Create(&r1).Error)
	r2 := models.Contact{UserID: user.ID, Firstname: "Erin", Lastname: "Eaton", Email: "eve@example.com"}
	require.NoError(t, db.Create(&r2).Error)

	// Cross-user isolation: another user owns a contact with the same email
	// as the email-dupe pair — must never appear.
	otherUser := models.User{Username: "other-dup", Password: "password123!A", Email: "other-dup@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	other := models.Contact{UserID: otherUser.ID, Firstname: "Alice", Lastname: "Adams", Email: "alice@example.com"}
	require.NoError(t, db.Create(&other).Error)

	// Name-tier grouping-key collision (review fix): the lowercased name
	// tuple must group as a TUPLE, never a separator-joined string. "A|B"+"C"
	// and "A"+"B|C" must not collapse onto the same key (both string-concat to
	// "a|b|c"); two contacts with the SAME pipe-containing tuple must still
	// pair. col1+col3 share (A|B, C) -> one name pair; col2 stays out.
	col1 := models.Contact{UserID: user.ID, Firstname: "A|B", Lastname: "C", Email: "col1@example.com"}
	require.NoError(t, db.Create(&col1).Error)
	col2 := models.Contact{UserID: user.ID, Firstname: "A", Lastname: "B|C", Email: "col2@example.com"}
	require.NoError(t, db.Create(&col2).Error)
	col3 := models.Contact{UserID: user.ID, Firstname: "A|B", Lastname: "C", Email: "col3@example.com"}
	require.NoError(t, db.Create(&col3).Error)

	// --- first scan --------------------------------------------------------
	pairs := scan()
	require.Len(t, pairs, 6, "exactly six pairs: email, name, phone, non-primary-phone, archived-email, pipe-name")

	emailPair := pairHas(pairs, e1.ID, e2.ID)
	require.NotNil(t, emailPair, "email dupe must be detected")
	assert.ElementsMatch(t, []string{"email"}, emailPair.Reasons)
	assert.InDelta(t, 0.7, emailPair.Confidence, 0.001)

	namePair := pairHas(pairs, n1.ID, n2.ID)
	require.NotNil(t, namePair, "name dupe must be detected")
	assert.ElementsMatch(t, []string{"name"}, namePair.Reasons)
	assert.InDelta(t, 0.5, namePair.Confidence, 0.001)

	phonePair := pairHas(pairs, p1.ID, p2.ID)
	require.NotNil(t, phonePair, "country-code phone dupe must be detected")
	assert.ElementsMatch(t, []string{"phone"}, phonePair.Reasons)
	assert.InDelta(t, 0.75, phonePair.Confidence, 0.001)

	nonPrimaryPair := pairHas(pairs, q1.ID, q2.ID)
	require.NotNil(t, nonPrimaryPair, "non-primary phone dupe must be detected")
	assert.ElementsMatch(t, []string{"phone"}, nonPrimaryPair.Reasons)
	assert.InDelta(t, 0.75, nonPrimaryPair.Confidence, 0.001)

	archivedPair := pairHas(pairs, r1.ID, r2.ID)
	require.NotNil(t, archivedPair, "archived contact's duplicate must be detected")
	assert.True(t, archivedPair.A.Archived || archivedPair.B.Archived, "the archived contact must be flagged")

	// The pipe-name tuple: col1+col3 are the same exact name, so they pair —
	// but col2 ("A"+"B|C"), which a separator-joined key would wrongly fold
	// into "a|b|c", must not appear in any pair at all.
	pipePair := pairHas(pairs, col1.ID, col3.ID)
	require.NotNil(t, pipePair, "two contacts with the same pipe-containing name must still pair")
	assert.ElementsMatch(t, []string{"name"}, pipePair.Reasons)
	assert.Nil(t, pairHas(pairs, col1.ID, col2.ID), "'A|B'+'C' and 'A'+'B|C' must NOT collide onto one name key")
	assert.Nil(t, pairHas(pairs, col2.ID, col3.ID), "'A'+'B|C' must not pair with 'A|B'+'C'")

	assert.Nil(t, pairHas(pairs, x1.ID, x2.ID), "name-identical contacts with no contact info must not be paired")
	assert.Nil(t, pairHas(pairs, e1.ID, other.ID), "another user's contact must never appear")

	// Strongest-first ordering: phone (0.75) before email (0.7) before name (0.5).
	assert.True(t, pairs[0].Confidence >= pairs[len(pairs)-1].Confidence, "pairs must be ordered strongest first")

	// --- dismissal persists across scans ------------------------------------
	w := doJSON("POST", "/contacts/duplicates/dismiss", models.DuplicateDismissalInput{UIDA: e1.VCardUID, UIDB: e2.VCardUID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	pairs = scan()
	require.Len(t, pairs, 5, "dismissed pair must not be re-offered")
	assert.Nil(t, pairHas(pairs, e1.ID, e2.ID))

	// Dismissing the same pair again (reversed order) is an idempotent 200.
	w = doJSON("POST", "/contacts/duplicates/dismiss", models.DuplicateDismissalInput{UIDA: e2.VCardUID, UIDB: e1.VCardUID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// A dismissal referencing a contact that isn't the user's must 404.
	w = doJSON("POST", "/contacts/duplicates/dismiss", models.DuplicateDismissalInput{UIDA: p1.VCardUID, UIDB: other.VCardUID})
	require.Equal(t, http.StatusNotFound, w.Code, "dismissing a pair that references another user's contact must 404")

	// --- dismissing a pair then deleting one contact cleans the row up -------
	w = doJSON("POST", "/contacts/duplicates/dismiss", models.DuplicateDismissalInput{UIDA: p1.VCardUID, UIDB: p2.VCardUID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var dismissalCount int64
	require.NoError(t, db.Model(&models.DismissedDuplicatePair{}).
		Where("user_id = ? AND uid_low = ? AND uid_high = ?", user.ID, orderedUID(p1.VCardUID, p2.VCardUID, true), orderedUID(p1.VCardUID, p2.VCardUID, false)).
		Count(&dismissalCount).Error)
	assert.Equal(t, int64(1), dismissalCount, "dismissal row must exist before the delete")

	w = doJSON("DELETE", "/contacts/"+strconv.FormatUint(uint64(p2.ID), 10), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.NoError(t, db.Model(&models.DismissedDuplicatePair{}).
		Where("user_id = ? AND (uid_low = ? OR uid_high = ?)", user.ID, p2.VCardUID, p2.VCardUID).
		Count(&dismissalCount).Error)
	assert.Zero(t, dismissalCount, "deleting either contact must clean up its dismissal rows")

	// And the deleted contact's other pairs disappear from the scan.
	pairs = scan()
	assert.Nil(t, pairHas(pairs, p1.ID, p2.ID))
}

// TestDuplicatePairs_EmptyResultIsJSONArrayNotNull is the CLAUDE.md trap #8
// raw-JSON assertion: an account with no candidate pairs must get literal
// `"pairs":[]`, never `"pairs":null` — decoding into a Go struct would make
// the two indistinguishable, which is exactly how this class of bug ships.
func TestDuplicatePairs_EmptyResultIsJSONArrayNotNull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "duplicates-empty.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "dupempty", Password: "password123!A", Email: "dupempty@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/contacts/duplicates", GetDuplicatePairs)

	req, _ := http.NewRequest("GET", "/contacts/duplicates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"pairs":[],"total":0,"page":1,"limit":25}`, w.Body.String())
	assert.Contains(t, w.Body.String(), `"pairs":[]`, "must serialize as an empty array, not null")
	assert.NotContains(t, w.Body.String(), `"pairs":null`)
}

// orderedUID returns a or b in the given position, matching the controller's
// lexicographic ordering of the pair into uid_low/uid_high.
func orderedUID(a, b string, wantLow bool) string {
	if (a < b) == wantLow {
		return a
	}
	return b
}

// TestDuplicatePairs_DismissIdempotentUnderConcurrency pins the idempotency
// contract against the race that a count-then-create implementation would
// have: N concurrent dismissals of the same pair must all return 200 and
// produce exactly one dismissal row. The unique index is the guard, so the
// "losing" writers hit gorm.ErrDuplicatedKey and that must be treated as
// success, not a 500.
func TestDuplicatePairs_DismissIdempotentUnderConcurrency(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "duplicates-concurrent.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "dupconc", Password: "password123!A", Email: "dupconc@example.com"}
	require.NoError(t, db.Create(&user).Error)

	a := models.Contact{UserID: user.ID, Firstname: "Conc", Lastname: "One", Email: "conc@example.com"}
	require.NoError(t, db.Create(&a).Error)
	b := models.Contact{UserID: user.ID, Firstname: "Conc", Lastname: "Two", Email: "conc@example.com"}
	require.NoError(t, db.Create(&b).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/duplicates/dismiss", middleware.ValidateJSONMiddleware(&models.DuplicateDismissalInput{}), DismissDuplicatePair)

	const workers = 8
	statuses := make([]int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body, _ := json.Marshal(models.DuplicateDismissalInput{UIDA: a.VCardUID, UIDB: b.VCardUID})
			req, _ := http.NewRequest("POST", "/contacts/duplicates/dismiss", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			statuses[idx] = w.Code
		}(i)
	}
	wg.Wait()

	for i, code := range statuses {
		assert.Equal(t, http.StatusOK, code, "dismissal %d must be an idempotent 200 under concurrency (body: see log)", i)
	}

	var count int64
	require.NoError(t, db.Model(&models.DismissedDuplicatePair{}).
		Where("user_id = ? AND uid_low = ? AND uid_high = ?", user.ID, orderedUID(a.VCardUID, b.VCardUID, true), orderedUID(a.VCardUID, b.VCardUID, false)).
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "exactly one dismissal row must exist after concurrent dismissals")
}
