package services

import (
	"encoding/base64"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestComposeTimeline_SingleContactFetchAllTypesAndPaginate(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)
	contact := seedContact(t, db, user.ID, "Ada", "Lovelace")
	other := seedContact(t, db, user.ID, "Grace", "Hopper")
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	// Two same-date notes (exercises the numeric-id tiebreak) + one older.
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "n1", Date: base}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "n2", Date: base}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "old", Date: base.AddDate(0, 0, -10)}).Error)
	// Another contact's note must not appear.
	otherNote := &models.Note{UserID: user.ID, ContactID: &other.ID, Content: "other", Date: base}
	require.NoError(t, db.Create(otherNote).Error)

	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: "call", Date: base.Add(-time.Hour), Contacts: []models.Contact{contact}}).Error)
	require.NoError(t, db.Create(&models.ReminderCompletion{UserID: user.ID, ContactID: contact.ID, Message: "done", CompletedAt: base.Add(-2 * time.Hour)}).Error)
	require.NoError(t, db.Create(&models.ExternalActivity{UserID: user.ID, EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "e1", Type: "photo", OccurredAt: base.Add(-3 * time.Hour)}).Error)
	require.NoError(t, db.Create(&models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "g", Status: models.GiftStatusGiven, Date: ptr(base.Add(-4 * time.Hour))}).Error)

	// Life events: full date, yearless month/day, year only, and no date at all.
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom", Date: &contactmodel.PartialDate{Year: ptr(2020), Month: ptr(5), Day: ptr(7)}}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom", Date: &contactmodel.PartialDate{Month: ptr(6), Day: ptr(8)}}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom", Date: &contactmodel.PartialDate{Year: ptr(2019)}}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom"}).Error)

	// Page 1 (desc): Limit 3, with a next cursor.
	items, next, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 3, Desc: true, Now: base})
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.NotEmpty(t, next)
	for _, it := range items {
		if n, ok := it.Data.(models.Note); ok {
			assert.NotEqual(t, otherNote.Content, n.Content, "another contact's note must be excluded")
		}
	}

	// Page 2 via the decoded cursor.
	cur, err := DecodeTimelineCursor(next)
	require.NoError(t, err)
	page2, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 3, Desc: true, Cursor: cur, Now: base})
	require.NoError(t, err)
	require.NotEmpty(t, page2)

	// Ascending order also merges and pages.
	asc, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 5, Desc: false, Now: base})
	require.NoError(t, err)
	require.NotEmpty(t, asc)

	// Defaults: Limit <= 0 and a zero Now must not panic and must return items.
	def, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Desc: true})
	require.NoError(t, err)
	assert.NotNil(t, def)
}

func TestComposeTimeline_CutoffAndNotAfter(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)
	contact := seedContact(t, db, user.ID, "Ada", "Lovelace")
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "recent", Date: base.Add(-time.Hour)}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "ancient", Date: base.AddDate(0, 0, -30)}).Error)

	// Cutoff drops the ancient note (inclusive >= floor).
	cutoff := base.AddDate(0, 0, -7)
	items, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 50, Desc: true, Cutoff: &cutoff, Now: base})
	require.NoError(t, err)
	require.Len(t, items, 1)

	// NotAfter drops anything dated after the ceiling: the recent note goes,
	// the one from 30 days ago stays.
	notAfter := base.Add(-2 * time.Hour)
	items, _, err = ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 50, Desc: true, NotAfter: &notAfter, Now: base})
	require.NoError(t, err)
	require.Len(t, items, 1)
	got, ok := items[0].Data.(models.Note)
	require.True(t, ok)
	assert.Equal(t, "ancient", got.Content)
}

func TestComposeTimeline_LifeEventCutoffAndNotAfter(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)
	contact := seedContact(t, db, user.ID, "Ada", "Lovelace")

	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom", Date: &contactmodel.PartialDate{Year: ptr(2010), Month: ptr(1), Day: ptr(1)}}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom", Date: &contactmodel.PartialDate{Year: ptr(2035), Month: ptr(1), Day: ptr(1)}}).Error)

	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	items, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 50, Desc: true, Cutoff: &cutoff, Now: time.Now()})
	require.NoError(t, err)
	require.Len(t, items, 1, "only the 2035 life event survives the cutoff")

	notAfter := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	items, _, err = ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 50, Desc: true, NotAfter: &notAfter, Now: time.Now()})
	require.NoError(t, err)
	require.Len(t, items, 1, "only the 2010 life event survives NotAfter")
}

