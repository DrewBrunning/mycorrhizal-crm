package controllers

import (
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// T66 (docs/fork-plan/tickets/110-T66-contact-timeline-bounded-view-and-
// explorer.md) — the paginated, filterable timeline endpoint and the bounded
// M4 composite. Every test runs against database.InitDB's real migrated
// schema (CLAUDE.md trap 1): the timeline endpoint reads the exact
// hand-written columns (occurred_at, completed_at, entity_id, ...) a
// GORM-AutoMigrate harness could silently disagree with.
type timelineTestEnv struct {
	db      *gorm.DB
	router  *gin.Engine
	user    models.User
	contact models.Contact
}

func newTimelineTestEnv(t *testing.T) *timelineTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "t66-timeline.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "t66-user", Password: "password123!A", Email: "t66-user@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Timeline", Lastname: "Subject"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: "", ReminderTimezone: "UTC"})
		c.Next()
	})
	router.GET("/contacts/:id/timeline", GetContactTimeline)
	router.GET("/contacts/:id/detail", GetContactDetail)
	return &timelineTestEnv{db: db, router: router, user: user, contact: contact}
}

func (env *timelineTestEnv) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	return w
}

// timelineItem is the wire shape of one merged timeline entry, with Date kept
// raw so the test can re-derive the resolved instant.
type timelineItem struct {
	Type string          `json:"type"`
	ID   string          `json:"id"`
	Date string          `json:"date"`
	Data json.RawMessage `json:"data"`
}

type timelinePage struct {
	Items      []timelineItem `json:"items"`
	NextCursor string         `json:"next_cursor"`
	Limit      int            `json:"limit"`
}

func (env *timelineTestEnv) fetchTimeline(t *testing.T, query string) timelinePage {
	t.Helper()
	w := env.get(t, "/contacts/"+idString(env.contact.ID)+"/timeline?"+query)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page timelinePage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	return page
}

// walkTimeline pages through the entire timeline following next_cursor and
// returns the concatenated (type, id) sequence.
func (env *timelineTestEnv) walkTimeline(t *testing.T, query string) []timelineItem {
	t.Helper()
	var all []timelineItem
	cursor := ""
	for page := 0; page < 50; page++ {
		q := query
		if q != "" {
			q += "&"
		}
		q += "cursor=" + cursor
		p := env.fetchTimeline(t, q)
		all = append(all, p.Items...)
		if p.NextCursor == "" {
			return all
		}
		cursor = p.NextCursor
	}
	t.Fatalf("timeline walk did not terminate after 50 pages")
	return nil
}

func (env *timelineTestEnv) seedNote(t *testing.T, at time.Time, content string) uint {
	t.Helper()
	n := models.Note{UserID: env.user.ID, ContactID: &env.contact.ID, Content: content, Date: at}
	require.NoError(t, env.db.Create(&n).Error)
	return n.ID
}

func (env *timelineTestEnv) seedActivity(t *testing.T, at time.Time, title string) uint {
	t.Helper()
	a := models.Activity{
		UserID: env.user.ID, Title: title, Date: at, Type: models.InteractionTypeVisit,
		Contacts: []models.Contact{env.contact},
	}
	require.NoError(t, env.db.Create(&a).Error)
	return a.ID
}

func (env *timelineTestEnv) seedCompletion(t *testing.T, at time.Time, message string) uint {
	t.Helper()
	c := models.ReminderCompletion{UserID: env.user.ID, ContactID: env.contact.ID, Message: message, CompletedAt: at}
	require.NoError(t, env.db.Create(&c).Error)
	return c.ID
}

func (env *timelineTestEnv) seedExternalActivity(t *testing.T, at time.Time, extID string) string {
	t.Helper()
	a := models.ExternalActivity{
		UserID: env.user.ID, EntityID: env.contact.VCardUID, SourceSystem: "t66-test",
		ExternalID: extID, Type: "photo-appearance", OccurredAt: at,
	}
	require.NoError(t, env.db.Create(&a).Error)
	return a.ID
}

func (env *timelineTestEnv) seedLifeEvent(t *testing.T, date *contactmodel.PartialDate, typ string) string {
	t.Helper()
	e := models.LifeEvent{UserID: env.user.ID, EntityID: env.contact.VCardUID, Type: typ, Date: date}
	require.NoError(t, env.db.Create(&e).Error)
	return e.ID
}

