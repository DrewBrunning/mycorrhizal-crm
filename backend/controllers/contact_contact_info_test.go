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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// contactInfoPage fetches one GET /contacts page and returns the items plus
// the response's hidden_count, if any ("" / nil when absent). Mirrors
// sortPage's shape so the T103 tests read the same way.
func contactInfoPage(t *testing.T, router *gin.Engine, query string) ([]map[string]any, any) {
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
	return out, body["hidden_count"]
}

func contactInfoFirstnames(items []map[string]any) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["firstname"].(string)
	}
	return out
}

// walkContactInfoPages collects every contact returned across all pages of a
// has_contact_info-filtered walk, following next_cursor until it runs out —
// the filtered-set pagination-total-order check. Returns the first page's
// hidden_count too, so the "whole-set count on every page" contract can be
// asserted.
func walkContactInfoPages(t *testing.T, router *gin.Engine, baseQuery string, limit int) ([]string, any, map[string]any) {
	t.Helper()
	var all []string
	var firstHidden any
	var firstBody map[string]any
	next := ""
	for page := 0; ; page++ {
		query := baseQuery + "&limit=" + strconv.Itoa(limit)
		if next != "" {
			query += "&cursor=" + next
		}
		req, _ := http.NewRequest("GET", "/contacts?"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		items, _ := body["contacts"].([]any)
		for _, raw := range items {
			all = append(all, raw.(map[string]any)["firstname"].(string))
		}
		n, _ := body["next_cursor"].(string)
		if page == 0 {
			firstHidden = body["hidden_count"]
			firstBody = body
		}
		if n == "" {
			break
		}
		next = n
		if page > 20 {
			t.Fatal("filtered pagination did not terminate")
		}
	}
	return all, firstHidden, firstBody
}

// TestGetContacts_ContactInfoFilterOffByDefault pins the API default: without
// ?has_contact_info=, every contact is returned — the filter is opt-in for API
// consumers; only the web Contacts page turns it on by default. Explicit
// has_contact_info=false must behave identically (all rows, no hidden_count).
func TestGetContacts_ContactInfoFilterOffByDefault(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	for _, c := range []models.Contact{
		{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"},
		{UserID: user.ID, Firstname: "Pet"},
		{UserID: user.ID, Firstname: "Bob", Phones: []models.ContactPhone{{Type: "cell", Value: "+15550001111"}}},
	} {
		require.NoError(t, db.Create(&c).Error)
	}

	items, hidden := contactInfoPage(t, router, "limit=10")
	require.Len(t, items, 3, "no param must return every contact")
	assert.Nil(t, hidden, "hidden_count must be absent when the filter is off")

	items, hidden = contactInfoPage(t, router, "has_contact_info=false&limit=10")
	require.Len(t, items, 3, "explicit has_contact_info=false must return every contact, stubs included")
	assert.Nil(t, hidden, "hidden_count must be absent when the filter is explicitly off")
}

// TestGetContacts_ContactInfoFilterNullScalars is the COALESCE regression
// test: a row whose flat email/phone are NULL (raw-SQL/legacy data — GORM
// writes ” instead, but the column is nullable) must be hidden by the filter
// AND counted in hidden_count. Without the COALESCE, `length(trim(NULL)) > 0`
// is NULL, and the count's `NOT (clause)` is then NULL too — the row vanishes
// from the list but is silently not counted as hidden, under-reporting the
// disclosure.
func TestGetContacts_ContactInfoFilterNullScalars(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}).Error)
	stub := models.Contact{UserID: user.ID, Firstname: "NullScalar"}
	require.NoError(t, db.Create(&stub).Error)
	require.NoError(t, db.Exec("UPDATE contacts SET email = NULL, phone = NULL WHERE id = ?", stub.ID).Error)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&limit=10")
	assert.Equal(t, []string{"Alice"}, contactInfoFirstnames(items))
	assert.Equal(t, float64(1), hidden, "a NULL-scalar row is hidden AND must be counted as hidden")
}

