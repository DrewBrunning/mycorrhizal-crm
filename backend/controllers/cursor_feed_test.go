package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupRouterWithRetention is setupRouter (activity_controller_test.go) with a
// non-zero DELETE_RETENTION_DAYS so ?since= feed cursors are not all rejected
// as too old. The T17 feed tests need a real retention window; the default
// test router's config has zero.
func setupRouterWithRetention(retentionDays int) (*gorm.DB, *gin.Engine) {
	gin.SetMode(gin.ReleaseMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	db.AutoMigrate(&models.Contact{}, &models.Activity{}, &models.Note{}, models.Reminder{}, models.User{}, models.Webhook{}, models.WebhookDelivery{}, models.ContactSubscription{}, models.ContactSyncLink{}, models.RelationshipEdge{}, models.Circle{}, models.CircleMember{}, models.Tag{}, models.ContactTag{}, models.LifeEvent{}, models.Household{}, models.HouseholdMember{}, models.FieldDefinition{}, models.FieldValue{}, models.CardDAVSync{}, models.ApiToken{}, models.ReminderCompletion{}, models.CalendarSubscription{}, models.CalendarEventLink{}, models.Preference{})

	user := models.User{Username: "tester", Password: "password123", Email: "tester@example.com"}
	if err := db.Create(&user).Error; err != nil {
		panic("failed to seed user")
	}

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: os.Getenv("PROFILE_PHOTO_DIR"), DeleteRetentionDays: retentionDays})
		c.Next()
	})

	return db, router
}

// contactsResponse is the slim decoder the feed tests use to read the T17
// cursor envelope (there is no total/page anymore).
type contactsResponse struct {
	Contacts   []map[string]any `json:"contacts"`
	NextCursor string           `json:"next_cursor"`
	Limit      int              `json:"limit"`
	Sync       map[string]any   `json:"sync"`
}

