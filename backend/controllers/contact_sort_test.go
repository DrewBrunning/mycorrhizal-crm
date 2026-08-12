package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sortPage fetches one GET /contacts page and returns its items plus the
// response's next_cursor ("" when there is no next page).
func sortPage(t *testing.T, router *gin.Engine, query string) ([]map[string]any, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	items, _ := body["contacts"].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		out = append(out, raw.(map[string]any))
	}
	next, _ := body["next_cursor"].(string)
	return out, next
}

// walkNameSortedPages collects every contact returned across all pages of a
// name-sorted walk, following next_cursor until it runs out. This is the
// T73 pagination-total-order check: a cursor bug that drops or duplicates a
// row at a page boundary (especially inside a run of equal sort_names) shows
// up here as an unexpected length or order.
func walkNameSortedPages(t *testing.T, router *gin.Engine, order string, limit int) []map[string]any {
	t.Helper()
	var all []map[string]any
	next := ""
	for page := 0; ; page++ {
		query := "sort=name&order=" + order + "&limit=" + strconv.Itoa(limit)
		if next != "" {
			query += "&cursor=" + next
		}
		pageItems, n := sortPage(t, router, query)
		all = append(all, pageItems...)
		if n == "" {
			break
		}
		next = n
		if page > 20 {
			t.Fatal("name-sorted pagination did not terminate")
		}
	}
	return all
}

func firstnames(items []map[string]any) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["firstname"].(string)
	}
	return out
}

func reverseStrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}

// TestGetContacts_NameSortOrdersByName pins the T73 core: GET /contacts
// supports sort=name, ordering by the denormalized sort_name key
// (lower(trim(lastname)) else lower(trim(firstname))) in both directions —
// including the lastname/firstname fallback, the trim, and the lowercase.
func TestGetContacts_NameSortOrdersByName(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	contacts := []models.Contact{
		{UserID: user.ID, Firstname: "Grace", Lastname: "  AARON  "}, // trim + lowercase -> "aaron"
		{UserID: user.ID, Firstname: "Charlie", Lastname: "Adams"},
		{UserID: user.ID, Firstname: "Eve"}, // no lastname -> "eve"
		{UserID: user.ID, Firstname: "Henry", Lastname: "De La Cruz"},
	}
	for i := range contacts {
		require.NoError(t, db.Create(&contacts[i]).Error)
	}

	asc, _ := sortPage(t, router, "sort=name&order=asc&limit=10")
	require.Len(t, asc, 4)
	assert.Equal(t, []string{"Grace", "Charlie", "Henry", "Eve"}, firstnames(asc),
		"ascending name sort must order by lower(trim(lastname)) else lower(trim(firstname))")

	desc, _ := sortPage(t, router, "sort=name&order=desc&limit=10")
	require.Len(t, desc, 4)
	assert.Equal(t, []string{"Eve", "Henry", "Charlie", "Grace"}, firstnames(desc),
		"descending name sort must reverse the ascending order")
}

// TestGetContacts_NameSortPagingIsTotal is the T73 pagination-total-order
// proof: walking every page of a name-sorted list must return every contact
// exactly once, in exactly the expected (sort_name, id) order, with several
// contacts sharing a sort_name (the case the id tiebreak exists for) and
// several with no lastname straddling page boundaries. This is the bug class
// T17's (updated_at, id) scheme exists to prevent, now for the name sort.
func TestGetContacts_NameSortPagingIsTotal(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	// Sorted by sort_name, then id. The three Smiths tie; Grace's padded
	// lastname exercises trim; Eve/Frank exercise the no-lastname fallback.
	contacts := []models.Contact{
		{UserID: user.ID, Firstname: "Zoe", Lastname: "Smith"},
		{UserID: user.ID, Firstname: "Charlie", Lastname: "Adams"},
		{UserID: user.ID, Firstname: "Ada", Lastname: "Smith"},
		{UserID: user.ID, Firstname: "Grace", Lastname: "  AARON  "},
		{UserID: user.ID, Firstname: "Bob", Lastname: "Smith"},
		{UserID: user.ID, Firstname: "David", Lastname: "Brown"},
		{UserID: user.ID, Firstname: "Eve"},
		{UserID: user.ID, Firstname: "Frank"},
		{UserID: user.ID, Firstname: "Henry", Lastname: "De La Cruz"},
	}
	for i := range contacts {
		require.NoError(t, db.Create(&contacts[i]).Error)
	}

	// Expected asc order: by (sort_name, id). ids are assigned in creation
	// order, so the Smith tie (ids 1/3/5: Zoe, Ada, Bob) resolves to
	// Zoe, Ada, Bob by id.
	wantAsc := []string{"Grace", "Charlie", "David", "Henry", "Eve", "Frank", "Zoe", "Ada", "Bob"}
	gotAsc := firstnames(walkNameSortedPages(t, router, "asc", 2))
	assert.Equal(t, wantAsc, gotAsc, "ascending name-sorted paging must return every contact exactly once in (sort_name, id) order")

	wantDesc := reverseStrings(wantAsc)
	gotDesc := firstnames(walkNameSortedPages(t, router, "desc", 3))
	assert.Equal(t, wantDesc, gotDesc, "descending name-sorted paging must return every contact exactly once in reverse (sort_name, id) order")
}