// TestGetContacts_ContactInfoFilterFlatScalars pins the cheap path: a
// contactable contact via the flat email or phone scalar is included, and a
// contact with no contact fields at all (a pet or relationship stub) is
// excluded. Also pins hidden_count: 1 of 3 excluded.
func TestGetContacts_ContactInfoFilterFlatScalars(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	for _, c := range []models.Contact{
		{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"},
		{UserID: user.ID, Firstname: "Bob", Phone: "+15550002222"},
		{UserID: user.ID, Firstname: "Pet", Lastname: "Rex"}, // no contact fields
	} {
		require.NoError(t, db.Create(&c).Error)
	}

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&limit=10")
	assert.ElementsMatch(t, []string{"Alice", "Bob"}, contactInfoFirstnames(items),
		"only contacts with a non-empty email or phone scalar are contactable")
	assert.Equal(t, float64(1), hidden, "one contact (the stub) is hidden")
}

// TestGetContacts_ContactInfoFilterArrayOnly pins the ticket's core case: a
// contact whose only email lives in the emails JSON array and NOT in the flat
// email scalar must still count as contactable. Built with SkipHooks so
// BeforeSave's scalar derivation never runs — mirroring the real-data rows the
// predicate's json_each leg exists for.
func TestGetContacts_ContactInfoFilterArrayOnly(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&models.Contact{
		UserID:    user.ID,
		Firstname: "ArrayOnly",
		Emails:    []models.ContactEmail{{Type: "home", Value: "array@only.example"}},
	}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&models.Contact{
		UserID:    user.ID,
		Firstname: "URLOnly",
		URLs:      []models.ContactURL{{Type: "work", Value: "https://only.example"}},
	}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&models.Contact{
		UserID:    user.ID,
		Firstname: "PhoneArrayOnly",
		Phones:    []models.ContactPhone{{Type: "cell", Value: "+15550003333"}},
	}).Error)
	// A stub with an array entry whose value is whitespace must NOT count —
	// "non-empty entry" means non-empty.
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&models.Contact{
		UserID:    user.ID,
		Firstname: "BlankEmail",
		Emails:    []models.ContactEmail{{Type: "home", Value: "   "}},
	}).Error)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&limit=10")
	assert.ElementsMatch(t, []string{"ArrayOnly", "PhoneArrayOnly", "URLOnly"}, contactInfoFirstnames(items),
		"array-only emails, phones and URLs must count; a whitespace-only entry must not")
	assert.Equal(t, float64(1), hidden, "only the whitespace-only stub is hidden")
}

// TestGetContacts_ContactInfoFilterHiddenCountScoped pins that hidden_count
// reflects the CURRENT filter scope (archive + circle here), not the whole
// table: an archived or different-circle contact that lacks contact info is
// not part of the set the user is looking at, so it is not counted as hidden.
func TestGetContacts_ContactInfoFilterHiddenCountScoped(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Stub"}).Error)
	archived := models.Contact{UserID: user.ID, Firstname: "ArchivedStub"}
	require.NoError(t, db.Create(&archived).Error)
	require.NoError(t, db.Model(&archived).UpdateColumn("archived", true).Error)

	// Default scope (non-archived): the archived stub is out of scope.
	_, hidden := contactInfoPage(t, router, "has_contact_info=true&limit=10")
	assert.Equal(t, float64(1), hidden, "archived stubs are outside the default scope and must not be counted as hidden")

	// include_archived widens the scope: both stubs are now hidden.
	_, hidden = contactInfoPage(t, router, "has_contact_info=true&include_archived=true&limit=10")
	assert.Equal(t, float64(2), hidden, "include_archived must widen the hidden set to match the visible scope")
}

// TestGetContacts_ContactInfoFilterComposesWithCircle pins that the predicate
// ANDs with the circle filter instead of bypassing it: a contactable contact
// outside the circle stays hidden, and a non-contactable member inside it is
// counted as hidden within that circle's scope.
func TestGetContacts_ContactInfoFilterComposesWithCircle(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	circle := models.Circle{Name: "Friends", UserID: user.ID}
	require.NoError(t, db.Create(&circle).Error)

	inner := models.Contact{UserID: user.ID, Firstname: "InCircle", Email: "in@example.com"}
	outer := models.Contact{UserID: user.ID, Firstname: "OutCircle", Email: "out@example.com"}
	stub := models.Contact{UserID: user.ID, Firstname: "InCircleStub"}
	for _, c := range []*models.Contact{&inner, &outer, &stub} {
		require.NoError(t, db.Create(c).Error)
	}
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, MemberVCardUID: inner.VCardUID, UserID: user.ID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, MemberVCardUID: stub.VCardUID, UserID: user.ID}).Error)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&circle=Friends&limit=10")
	assert.Equal(t, []string{"InCircle"}, contactInfoFirstnames(items),
		"the contactable in-circle contact shows; the out-of-circle one is excluded by the circle filter")
	assert.Equal(t, float64(1), hidden, "the non-contactable in-circle stub is hidden within the circle scope")
}

// TestGetContacts_ContactInfoFilterRejectsMalformed pins the 400 path: any
// has_contact_info value other than true/false is rejected, matching how sort
// treats an unknown value — never a silent fallback.
func TestGetContacts_ContactInfoFilterRejectsMalformed(t *testing.T) {
	_, router := setupRouter()
	router.GET("/contacts", GetContacts)

	for _, bad := range []string{"yes", "1", "TRUE", "on"} {
		req, _ := http.NewRequest("GET", "/contacts?has_contact_info="+bad, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, "has_contact_info=%s must be a 400", bad)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Contains(t, body["error"].(map[string]any)["message"].(string), "has_contact_info")
	}

	// The two valid values still work.
	for _, ok := range []string{"true", "false"} {
		req, _ := http.NewRequest("GET", "/contacts?has_contact_info="+ok, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "has_contact_info=%s must be accepted", ok)
	}
}

