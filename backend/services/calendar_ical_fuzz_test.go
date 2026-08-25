package services

import (
	"bytes"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// calendarICALFuzzSeeds are self-contained ICS bodies covering the shapes
// calendar_sync_service_test.go already exercises against a real
// httptest.Server: a plain event, an all-day (VALUE=DATE) event, a
// STATUS:CANCELLED event, a weekly RRULE with EXDATE, and a RECURRENCE-ID
// override. Literal fixed dates rather than the test file's icalDate(n)
// helper (which is relative to time.Now()) — a fuzz seed just needs to be a
// valid-ish starting point, not a fresh one each run.
var calendarICALFuzzSeeds = []string{
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:event-1\n" +
		"SUMMARY:Quarterly catch-up\nDESCRIPTION:Notes\nLOCATION:Cafe Central\n" +
		"ATTENDEE:mailto:ada@example.com\nDTSTART:20260301T100000Z\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:event-2\n" +
		"SUMMARY:All-day planning\nDTSTART;VALUE=DATE:20260305\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n" +
		"BEGIN:VEVENT\nUID:cancelled\nSUMMARY:Cancelled\nSTATUS:CANCELLED\nDTSTART:20260302T090000Z\nEND:VEVENT\n" +
		"BEGIN:VEVENT\nUID:keeper\nRECURRENCE-ID:20260303T090000Z\nSUMMARY:Override instance\n" +
		"DTSTART:20260303T110000Z\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:weekly\n" +
		"SUMMARY:Weekly sync\nDTSTART:20260101T090000Z\nRRULE:FREQ=WEEKLY\nEXDATE:20260115T090000Z\n" +
		"END:VEVENT\nEND:VCALENDAR\n",
	// Multiple VCALENDAR blocks concatenated: fetchICS decodes in a loop
	// until io.EOF, tolerating trailing garbage once at least one calendar
	// has decoded — seed that shape too.
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:a\n" +
		"SUMMARY:First\nDTSTART:20260101T090000Z\nEND:VEVENT\nEND:VCALENDAR\n" +
		"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:b\n" +
		"SUMMARY:Second\nDTSTART:20260102T090000Z\nEND:VEVENT\nEND:VCALENDAR\n",
}

// FuzzExtractICalEvents covers issue #376's CalDAV target: the actual
// untrusted-input boundary isn't caldav/backend.go (that serves read-only —
// PutCalendarObject/DeleteCalendarObject are intentionally unimplemented,
// see backend/caldav/backend.go). It's CalendarSyncService.fetchICS
// (calendar_sync_service.go:389-430), which decodes raw ICS bytes fetched
// from a *subscribed remote calendar* over HTTP via github.com/emersion/go-ical
// and feeds the result into extractEvents. This fuzzes exactly that pipeline
// via decodeCalendarSafely — fetchICS's own per-calendar decode+extract
// step — rather than reimplementing the loop, so the harness exercises the
// same panic-recovery boundary production traffic does.
//
// This target found a real crash on first run: go-ical's decoder itself
// panics (index-out-of-range in lineDecoder.peek) on malformed input like
// "0;0=" — see decodeCalendarSafely's doc comment for why that mattered
// enough to fix rather than just document. go test -fuzz catches panics as
// failures automatically; decodeCalendarSafely turning that panic into a
// plain error is what keeps this target green.
func FuzzExtractICalEvents(f *testing.F) {
	for _, seed := range calendarICALFuzzSeeds {
		f.Add([]byte(seed))
	}

	windowStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, data []byte) {
		decoder := ical.NewDecoder(bytes.NewReader(data))
		for {
			_, err := decodeCalendarSafely(decoder, windowStart, windowEnd)
			if err != nil {
				return
			}
		}
	})
}