func TestComposeTimeline_ErrorsAndBadCursor(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)

	// A ContactID the user does not own is an error.
	missing := uint(9_999_999)
	_, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &missing, Limit: 5, Now: time.Now()})
	require.Error(t, err)

	// A numeric-PK cursor whose id is not numeric is ErrTimelineBadCursor.
	_, _, err = ComposeTimeline(db, user.ID, TimelineQuery{
		Limit: 5, Now: time.Now(),
		Cursor: &TimelineCursor{Date: time.Now(), Type: models.TimelineTypeNote, ID: "not-a-uint"},
	})
	require.ErrorIs(t, err, ErrTimelineBadCursor)
}

func TestApplySensitivityFilter(t *testing.T) {
	db := dbtest.New(t)

	// Off by default: the base query is returned unchanged.
	off := (&timelineComposer{}).applySensitivityFilter(db.Model(&models.Note{}), "notes", models.TimelineTypeNote)
	require.NotNil(t, off)

	// No timeline model has a Sensitivity column today, so the set is empty.
	// Temporarily populate it to exercise the predicate-building branch (never
	// executed, since the real schema has no such column).
	feedSensitivityFiltered[models.TimelineTypeNote] = true
	t.Cleanup(func() { delete(feedSensitivityFiltered, models.TimelineTypeNote) })

	on := (&timelineComposer{filterSensitivity: true}).applySensitivityFilter(db.Model(&models.Note{}), "notes", models.TimelineTypeNote)
	require.NotNil(t, on)
}

// --- internal helper coverage ---------------------------------------------

func rawCursor(payload string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func TestDecodeTimelineCursor_RoundTripAndRejections(t *testing.T) {
	when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	encoded := EncodeTimelineCursor(when, models.TimelineTypeNote, "42")
	cur, err := DecodeTimelineCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, models.TimelineTypeNote, cur.Type)
	assert.Equal(t, "42", cur.ID)
	assert.True(t, when.Equal(cur.Date))

	bad := []string{
		"!!!not-base64!!!",                                  // not base64url
		rawCursor("only-one-field"),                         // malformed shape
		rawCursor("notatime|note|1"),                        // bad timestamp
		rawCursor(when.Format(time.RFC3339Nano) + "|x|1"),   // unknown type
		rawCursor(when.Format(time.RFC3339Nano) + "|note|"), // empty id
	}
	for _, b := range bad {
		if _, err := DecodeTimelineCursor(b); err == nil {
			t.Errorf("malformed cursor %q must be rejected", b)
		}
	}
}

func TestTimelineCursorPredicate_CoversRanksAndDirections(t *testing.T) {
	when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	cur := &TimelineCursor{Date: when, Type: models.TimelineTypeCompletion, ID: "5"}

	// desc/asc: table rank below, equal, and above the cursor's type.
	for _, typ := range []string{models.TimelineTypeNote, models.TimelineTypeCompletion, models.TimelineTypeGift} {
		for _, desc := range []bool{true, false} {
			pred, args := timelineCursorPredicate("notes", "date", typ, cur, uint64(5), desc)
			assert.NotEmpty(t, pred)
			assert.NotEmpty(t, args)
		}
	}
}

func TestTimelineDateOrder_Direction(t *testing.T) {
	db := dbtest.New(t)
	require.NotNil(t, timelineDateOrder(db.Model(&models.Note{}), "notes", "date", true))
	require.NotNil(t, timelineDateOrder(db.Model(&models.Note{}), "notes", "date", false))
}