func getContactsPage(t *testing.T, router *gin.Engine, query string) contactsResponse {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp contactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// TestCursorEncodeDecodeRoundTrip proves a cursor survives the base64url
// round trip with both timestamp and id intact, and that the timestamp keeps
// its offset (the DATETIME-as-text comparison depends on the bound value
// matching the stored format — see EncodeCursor's doc comment).
func TestCursorEncodeDecodeRoundTrip(t *testing.T) {
	for _, id := range []any{uint(7), uint64(42), "1f2e3d4c-5b6a-7890-abcd-ef1234567890"} {
		ts := time.Date(2026, 8, 2, 18, 7, 19, 402476499, time.FixedZone("CDT", -5*3600))
		raw := EncodeCursor(ts, id)
		cur, err := DecodeCursor(raw)
		require.NoError(t, err)
		assert.True(t, ts.Equal(cur.UpdatedAt), "timestamp must survive the round trip")
		_, off := cur.UpdatedAt.Zone()
		assert.Equal(t, -5*3600, off, "timestamp offset must survive the round trip")
		assert.Equal(t, fmt.Sprint(id), cur.ID, "id must survive the round trip")
	}
}

// TestCursorDecodeRejectsMalformed pins the 400 path: garbage, truncated
// base64url, missing id, and a non-timestamp payload must all fail decode.
func TestCursorDecodeRejectsMalformed(t *testing.T) {
	for _, raw := range []string{
		"",
		"!!!not-base64url!!!",
		encodeRawURL("2026-08-02T18:07:19.402476499-05:00|"), // missing id
		encodeRawURL("not-a-time|7"),                         // bad timestamp
		"aGVsbG8=",                                           // decodes but has no | separator
	} {
		_, err := DecodeCursor(raw)
		assert.Error(t, err, "cursor %q should fail to decode", raw)
	}
}

// encodeRawURL is a tiny mirror of EncodeCursor's encoding for test inputs.
func encodeRawURL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// TestGetContactsCursorStableUnderBoundaryInsert is the T17 stability test —
// the bug the (updated_at, id) cursor scheme exists to prevent. Six contacts
// are paged through with limit=2; a seventh is inserted AT the page boundary
// mid-walk (between the last row of page 1 and the first row of page 2).
// Cursor pagination must still return every row exactly once — no drops, no
// duplicates.
func TestGetContactsCursorStableUnderBoundaryInsert(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	var seeded []models.Contact
	for _, name := range []string{"A", "B", "C", "D", "E", "F"} {
		c := models.Contact{UserID: user.ID, Firstname: name}
		require.NoError(t, db.Create(&c).Error)
		seeded = append(seeded, c)
	}

	// Page 1: newest two by (updated_at, id) DESC.
	page1 := getContactsPage(t, router, "limit=2")
	require.Len(t, page1.Contacts, 2)
	require.NotEmpty(t, page1.NextCursor, "a full page must carry a next_cursor")

	// Decode page 1's last row position and insert a new contact just after
	// it in the ordering — i.e. exactly at the page boundary.
	boundary := page1.Contacts[1]
	boundaryID := uint(boundary["id"].(float64))
	var last models.Contact
	require.NoError(t, db.First(&last, boundaryID).Error)

	inserted := models.Contact{UserID: user.ID, Firstname: "G"}
	require.NoError(t, db.Create(&inserted).Error)
	boundaryTime := last.UpdatedAt.Add(-time.Nanosecond)
	require.NoError(t, db.Model(&inserted).Update("updated_at", boundaryTime).Error)

	// Walk every page following next_cursor and collect ids.
	seen := map[uint]int{}
	next := ""
	for page := 0; ; page++ {
		query := "limit=2"
		if next != "" {
			query += "&cursor=" + next
		}
		resp := getContactsPage(t, router, query)
		for _, raw := range resp.Contacts {
			seen[uint(raw["id"].(float64))]++
		}
		if resp.NextCursor == "" {
			break
		}
		next = resp.NextCursor
		if page > 10 {
			t.Fatal("cursor pagination did not terminate")
		}
	}

	// All seven contacts, each exactly once.
	assert.Len(t, seen, 7, "every contact must appear")
	for id, count := range seen {
		assert.Equal(t, 1, count, "contact %d must not be dropped or duplicated", id)
	}
}

// TestChangeFeedSinceAndTombstones is the change-feed contract test: ?since=
// returns only rows changed after the cursor, ordered forward, AND a
// soft-deleted row is returned as a deletion (deleted:true) rather than
// silently vanishing.
func TestChangeFeedSinceAndTombstones(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)

	// Seed three contacts, then archive one (archived rows must still appear
	// in the feed — a replica needs the whole picture). Archiving bumps
	// Alice's updated_at past Bob/Carol's.
	a := models.Contact{UserID: user.ID, Firstname: "Alice"}
	b := models.Contact{UserID: user.ID, Firstname: "Bob"}
	c := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Model(&a).Update("archived", true).Error)

	// Cursor positioned after Carol (the last-created row).
	afterCarol := EncodeCursor(c.UpdatedAt, c.ID)

	// The feed must be strictly after the cursor: only Alice's archive
	// update. Bob is before the cursor; Carol is the cursor row itself.
	feed := getContactsPage(t, router, "since="+afterCarol+"&limit=10")
	require.Len(t, feed.Contacts, 1, "only the row changed after the cursor should appear")
	assert.Equal(t, "Alice", feed.Contacts[0]["firstname"])
	assert.Equal(t, true, feed.Contacts[0]["archived"], "archived contacts must appear in the feed")
	assert.Equal(t, "incremental", feed.Sync["mode"])

	// Soft-delete Carol, then replay the same since: Carol must come back as
	// a deletion, not vanish. Contact.AfterDelete bumps her updated_at past
	// the cursor so the tombstone actually lands in the feed.
	require.NoError(t, db.Delete(&c).Error)
	feed2 := getContactsPage(t, router, "since="+afterCarol+"&limit=10")
	require.Len(t, feed2.Contacts, 2, "Alice's archive update and Carol's tombstone both changed after the cursor")
	byName := map[string]map[string]any{}
	for _, raw := range feed2.Contacts {
		byName[raw["firstname"].(string)] = raw
	}
	assert.Equal(t, true, byName["Carol"]["deleted"], "soft-deleted rows must be returned with deleted:true")
	assert.NotEqual(t, true, byName["Alice"]["deleted"], "a live row must not be marked deleted")
}

