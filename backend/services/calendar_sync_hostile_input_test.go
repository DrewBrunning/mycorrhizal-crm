package services

// Issue #512: end-to-end hostile-input coverage for the live CalDAV
// reconcile path -- SyncSubscription -> fetchICS/queryCalDAV -> importEvents
// -- against a real database.InitDB-migrated schema and a real httptest
// CalDAV server. The unit-level fuzz coverage already exists
// (calendar_ical_fuzz_test.go's FuzzExtractICalEvents, issue #376) and found
// a real decoder panic; decodeCalendarSafely's panic recovery and
// extractEvents' length-clamping (clampRunes) are the fixes that landed from
// it. What was missing was proof those defenses are actually wired into the
// live SyncSubscription path end-to-end, and that hostile *content* (not
// just malformed *syntax*) survives the same "stored verbatim, not stripped"
// bar issue #375/#416 established for contacts.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCalendarSync_HostileVEVENT_OversizedAndHTMLClampedNotCrashed drives one
// VCALENDAR with three adversarial VEVENTs through the full SyncSubscription
// pipeline in a single sync:
//   - a SUMMARY far longer than any legitimate event title, containing an
//     HTML/script payload -- must survive clamped to 200 runes (clampRunes)
//     and with the payload stored literally, not stripped (matches the
//     project's established "frontend renders free text verbatim, so
//     stripping here would only destroy data" position);
//   - a malformed RRULE that cannot be parsed as a recurrence rule -- must
//     degrade to importing the event's first occurrence rather than failing
//     the whole sync (extractEvents' existing fallback);
//   - an event with no usable DTSTART at all -- must be skipped, not crash
//     the sync for the other two events in the same feed.
func TestCalendarSync_HostileVEVENT_OversizedAndHTMLClampedNotCrashed(t *testing.T) {
	db := dbtest.New(t)
	cfg := calendarTestConfig()
	user := createCalendarTestUser(t, db)

	// The payload leads, padding trails: clampRunes (calendar_sync_service.go)
	// keeps the first N runes and truncates the tail, so this is what proves
	// the payload itself -- not just some surviving prefix of it -- makes it
	// through clamping intact.
	hostileSummary := "<script>alert(1)</script>" + strings.Repeat("A", 5000)
	hostileDescription := "<img src=x onerror=alert(1)>" + strings.Repeat("B", 10000)
	hostileLocation := strings.Repeat("C", 2000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "REPORT" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
			// Well-formed but hostile-content event: still inside the window.
			"BEGIN:VEVENT\r\nUID:hostile-content\r\nSUMMARY:" + hostileSummary + "\r\nDESCRIPTION:" + hostileDescription +
			"\r\nLOCATION:" + hostileLocation + "\r\nDTSTART:" + icalDate(1) + "\r\nEND:VEVENT\r\n" +
			// Malformed RRULE: must degrade to first-occurrence import, not fail.
			"BEGIN:VEVENT\r\nUID:bad-rrule\r\nSUMMARY:Bad recurrence\r\nDTSTART:" + icalDate(2) +
			"\r\nRRULE:FREQ=BOGUS;COUNT=notanumber\r\nEND:VEVENT\r\n" +
			// No usable DTSTART at all: must be skipped, not crash the sync.
			"BEGIN:VEVENT\r\nUID:no-dtstart\r\nSUMMARY:Missing start\r\nEND:VEVENT\r\n" +
			"END:VCALENDAR\r\n"))
	}))
	defer server.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, server.URL+"/cal.ics", "", "")
	stats, err := NewCalendarSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "hostile event content/syntax must be contained, not fail the whole sync")
	assert.Equal(t, CalendarSyncStats{Created: 2}, stats, "the two events with a usable DTSTART are imported; the one without is silently skipped")

	var activities []models.Activity
	require.NoError(t, db.Order("id").Find(&activities).Error)
	require.Len(t, activities, 2)

	var hostile, badRRule models.Activity
	for _, a := range activities {
		switch {
		case strings.HasPrefix(a.Title, "<script>"):
			hostile = a
		case a.Title == "Bad recurrence":
			badRRule = a
		}
	}

	require.NotEmpty(t, hostile.Title, "the hostile-content event must have been imported")
	assert.LessOrEqual(t, len([]rune(hostile.Title)), 200, "Title must be clamped to clampRunes' 200-rune limit")
	assert.True(t, strings.HasPrefix(hostile.Title, "<script>alert(1)</script>"), "the HTML/script payload must survive clamping intact, stored verbatim not stripped")
	assert.LessOrEqual(t, len([]rune(hostile.Description)), 2000, "Description must be clamped to clampRunes' 2000-rune limit")
	assert.True(t, strings.HasPrefix(hostile.Description, "<img src=x onerror=alert(1)>"), "HTML in Description must be stored verbatim, not stripped -- matches the established contacts bar (#375/#416)")
	assert.LessOrEqual(t, len([]rune(hostile.Location)), 300, "Location must be clamped to clampRunes' 300-rune limit")

	require.NotEmpty(t, badRRule.Title, "an event with an unparseable RRULE must still import its first occurrence rather than being dropped")
}

// TestSyncSubscription_OversizedCalendarResponse_RejectedNotSilentlyAccepted
// pins that maxCalendarResponseBytes (calendar_sync_service.go) is actually
// wired into the live HTTP path -- the CalDAV-side counterpart of
// TestSyncSubscription_OversizedResponse_RejectedNotSilentlyAccepted in
// contact_sync_hostile_input_test.go.
func TestSyncSubscription_OversizedCalendarResponse_RejectedNotSilentlyAccepted(t *testing.T) {
	huge := strings.Repeat("A", 21<<20)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:huge\r\nSUMMARY:Huge\r\nDESCRIPTION:" + huge + "\r\nDTSTART:" + icalDate(1) +
			"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	}))
	defer server.Close()

	db := dbtest.New(t)
	cfg := calendarTestConfig()
	user := createCalendarTestUser(t, db)
	sub := newTestSubscription(t, db, cfg, user.ID, server.URL+"/cal.ics", "", "")

	_, err := NewCalendarSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, err, "an oversized calendar response must not be silently accepted")

	var count int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count, "no activity must be created from a response the size guard refused")

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.CalendarSyncStatusError, sub.LastSyncStatus)
}