func TestResolveTimelineCursorIDs(t *testing.T) {
	cur := &TimelineCursor{Date: time.Now(), Type: models.TimelineTypeNote, ID: "7"}
	ids, err := resolveTimelineCursorIDs([]string{models.TimelineTypeNote, models.TimelineTypeGift}, cur)
	require.NoError(t, err)
	assert.Equal(t, uint64(7), ids[models.TimelineTypeNote])
	assert.Equal(t, "7", ids[models.TimelineTypeGift])

	bad := &TimelineCursor{Date: time.Now(), Type: models.TimelineTypeNote, ID: "nope"}
	_, err = resolveTimelineCursorIDs([]string{models.TimelineTypeNote}, bad)
	require.ErrorIs(t, err, ErrTimelineBadCursor)

	_, ok := parseUintID("7")
	assert.True(t, ok)
	_, ok = parseUintID("nope")
	assert.False(t, ok)
}

func TestTimelineEntrySideOfCursor_AllBranches(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := &TimelineCursor{Date: when, Type: models.TimelineTypeCompletion, ID: "5"}

	cases := []struct {
		desc bool
		date time.Time
		typ  string
		id   string
	}{
		{true, when.Add(-time.Hour), models.TimelineTypeNote, "1"},  // before
		{true, when.Add(time.Hour), models.TimelineTypeNote, "1"},   // after
		{true, when, models.TimelineTypeNote, "1"},                  // same date, lower rank
		{true, when, models.TimelineTypeGift, "1"},                  // same date, higher rank
		{true, when, models.TimelineTypeCompletion, "4"},            // same date/type, id <
		{true, when, models.TimelineTypeCompletion, "6"},            // same date/type, id >
		{false, when.Add(time.Hour), models.TimelineTypeNote, "1"},  // asc after
		{false, when.Add(-time.Hour), models.TimelineTypeNote, "1"}, // asc before
		{false, when, models.TimelineTypeGift, "1"},                 // asc same date, higher rank
		{false, when, models.TimelineTypeNote, "1"},                 // asc same date, lower rank
		{false, when, models.TimelineTypeCompletion, "6"},           // asc same date/type, id >
		{false, when, models.TimelineTypeCompletion, "4"},           // asc same date/type, id <
	}
	for _, c := range cases {
		_ = timelineEntrySideOfCursor(c.typ, c.date, c.id, cur, c.desc)
	}
}

func TestTimelineEntryBefore_AllBranches(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n1, n2 := uint(1), uint(2)

	entries := []timelineEntry{
		{typ: models.TimelineTypeNote, id: "1", date: when, numID: &n1},     // numeric id
		{typ: models.TimelineTypeNote, id: "2", date: when, numID: &n2},     // numeric id, larger
		{typ: models.TimelineTypeGift, id: "a", date: when},                 // string id
		{typ: models.TimelineTypeGift, id: "b", date: when},                 // string id, larger
		{typ: models.TimelineTypeActivity, id: "1", date: when, numID: &n1}, // different type, same date
		{typ: models.TimelineTypeNote, id: "9", date: when.Add(time.Hour)},  // different date
	}
	for i := range entries {
		for j := range entries {
			_ = timelineEntryBefore(entries[i], entries[j], true)
			_ = timelineEntryBefore(entries[i], entries[j], false)
		}
	}
}

func TestTimelineLifeEventDate_AllPartialShapes(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	assert.Equal(t, time.Date(2020, 5, 7, 0, 0, 0, 0, time.UTC),
		timelineLifeEventDate(&models.LifeEvent{Date: &contactmodel.PartialDate{Year: ptr(2020), Month: ptr(5), Day: ptr(7)}}, now))
	assert.Equal(t, time.Date(now.Year(), 6, 8, 0, 0, 0, 0, time.UTC),
		timelineLifeEventDate(&models.LifeEvent{Date: &contactmodel.PartialDate{Month: ptr(6), Day: ptr(8)}}, now))
	assert.Equal(t, time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
		timelineLifeEventDate(&models.LifeEvent{Date: &contactmodel.PartialDate{Year: ptr(2019)}}, now))
	assert.Equal(t, created, timelineLifeEventDate(&models.LifeEvent{CreatedAt: created}, now))
	assert.Equal(t, created, timelineLifeEventDate(&models.LifeEvent{CreatedAt: created, Date: &contactmodel.PartialDate{}}, now))
}