// TestGetContacts_ContactInfoFilterFeedUnaffected pins the deliberate decision
// that the ?since= change feed ignores this filter: a feed is sync state and
// must return every changed row regardless of filters, so has_contact_info
// must not narrow it — exactly as archive/search/circle are ignored there.
func TestGetContacts_ContactInfoFilterFeedUnaffected(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	contactable := models.Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	stub := models.Contact{UserID: user.ID, Firstname: "Stub"}
	require.NoError(t, db.Create(&contactable).Error)
	require.NoError(t, db.Create(&stub).Error)
	since := EncodeCursor(contactable.UpdatedAt, contactable.ID)

	req, _ := http.NewRequest("GET", "/contacts?since="+since+"&has_contact_info=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body["contacts"].([]any), 1, "the feed must still return the stub regardless of has_contact_info")
	assert.NotContains(t, body, "hidden_count", "the feed response never carries hidden_count")
}

// TestGetContacts_ContactInfoFilterRealMigratedSchema is trap #1's check for
// T103: the predicate's json_each legs and the flat comparisons must work
// against the REAL migrated schema (database.InitDB), where the emails/phones/
// urls columns are the hand-written migration SQL — not GORM's AutoMigrate
// derivation.
func TestGetContacts_ContactInfoFilterRealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t103-real-filter.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "realdb-contactinfo", Password: "password123!A", Email: "realdb-contactinfo@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Stub"}).Error)
	// Array-only email, flat scalar never populated (real-data shape the
	// json_each leg exists for).
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&models.Contact{
		UserID:    user.ID,
		Firstname: "ArrayOnly",
		Emails:    []models.ContactEmail{{Type: "home", Value: "array@only.example"}},
	}).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: ""})
		c.Next()
	})
	router.GET("/contacts", GetContacts)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&limit=10")
	assert.ElementsMatch(t, []string{"Alice", "ArrayOnly"}, contactInfoFirstnames(items),
		"real-migrated-schema filter must include flat and array-only contactable rows")
	assert.Equal(t, float64(1), hidden, "the stub is hidden on the real schema too")
}

// TestGetContacts_ContactInfoFilterPagesFilteredSet proves the filter
// composes with cursor pagination: walking every page of the filtered set must
// return every contactable contact exactly once (no drops/duplicates at page
// boundaries), while stubs stay out, and every page carries the same
// whole-set hidden_count (not the per-page count).
func TestGetContacts_ContactInfoFilterPagesFilteredSet(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	// Five contactable + two stubs. Contactable names are unique; stubs
	// interleave so pagination must keep excluding them at every boundary.
	for _, c := range []models.Contact{
		{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"},
		{UserID: user.ID, Firstname: "StubOne"},
		{UserID: user.ID, Firstname: "Bob", Phone: "+15550000002"},
		{UserID: user.ID, Firstname: "Carol", Emails: []models.ContactEmail{{Type: "home", Value: "carol@example.com"}}},
		{UserID: user.ID, Firstname: "StubTwo"},
		{UserID: user.ID, Firstname: "Dave", URLs: []models.ContactURL{{Type: "work", Value: "https://dave.example"}}},
		{UserID: user.ID, Firstname: "Eve", Phone: "+15550000005"},
	} {
		require.NoError(t, db.Create(&c).Error)
	}

	got, hidden, _ := walkContactInfoPages(t, router, "has_contact_info=true", 2)
	assert.ElementsMatch(t, []string{"Alice", "Bob", "Carol", "Dave", "Eve"}, got,
		"paging the filtered set must return every contactable contact exactly once and no stubs")
	assert.Equal(t, float64(2), hidden, "the whole-set hidden count must reflect both stubs, on the first page")
}

// TestGetContacts_ContactInfoFilterNameSort pins the filter against T73's
// name-sorted cursor: the predicate ANDs with sort=name, hidden_count is still
// present, and the filtered rows come back in (sort_name, id) order.
func TestGetContacts_ContactInfoFilterNameSort(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zoe", Email: "zoe@example.com"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Ann", Lastname: "Adams", Email: "ann@example.com"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Stub"}).Error)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&sort=name&order=asc&limit=10")
	assert.Equal(t, []string{"Ann", "Zoe"}, contactInfoFirstnames(items),
		"name-sorted filtered list must come back in (sort_name, id) order")
	assert.Equal(t, float64(1), hidden, "hidden_count must survive sort=name")
}

// TestGetContacts_ContactInfoFilterIncludesRelations pins that the
// has_contact_info filter and hidden_count also apply on the
// ContactSummaryWithRelations response branch (?includes=) — the other
// response shape GetContacts can take.
func TestGetContacts_ContactInfoFilterIncludesRelations(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	contactable := models.Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	stub := models.Contact{UserID: user.ID, Firstname: "Stub"}
	require.NoError(t, db.Create(&contactable).Error)
	require.NoError(t, db.Create(&stub).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contactable.ID, Content: "hello", Date: time.Now()}).Error)

	items, hidden := contactInfoPage(t, router, "has_contact_info=true&includes=notes&limit=10")
	require.Len(t, items, 1, "the includes branch must apply the contact-info filter too")
	assert.Equal(t, "Alice", items[0]["firstname"])
	assert.NotEmpty(t, items[0]["notes"], "the notes relation must still be included")
	assert.Equal(t, float64(1), hidden, "hidden_count must be present on the includes branch too")
}