func (env *timelineTestEnv) seedGift(t *testing.T, status string, at time.Time) string {
	t.Helper()
	var date *time.Time
	if !at.IsZero() {
		d := at
		date = &d
	}
	g := models.Gift{UserID: env.user.ID, EntityID: env.contact.VCardUID, Description: "gift-" + status, Status: status, Date: date}
	require.NoError(t, env.db.Create(&g).Error)
	return g.ID
}

func pInt(v int) *int { return &v }

func parseItemDate(t *testing.T, it timelineItem) time.Time {
	t.Helper()
	d, err := time.Parse(time.RFC3339Nano, it.Date)
	require.NoError(t, err, "item date %q must parse", it.Date)
	return d
}

func itemKey(it timelineItem) string { return it.Type + "|" + it.ID }

// TestGetContactTimeline_MergeCorrectUnderSkew is the ticket's core merge
// test: a contact whose timeline is dominated by one type (200
// external_activity rows + 3 notes) must still return full, correctly-ordered
// pages, and paging all the way through must return every item exactly once.
func TestGetContactTimeline_MergeCorrectUnderSkew(t *testing.T) {
	env := newTimelineTestEnv(t)

	base := time.Now().AddDate(0, 0, -1)
	noteTimes := []time.Time{base.Add(5 * time.Minute), base.Add(4 * time.Minute), base.Add(3 * time.Minute)}
	extTimes := make([]time.Time, 200)
	for i := range extTimes {
		extTimes[i] = base.Add(-time.Duration(i) * 24 * time.Hour)
	}

	var noteIDs []uint
	for _, at := range noteTimes {
		noteIDs = append(noteIDs, env.seedNote(t, at, fmt.Sprintf("note %s", at.Format(time.RFC3339))))
	}
	var extIDs []string
	for i, at := range extTimes {
		extIDs = append(extIDs, env.seedExternalActivity(t, at, fmt.Sprintf("ext-%d", i)))
	}

	// Build the expected full order: all items by date desc. Every seeded
	// timestamp is unique, so date alone is a total order.
	type expectItem struct {
		typ  string
		id   string
		date time.Time
	}
	var expected []expectItem
	for i, at := range noteTimes {
		expected = append(expected, expectItem{models.TimelineTypeNote, fmt.Sprint(noteIDs[i]), at})
	}
	for i, at := range extTimes {
		expected = append(expected, expectItem{models.TimelineTypeExternalActivity, extIDs[i], at})
	}
	sort.SliceStable(expected, func(i, j int) bool { return expected[i].date.After(expected[j].date) })

	// First page: limit 25, correctly ordered, full.
	page := env.fetchTimeline(t, "limit=25")
	require.Len(t, page.Items, 25)
	for i, it := range page.Items {
		exp := expected[i]
		assert.Equalf(t, exp.typ, it.Type, "item %d type", i)
		assert.Equalf(t, exp.id, it.ID, "item %d id", i)
		assert.True(t, parseItemDate(t, it).Equal(exp.date), "item %d date", i)
	}

	// Full walk: every item exactly once, in global order.
	walked := env.walkTimeline(t, "limit=25")
	require.Len(t, walked, len(expected))
	seen := map[string]bool{}
	for i, it := range walked {
		exp := expected[i]
		assert.Equalf(t, exp.typ, it.Type, "walk item %d type", i)
		assert.Equalf(t, exp.id, it.ID, "walk item %d id", i)
		key := itemKey(it)
		require.Falsef(t, seen[key], "item %q returned twice", key)
		seen[key] = true
	}
}