// TestGetContacts_NameSortRejectsUnknownSort pins the 400 path: an
// unrecognized sort value must be a 400, not a silent fallback to the
// default order.
func TestGetContacts_NameSortRejectsUnknownSort(t *testing.T) {
	_, router := setupRouter()
	router.GET("/contacts", GetContacts)

	req, _ := http.NewRequest("GET", "/contacts?sort=firstname", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"].(map[string]any)["message"].(string), "sort")
}

// TestGetContacts_NameSortRejectsSince pins the decided T73 behavior: the
// ?since= change feed is sync state ordered by (updated_at, id), never name
// order, so sort=name combined with since is a 400 — NOT a silent fallback a
// sync client could mistake for a name-ordered feed.
func TestGetContacts_NameSortRejectsSince(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	c := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&c).Error)
	// A valid time-based feed cursor, positioned after the contact.
	since := EncodeCursor(c.UpdatedAt, c.ID)

	req, _ := http.NewRequest("GET", "/contacts?sort=name&since="+since, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// sort=updated_at + since must still work (the feed is the default).
	req2, _ := http.NewRequest("GET", "/contacts?sort=updated_at&since="+since, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
}

// TestGetContacts_NameSortRejectsCrossShapedCursor pins the opaque-cursor
// contract: a cursor minted under one sort is rejected (400) under the
// other, never silently misapplied. A time-shaped cursor under sort=name
// fails DecodeNameCursor's timestamp-shape rejection; a name-shaped cursor
// under the default sort fails DecodeCursor's time parse.
func TestGetContacts_NameSortRejectsCrossShapedCursor(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	c := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&c).Error)

	timeCursor := EncodeCursor(c.UpdatedAt, c.ID)
	req, _ := http.NewRequest("GET", "/contacts?sort=name&cursor="+timeCursor, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, "a time-based cursor must not be accepted under sort=name")

	nameCursor := EncodeNameCursor(c.SortName, c.ID)
	req2, _ := http.NewRequest("GET", "/contacts?cursor="+nameCursor, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusBadRequest, w2.Code, "a name-sorted cursor must not be accepted under the default sort")
}

// TestGetContacts_NameSortComposesWithSearch pins that the T73 sort applies
// on top of the existing list filters (search here) rather than bypassing
// them.
func TestGetContacts_NameSortComposesWithSearch(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zed", Lastname: "Smith"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Ann", Lastname: "Smith"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Jones"}).Error)

	items, _ := sortPage(t, router, "sort=name&order=asc&limit=10&search=Smith")
	require.Len(t, items, 2, "search must still narrow the name-sorted list")
	assert.Equal(t, []string{"Zed", "Ann"}, firstnames(items),
		"the Smith tie must resolve by id (Zed created before Ann), inside a search-narrowed list")
}

// TestGetContacts_NameSortRealMigratedSchema is trap #1's check for T73: the
// name-sort path must work against the REAL migrated schema (database.InitDB),
// where GORM's column derivation cannot silently disagree with the hand-written
// migration SQL for `sort_name`.
func TestGetContacts_NameSortRealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t73-real-sort.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "realdb-sort", Password: "password123!A", Email: "realdb-sort@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zoe", Lastname: "Smith"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Smith"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Eve"}).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: ""})
		c.Next()
	})
	router.GET("/contacts", GetContacts)

	items, _ := sortPage(t, router, "sort=name&order=asc&limit=10")
	require.Len(t, items, 3)
	assert.Equal(t, []string{"Eve", "Zoe", "Ada"}, firstnames(items),
		"real-migrated-schema name sort must use the migrated sort_name column")
}
