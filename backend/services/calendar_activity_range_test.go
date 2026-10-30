package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2006: a multi-day remote event becomes ONE activity with a range
// (Date .. EndDate), never one activity per day. iCalendar all-day DTEND is
// exclusive, so the inclusive end day is DTEND - 1 day; a timed event's end
// day is the UTC day of the last instant it covers (DTEND - 1ns).

func parseEvents(t *testing.T, vevents string) []calendarEvent {
	t.Helper()
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\n" + strings.ReplaceAll(vevents, "\n", "\r\n") + "\r\nEND:VCALENDAR\r\n"
	cal, err := ical.NewDecoder(strings.NewReader(raw)).Decode()
	require.NoError(t, err)
	return extractEvents(cal, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestExtractEventsAllDayMultiDayDTENDIsExclusive(t *testing.T) {
	events := parseEvents(t, "BEGIN:VEVENT\nUID:trip\nSUMMARY:Trip\nDTSTART;VALUE=DATE:20260512\nDTEND;VALUE=DATE:20260516\nEND:VEVENT")
	require.Len(t, events, 1, "one event, not one per day")
	assert.Equal(t, day(2026, 5, 12), events[0].Date)
	require.NotNil(t, events[0].EndDate)
	assert.Equal(t, day(2026, 5, 15), *events[0].EndDate, "DTEND 16 May (exclusive) is an inclusive 15 May end")
}

func TestExtractEventsSingleDayAndZeroLengthHaveNoEnd(t *testing.T) {
	cases := map[string]string{
		"all-day with exclusive next-day DTEND":  "DTSTART;VALUE=DATE:20260512\nDTEND;VALUE=DATE:20260513",
		"all-day without DTEND":                  "DTSTART;VALUE=DATE:20260512",
		"timed, same day":                        "DTSTART:20260512T100000Z\nDTEND:20260512T110000Z",
		"timed zero-length (DTEND == DTSTART)":   "DTSTART:20260512T100000Z\nDTEND:20260512T100000Z",
		"all-day zero-length (DTEND == DTSTART)": "DTSTART;VALUE=DATE:20260512\nDTEND;VALUE=DATE:20260512",
		"timed ending exactly at next midnight":  "DTSTART:20260512T220000Z\nDTEND:20260513T000000Z",
	}
	for name, props := range cases {
		t.Run(name, func(t *testing.T) {
			events := parseEvents(t, "BEGIN:VEVENT\nUID:e\nSUMMARY:E\n"+props+"\nEND:VEVENT")
			require.Len(t, events, 1)
			assert.Nil(t, events[0].EndDate)
		})
	}
}

func TestExtractEventsTimedMultiDayAndDuration(t *testing.T) {
	events := parseEvents(t, "BEGIN:VEVENT\nUID:conf\nSUMMARY:Conf\nDTSTART:20260512T180000Z\nDTEND:20260514T090000Z\nEND:VEVENT\n"+
		"BEGIN:VEVENT\nUID:dur\nSUMMARY:Dur\nDTSTART;VALUE=DATE:20260512\nDURATION:P3D\nEND:VEVENT")
	require.Len(t, events, 2)
	require.NotNil(t, events[0].EndDate)
	assert.Equal(t, day(2026, 5, 14), *events[0].EndDate)
	require.NotNil(t, events[1].EndDate)
	assert.Equal(t, day(2026, 5, 14), *events[1].EndDate, "P3D from 12 May exclusive-ends 15 May, so the inclusive end is 14 May")
}

// DST boundaries: timed events with a TZID are compared as UTC calendar days,
// the same reading the start date already gets (Date is stored UTC). A span
// across the US fall-back (1 Nov 2026) is 49 hours but still lands on the
// right UTC days, and one across spring-forward stays inside one UTC day.
func TestExtractEventsTimedAcrossDSTBoundaries(t *testing.T) {
	t.Run("fall back", func(t *testing.T) {
		events := parseEvents(t, "BEGIN:VEVENT\nUID:fb\nSUMMARY:Fall back\nDTSTART;TZID=America/Chicago:20261031T180000\nDTEND;TZID=America/Chicago:20261102T090000\nEND:VEVENT")
		require.Len(t, events, 1)
		assert.Equal(t, time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), events[0].Date)
		require.NotNil(t, events[0].EndDate)
		assert.Equal(t, day(2026, 11, 2), *events[0].EndDate)
	})
	t.Run("fall back all-day is unaffected", func(t *testing.T) {
		events := parseEvents(t, "BEGIN:VEVENT\nUID:fb2\nSUMMARY:Fall back\nDTSTART;VALUE=DATE:20261031\nDTEND;VALUE=DATE:20261103\nEND:VEVENT")
		require.Len(t, events, 1)
		assert.Equal(t, day(2026, 10, 31), events[0].Date)
		require.NotNil(t, events[0].EndDate)
		assert.Equal(t, day(2026, 11, 2), *events[0].EndDate)
	})
	t.Run("spring forward within one UTC day", func(t *testing.T) {
		events := parseEvents(t, "BEGIN:VEVENT\nUID:sf\nSUMMARY:Spring forward\nDTSTART;TZID=America/Chicago:20260307T230000\nDTEND;TZID=America/Chicago:20260308T040000\nEND:VEVENT")
		require.Len(t, events, 1)
		assert.Nil(t, events[0].EndDate)
	})
	t.Run("spring forward all-day range", func(t *testing.T) {
		events := parseEvents(t, "BEGIN:VEVENT\nUID:sf2\nSUMMARY:Spring forward\nDTSTART;VALUE=DATE:20260307\nDTEND;VALUE=DATE:20260310\nEND:VEVENT")
		require.Len(t, events, 1)
		require.NotNil(t, events[0].EndDate)
		assert.Equal(t, day(2026, 3, 9), *events[0].EndDate)
	})
}

func TestExtractEventsRecurringRangeShiftsEachOccurrence(t *testing.T) {
	events := parseEvents(t, "BEGIN:VEVENT\nUID:wk\nSUMMARY:Retreat\nDTSTART;VALUE=DATE:20260105\nDTEND;VALUE=DATE:20260108\nRRULE:FREQ=WEEKLY;COUNT=3\nEND:VEVENT")
	require.Len(t, events, 3)
	for i, ev := range events {
		start := day(2026, 1, 5).AddDate(0, 0, 7*i)
		assert.Equal(t, start, ev.Date.UTC())
		require.NotNil(t, ev.EndDate, "occurrence %d", i)
		assert.Equal(t, start.AddDate(0, 0, 2), *ev.EndDate, "every occurrence keeps its 3-day span")
	}
}

func TestCalendarEventContentHashCoversEndDateButNotForSingleDay(t *testing.T) {
	single := calendarEvent{Title: "T", Description: "D", Location: "L", Date: day(2026, 5, 12)}
	// A single-day event must hash exactly as before this feature, otherwise
	// every existing calendar link would look changed on the next sync.
	legacy := sha256.Sum256([]byte(strings.Join([]string{"T", "D", "L", "2026-05-12T00:00:00Z"}, "\x1f")))
	assert.Equal(t, hex.EncodeToString(legacy[:]), single.contentHash())

	end := day(2026, 5, 15)
	ranged := single
	ranged.EndDate = &end
	assert.NotEqual(t, single.contentHash(), ranged.contentHash())
	end2 := day(2026, 5, 16)
	ranged2 := single
	ranged2.EndDate = &end2
	assert.NotEqual(t, ranged.contentHash(), ranged2.contentHash())
}

func TestActivityToICalendarEmitsAllDayRangeWithExclusiveDTEND(t *testing.T) {
	end := day(2026, 5, 15)
	a := &models.Activity{Title: "Trip", Date: day(2026, 5, 12), EndDate: &end}
	out := encodeCal(t, activityToICalendar("uid-1", a))
	assert.Contains(t, out, "DTSTART;VALUE=DATE:20260512")
	assert.Contains(t, out, "DTEND;VALUE=DATE:20260516", "inclusive 15 May is an exclusive 16 May DTEND")

	// Zero-length range (end == start) is a single all-day with next-day DTEND.
	same := day(2026, 5, 12)
	z := &models.Activity{Title: "Day", Date: day(2026, 5, 12), EndDate: &same}
	out = encodeCal(t, activityToICalendar("uid-2", z))
	assert.Contains(t, out, "DTSTART;VALUE=DATE:20260512")
	assert.Contains(t, out, "DTEND;VALUE=DATE:20260513")

	// Month and year rollover of the exclusive end.
	eoy := day(2026, 12, 31)
	r := &models.Activity{Title: "NYE", Date: day(2026, 12, 30), EndDate: &eoy}
	out = encodeCal(t, activityToICalendar("uid-3", r))
	assert.Contains(t, out, "DTEND;VALUE=DATE:20270101")

	// A single-day activity is unchanged: DATE-TIME start, no DTEND.
	s := &models.Activity{Title: "Coffee", Date: time.Date(2026, 5, 12, 14, 30, 0, 0, time.UTC)}
	out = encodeCal(t, activityToICalendar("uid-4", s))
	assert.Contains(t, out, "DTSTART:20260512T143000Z")
	assert.NotContains(t, out, "DTEND")
}

func TestActivityRangeRoundTripsThroughICalendarAndHash(t *testing.T) {
	end := day(2026, 5, 15)
	a := &models.Activity{Title: "Trip", Date: time.Date(2026, 5, 12, 14, 30, 0, 0, time.UTC), EndDate: &end}
	cal := activityToICalendar("uid-1", a)
	events := extractEvents(cal, day(2000, 1, 1), day(2100, 1, 1))
	require.Len(t, events, 1)
	require.NotNil(t, events[0].EndDate)
	assert.Equal(t, end, *events[0].EndDate)
	assert.Equal(t, day(2026, 5, 12), events[0].Date)
	// What we push must hash like what we then pull back, or the next sync
	// would see a phantom remote edit.
	assert.Equal(t, activityContentHash(a), events[0].contentHash())
}

func encodeCal(t *testing.T, cal *ical.Calendar) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, ical.NewEncoder(&buf).Encode(cal))
	return buf.String()
}