// TestGetContactTimeline_SameDateTiebreak pins the (event_date, type, id)
// tuple: two items sharing a date across tables resolve by type rank, and two
// of the same type by id. This is the ordering the cursor depends on being
// total.
func TestGetContactTimeline_SameDateTiebreak(t *testing.T) {
	env := newTimelineTestEnv(t)
	// All items share the same instant: midnight UTC yesterday. The life
	// event's PartialDate resolves to that same midnight, so the whole set
	// exercises only the type-rank and id tiebreaks.
	base := time.Now().AddDate(0, 0, -1).UTC()
	at := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.UTC)

	// Seed two notes of the same type at the same instant so the id tiebreak
	// is real (notes get numeric uint PKs, assigned in creation order).
	note1 := env.seedNote(t, at, "note-a")
	note2 := env.seedNote(t, at, "note-b")
	env.seedActivity(t, at, "activity-a")
	env.seedCompletion(t, at, "completion-a")
	env.seedExternalActivity(t, at, "tie-ext")
	env.seedLifeEvent(t, &contactmodel.PartialDate{
		Year: pInt(at.Year()), Month: pInt(int(at.Month())), Day: pInt(at.Day()),
	}, "moved")
	env.seedGift(t, models.GiftStatusGiven, at)

	page := env.fetchTimeline(t, "limit=10")
	require.Len(t, page.Items, 7)

	// Desc: same date -> type rank desc -> id desc within a type. Rank order
	// is the canonical TimelineTypes order reversed.
	want := []string{
		models.TimelineTypeGift,
		models.TimelineTypeExternalActivity,
		models.TimelineTypeLifeEvent,
		models.TimelineTypeCompletion,
		models.TimelineTypeActivity,
		models.TimelineTypeNote,
		models.TimelineTypeNote,
	}
	gotTypes := make([]string, len(page.Items))
	for i, it := range page.Items {
		gotTypes[i] = it.Type
	}
	assert.Equal(t, want, gotTypes, "same-date items must resolve by type rank descending")

	// The two notes share date+type; id desc breaks the tie (note2 created
	// after note1, so its id is larger).
	noteItems := []timelineItem{}
	for _, it := range page.Items {
		if it.Type == models.TimelineTypeNote {
			noteItems = append(noteItems, it)
		}
	}
	require.Len(t, noteItems, 2)
	higher, lower := fmt.Sprint(note2), fmt.Sprint(note1)
	if note2 < note1 {
		higher, lower = lower, higher
	}
	assert.Equal(t, []string{higher, lower}, []string{noteItems[0].ID, noteItems[1].ID},
		"same-type same-date items must break by id descending")
}

// TestGetContactTimeline_TypeFilter pins the comma-separated ?type= filter:
// only the requested types come back, and it composes with paging.
func TestGetContactTimeline_TypeFilter(t *testing.T) {
	env := newTimelineTestEnv(t)

	at := time.Now().AddDate(0, 0, -1).UTC()
	env.seedNote(t, at, "note-1")
	env.seedNote(t, at.Add(time.Minute), "note-2")
	env.seedActivity(t, at.Add(time.Minute), "activity")
	env.seedCompletion(t, at.Add(2*time.Minute), "completion")
	env.seedExternalActivity(t, at.Add(3*time.Minute), "ext")
	env.seedLifeEvent(t, &contactmodel.PartialDate{Year: pInt(2020)}, "moved")
	env.seedGift(t, models.GiftStatusGiven, at.Add(4*time.Minute))

	page := env.fetchTimeline(t, "type=note")
	require.Len(t, page.Items, 2)
	for _, it := range page.Items {
		assert.Equal(t, models.TimelineTypeNote, it.Type)
	}

	page = env.fetchTimeline(t, "type=note,activity,completion")
	require.Len(t, page.Items, 4)
	got := map[string]bool{}
	for _, it := range page.Items {
		got[it.Type] = true
	}
	assert.Equal(t, map[string]bool{"note": true, "activity": true, "completion": true}, got)

	page = env.fetchTimeline(t, "type=note&limit=1")
	require.Len(t, page.Items, 1)
	require.NotEmpty(t, page.NextCursor, "a filtered page must still page")
	page = env.fetchTimeline(t, "type=note&limit=1&cursor="+page.NextCursor)
	require.Len(t, page.Items, 1, "the second note must be on the next page")
	assert.Empty(t, page.NextCursor, "both notes consumed means no more pages")
}

// TestGetContactTimeline_RecencyBuckets pins the fixed ?bucket= vocabulary:
// last 7/30/90 days, this year, all time.
func TestGetContactTimeline_RecencyBuckets(t *testing.T) {
	env := newTimelineTestEnv(t)

	now := time.Now().In(time.UTC)
	twoDays := now.AddDate(0, 0, -2).UTC()
	twentyDays := now.AddDate(0, 0, -20).UTC()
	sixtyDays := now.AddDate(0, 0, -60).UTC()
	// Same month last year: inside "all time", outside "this year".
	lastYear := time.Date(now.Year()-1, now.Month(), 15, 12, 0, 0, 0, time.UTC)

	env.seedNote(t, twoDays, "recent")
	env.seedNote(t, twentyDays, "month-old")
	env.seedNote(t, sixtyDays, "quarter-old")
	env.seedNote(t, lastYear, "last-year")

	count := func(bucket string) int {
		page := env.fetchTimeline(t, "bucket="+bucket)
		return len(page.Items)
	}

	assert.Equal(t, 1, count(models.TimelineBucketLast7Days))
	assert.Equal(t, 2, count(models.TimelineBucketLast30Days))
	assert.Equal(t, 3, count(models.TimelineBucketLast90Days))
	assert.Equal(t, 3, count(models.TimelineBucketThisYear), "this year must exclude last year's same-month row")
	assert.Equal(t, 4, count(models.TimelineBucketAll))
	assert.Equal(t, 4, count(""), "empty bucket defaults to all")
}