// TestChangeFeedCursorTooOld410 pins the sync-horizon contract (T17/T26): a
// ?since= cursor older than DELETED_RETENTION_DAYS returns 410 Gone telling
// the client to full-resync, because tombstones in that range were purged.
func TestChangeFeedCursorTooOld410(t *testing.T) {
	_, router := setupRouterWithRetention(30)
	router.GET("/contacts", GetContacts)

	tooOld := EncodeCursor(time.Now().AddDate(0, 0, -40), uint(1))
	req, _ := http.NewRequest("GET", "/contacts?since="+tooOld, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusGone, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errBody := body["error"].(map[string]any)
	assert.Equal(t, "GONE", errBody["code"])
	assert.Contains(t, errBody["message"].(string), "full resync")

	// A plain ?cursor= (browse navigation, not long-lived sync state) is NOT
	// subject to the horizon — no 410.
	browseReq, _ := http.NewRequest("GET", "/contacts?cursor="+tooOld, nil)
	browseW := httptest.NewRecorder()
	router.ServeHTTP(browseW, browseReq)
	require.Equal(t, http.StatusOK, browseW.Code)
}

// TestChangeFeedMalformedCursor400 pins that a garbage ?since= is a 400, not
// a 500.
func TestChangeFeedMalformedCursor400(t *testing.T) {
	_, router := setupRouterWithRetention(30)
	router.GET("/contacts", GetContacts)

	req, _ := http.NewRequest("GET", "/contacts?since=!!!not-a-cursor!!!", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestChangeFeedNotesTombstones covers the same tombstone trap for the notes
// feed: a soft-deleted note must surface via ?since= as deleted:true (Note's
// AfterDelete hook advances updated_at so the cursor moves past it).
func TestChangeFeedNotesTombstones(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)
	router.GET("/notes", GetUnassignedNotes)

	n1 := models.Note{UserID: user.ID, Content: "one"}
	n2 := models.Note{UserID: user.ID, Content: "two"}
	require.NoError(t, db.Create(&n1).Error)
	require.NoError(t, db.Create(&n2).Error)

	// Cursor positioned after the first note.
	afterOne := EncodeCursor(n1.UpdatedAt, n1.ID)

	req := func(query string) map[string]any {
		t.Helper()
		r, _ := http.NewRequest("GET", "/notes?"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body
	}

	feed := req("since=" + afterOne + "&limit=10")
	notes := feed["notes"].([]any)
	require.Len(t, notes, 1, "only the note after the cursor should appear")
	assert.Equal(t, "two", notes[0].(map[string]any)["content"])
	assert.Equal(t, "incremental", feed["sync"].(map[string]any)["mode"])

	// Soft-delete the second note, replay the same since: it comes back as a
	// deletion.
	require.NoError(t, db.Delete(&n2).Error)
	feed2 := req("since=" + afterOne + "&limit=10")
	notes2 := feed2["notes"].([]any)
	require.Len(t, notes2, 1)
	assert.Equal(t, "two", notes2[0].(map[string]any)["content"])
	assert.Equal(t, true, notes2[0].(map[string]any)["deleted"], "soft-deleted notes must be returned with deleted:true")
}

// TestFeedIndexesExistInMigratedSchema is the real-DB check for migration
// 000043: the composite (user_id, updated_at, id) indexes must exist on every
// paginated table in the real migrated schema (the cursor query degrades to a
// scan without them). Uses database.InitDB, not AutoMigrate.
func TestFeedIndexesExistInMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "feed-indexes.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	for _, table := range []string{
		"contacts", "notes", "activities", "life_events", "preferences",
		"circles", "households", "tags", "relationship_edges", "field_definitions",
	} {
		var count int64
		err := db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ? AND tbl_name = ?",
			"idx_"+table+"_feed", table,
		).Scan(&count).Error
		require.NoError(t, err)
		assert.Equal(t, int64(1), count, "migration 000043 must create idx_%s_feed", table)
	}
}
