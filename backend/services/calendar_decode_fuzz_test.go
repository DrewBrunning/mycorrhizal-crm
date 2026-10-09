package services

import (
	"bytes"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// calendarDecodeFuzzSeeds start from the shapes FuzzExtractICalEvents uses
// (a plain event, an all-day event, a cancelled event, a recurring event with
// EXDATE) plus the decoder panic input go-ical itself found — "0;0=" (see
// decodeCalendarSafely's doc comment) — which the recover must contain.
var calendarDecodeFuzzSeeds = []string{
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:event-1\n" +
		"SUMMARY:Quarterly catch-up\nDESCRIPTION:Notes\nLOCATION:Cafe Central\n" +
		"ATTENDEE:mailto:ada@example.com\nDTSTART:20260301T100000Z\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:event-2\n" +
		"SUMMARY:All-day planning\nDTSTART;VALUE=DATE:20260305\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:cancelled\n" +
		"SUMMARY:Cancelled\nSTATUS:CANCELLED\nDTSTART:20260302T090000Z\nEND:VEVENT\nEND:VCALENDAR\n",
	"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:weekly\n" +
		"SUMMARY:Weekly sync\nDTSTART:20260101T090000Z\nRRULE:FREQ=WEEKLY\nEXDATE:20260115T090000Z\n" +
		"END:VEVENT\nEND:VCALENDAR\n",
	// The known go-ical decoder panic (index-out-of-range in lineDecoder.peek).
	"0;0=",
	"",
	"BEGIN:VCALENDAR\nEND:VCALENDAR\n",
}

// FuzzDecodeCalendarSafely fuzzes decodeCalendarSafely (issue #1626), the
// panic-recovery boundary in calendar_sync_service.go. Its whole reason to
// exist is to CONTAIN panics from github.com/emersion/go-ical parsing bytes
// fetched from a subscribed *remote* calendar — an unrecovered panic there
// crashes the recurring sync goroutine, not just one subscription. The
// property the harness asserts is therefore the containment itself: the call
// returns normally with either (events, nil) or (nil, error), and a
// fuzzer-found panic that escapes would fail go test -fuzz automatically.
//
// The window is deliberately one day, not the ten years
// FuzzExtractICalEvents uses: extractEvents expands RRULEs into one event per
// occurrence inside the window, so a FREQ=SECONDLY rule over a multi-year
// window is a multi-hundred-million-event hang, not a parser bug. Panic
// containment is the target, and a narrow window keeps it that.
func FuzzDecodeCalendarSafely(f *testing.F) {
	for _, seed := range calendarDecodeFuzzSeeds {
		f.Add([]byte(seed))
	}

	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, data []byte) {
		events, err := decodeCalendarSafely(ical.NewDecoder(bytes.NewReader(data)), windowStart, windowEnd)
		if err != nil && events != nil {
			t.Fatalf("decodeCalendarSafely returned events alongside an error: %v", err)
		}
	})
}