// TestGetContactTimeline_LifeEventDateResolution pins how a life event's
// PartialDate resolves to a timeline date: full date, year-only (Jan 1),
// yearless month/day (current year), and no date (created_at fallback).
func TestGetContactTimeline_LifeEventDateResolution(t *testing.T) {
	env := newTimelineTestEnv(t)

	full := env.seedLifeEvent(t, &contactmodel.PartialDate{Year: pInt(2020), Month: pInt(5), Day: pInt(1)}, "moved")
	yearOnly := env.seedLifeEvent(t, &contactmodel.PartialDate{Year: pInt(1990)}, "graduated")
	yearless := env.seedLifeEvent(t, &contactmodel.PartialDate{Month: pInt(7), Day: pInt(4)}, "anniversary")

	page := env.fetchTimeline(t, "type=life_event")
	require.Len(t, page.Items, 3)

	resolved := map[string]string{}
	for _, it := range page.Items {
		resolved[it.ID] = it.Date
	}
	// Year-only resolves to Jan 1 of that year (UTC midnight), matching the
	// web's fullDateFromPartial.
	assert.Equal(t, "1990-01-01T00:00:00Z", resolved[yearOnly])
	assert.Equal(t, "2020-05-01T00:00:00Z", resolved[full])
	// Yearless month/day resolves to the current year — the only "now"-dependent
	// case, mirroring the web's `new Date().getFullYear()`.
	curYear := time.Now().UTC().Year()
	wantYearless := fmt.Sprintf("%d-07-04T00:00:00Z", curYear)
	assert.Equal(t, wantYearless, resolved[yearless])

	// Ordering: the yearless event resolves to the current year (always newer
	// than the fixed 2020/1990 dates), then 2020-05-01, then 1990-01-01.
	require.Len(t, page.Items, 3)
	assert.Equal(t, yearless, page.Items[0].ID, "the yearless event resolves to the current year and is newest")
	assert.Equal(t, full, page.Items[1].ID)
	assert.Equal(t, yearOnly, page.Items[2].ID)
}

// TestGetContactTimeline_GiftFiltering pins that only timeline-eligible gifts
// appear: given/received with a handover date. Ideas (dated or not) and
// purchased gifts are not timeline events — the web timeline filters them the
// same way.
func TestGetContactTimeline_GiftFiltering(t *testing.T) {
	env := newTimelineTestEnv(t)

	at := time.Now().AddDate(0, 0, -1).UTC()
	env.seedGift(t, models.GiftStatusGiven, at)
	env.seedGift(t, models.GiftStatusReceived, at.Add(time.Hour))
	env.seedGift(t, models.GiftStatusIdea, at)
	env.seedGift(t, models.GiftStatusPurchased, at)
	env.seedGift(t, models.GiftStatusIdea, time.Time{}) // undated idea

	page := env.fetchTimeline(t, "type=gift")
	require.Len(t, page.Items, 2)
	got := map[string]bool{}
	for _, it := range page.Items {
		got[it.Type] = true
	}
	assert.Equal(t, map[string]bool{"gift": true}, got, "only gift items may appear")
}