// End to end through SyncSubscription: one ranged activity, idempotent
// re-sync, an extended range updates it, and collapsing it to one day clears
// the end date.
func TestCalendarSyncImportsMultiDayEventAsOneRangedActivity(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := calendarTestConfig()
	user := createCalendarTestUser(t, db)

	startDay := time.Now().UTC().AddDate(0, 0, 3)
	fmtDay := func(t time.Time) string { return t.Format("20060102") }
	var mu sync.Mutex
	dtend := fmtDay(startDay.AddDate(0, 0, 4)) // exclusive: covers start..start+3

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "text/calendar")
		fmt.Fprint(w, "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n"+
			"BEGIN:VEVENT\nUID:trip\nSUMMARY:Family trip\nDTSTART;VALUE=DATE:"+fmtDay(startDay)+"\nDTEND;VALUE=DATE:"+dtend+"\nEND:VEVENT\n"+
			"END:VCALENDAR\n")
	}))
	defer server.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, server.URL+"/cal.ics", "", "")
	service := NewCalendarSyncService(false)

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Created: 1}, stats, "a four-day event is one activity")

	var acts []models.Activity
	require.NoError(t, db.Find(&acts).Error)
	require.Len(t, acts, 1)
	require.NotNil(t, acts[0].EndDate)
	assert.Equal(t, day(startDay.Year(), startDay.Month(), startDay.Day()).AddDate(0, 0, 3), acts[0].EndDate.UTC())

	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Skipped: 1}, stats)

	mu.Lock()
	dtend = fmtDay(startDay.AddDate(0, 0, 6))
	mu.Unlock()
	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Updated: 1}, stats)
	var extended models.Activity
	require.NoError(t, db.First(&extended, acts[0].ID).Error)
	assert.Equal(t, day(startDay.Year(), startDay.Month(), startDay.Day()).AddDate(0, 0, 5), extended.EndDate.UTC())

	mu.Lock()
	dtend = fmtDay(startDay.AddDate(0, 0, 1))
	mu.Unlock()
	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Updated: 1}, stats)
	var collapsed models.Activity
	require.NoError(t, db.First(&collapsed, acts[0].ID).Error)
	assert.Nil(t, collapsed.EndDate, "collapsing to a single day clears the end")
}