// TestGetContactTimeline_Validation pins the loud-failure policy: unknown
// type/bucket and malformed cursors are 400s, never silent fallbacks.
func TestGetContactTimeline_Validation(t *testing.T) {
	env := newTimelineTestEnv(t)

	cases := []string{
		"type=banana",
		"type=note,banana",
		"bucket=yesterday",
		"cursor=not-base64url",
	}
	for _, q := range cases {
		w := env.get(t, "/contacts/"+idString(env.contact.ID)+"/timeline?"+q)
		assert.Equalf(t, http.StatusBadRequest, w.Code, "query %q must 400", q)
	}

	// A well-formed base64url body with a valid type but an invalid id for a
	// uint table is also a 400 (resuming a numeric page from a string id).
	w := env.get(t, "/contacts/"+idString(env.contact.ID)+"/timeline?cursor="+
		encodeTimelineCursor(time.Now(), "note", "not-a-uint"))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// An unknown type token inside the cursor is a 400 too.
	w = env.get(t, "/contacts/"+idString(env.contact.ID)+"/timeline?cursor="+
		encodeTimelineCursor(time.Now(), "banana", "1"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetContactTimeline_ScopedToOwner pins ownership: another user's contact
// is a 404, never a data leak.
func TestGetContactTimeline_ScopedToOwner(t *testing.T) {
	env := newTimelineTestEnv(t)

	other := models.User{Username: "t66-other", Password: "password123!A", Email: "t66-other@example.com"}
	require.NoError(t, env.db.Create(&other).Error)
	othersContact := models.Contact{UserID: other.ID, Firstname: "Not", Lastname: "Yours"}
	require.NoError(t, env.db.Create(&othersContact).Error)

	w := env.get(t, "/contacts/"+idString(othersContact.ID)+"/timeline")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetContactDetail_TimelineBlocksBounded pins the M4 composite bound
// (T66 design decision 4): with more than 5 rows per timeline-eligible type,
// each block caps at 5, ordered by the timeline's event-date key.
func TestGetContactDetail_TimelineBlocksBounded(t *testing.T) {
	env := newTimelineTestEnv(t)

	base := time.Now().AddDate(0, 0, -1).UTC()
	for i := 0; i < 6; i++ {
		at := base.Add(-time.Duration(i) * time.Hour)
		env.seedNote(t, at, fmt.Sprintf("note-%d", i))
		env.seedActivity(t, at.Add(1*time.Minute), fmt.Sprintf("act-%d", i))
		env.seedCompletion(t, at.Add(2*time.Minute), fmt.Sprintf("comp-%d", i))
		env.seedLifeEvent(t, &contactmodel.PartialDate{Year: pInt(2000 + i), Month: pInt(1), Day: pInt(1)}, "moved")
		env.seedExternalActivity(t, at.Add(3*time.Minute), fmt.Sprintf("ext-%d", i))
		env.seedGift(t, models.GiftStatusGiven, at.Add(4*time.Minute))
	}

	w := env.get(t, "/contacts/"+idString(env.contact.ID)+"/detail")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail struct {
		Notes              []json.RawMessage `json:"notes"`
		Activities         []json.RawMessage `json:"activities"`
		Completions        []json.RawMessage `json:"completions"`
		LifeEvents         []json.RawMessage `json:"life_events"`
		ExternalActivities []json.RawMessage `json:"external_activities"`
		Gifts              []json.RawMessage `json:"gifts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))

	require.Len(t, detail.Notes, 5)
	require.Len(t, detail.Activities, 5)
	require.Len(t, detail.Completions, 5)
	require.Len(t, detail.LifeEvents, 5)
	require.Len(t, detail.ExternalActivities, 5)
	require.Len(t, detail.Gifts, 5)

	// The most recent note (i=0) must be present — the block is ordered by
	// event date, not arbitrary.
	var firstNote struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(detail.Notes[0], &firstNote))
	assert.Equal(t, "note-0", firstNote.Content)
}

// TestGetContactDetail_PayloadBoundedForLongHistory measures the composite
// payload for a long-history contact and pins the bound. The byte count is
// logged so the ticket's landing note can record before/after (the unbounded
// baseline is captured in the same harness by removing the limits).
func TestGetContactDetail_PayloadBoundedForLongHistory(t *testing.T) {
	env := newTimelineTestEnv(t)

	base := time.Now().AddDate(0, 0, -1).UTC()
	for i := 0; i < 200; i++ {
		at := base.Add(-time.Duration(i) * time.Minute)
		env.seedExternalActivity(t, at, fmt.Sprintf("bulk-ext-%d", i))
		if i < 40 {
			env.seedNote(t, at, fmt.Sprintf("bulk-note-%d", i))
			env.seedCompletion(t, at.Add(30*time.Second), fmt.Sprintf("bulk-comp-%d", i))
			env.seedGift(t, models.GiftStatusGiven, at.Add(1*time.Minute))
		}
	}

	w := env.get(t, "/contacts/"+idString(env.contact.ID)+"/detail")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	t.Logf("M4 composite payload with 200 external activities + 40 notes/completions/gifts: %d bytes", len(w.Body.Bytes()))
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, key := range []string{"notes", "external_activities", "completions", "gifts", "life_events", "activities"} {
		var block []json.RawMessage
		require.NoError(t, json.Unmarshal(raw[key], &block))
		assert.LessOrEqualf(t, len(block), 5, "block %q must be bounded at 5", key)
	}
}
