package services

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/stretchr/testify/require"

	"mycorrhizal/differential"
)

// TestICalDifferential_ReferenceToOurs is the reference -> ours half of the
// TEST-08 iCalendar leg (issue #680): the pinned independent reference
// implementation (github.com/arran4/golang-ical) emits the .ics from the
// corpus event, and OUR import pipeline (extractEvents, the same code path
// a real CalDAV subscription fetch goes through) parses it back. A
// disagreement fails with the named VEVENT property.
//
// LifeEvents are recurring (FREQ=YEARLY); extractEvents expands the
// recurrence, so the first occurrence in the window is compared — its
// summary/description/location/date must match the source life event.
func TestICalDifferential_ReferenceToOurs(t *testing.T) {
	corpus, err := differential.ICalCorpus()
	require.NoError(t, err)
	require.NotEmpty(t, corpus)

	for _, entry := range corpus {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			want := entry.ExpectedEvent()

			// The reference emits the .ics.
			refCal := differential.GoIcalFromEvent(want)
			raw := refCal.Serialize()

			// Our pipeline parses it: emersion decoder -> extractEvents.
			dec := ical.NewDecoder(strings.NewReader(raw))
			parsed, err := dec.Decode()
			require.NoError(t, err, "our emersion-based decoder failed on golang-ical's .ics output")
			win := windowFor(want)
			events := extractEvents(parsed, win.start, win.end)
			require.NotEmpty(t, events, "our import parser found no events in golang-ical's .ics output")

			// Recurring life events expand into per-year occurrences; the
			// first occurrence in the window is the comparison target. Its
			// UID is the derived occurrence UID (base + "#" + occurrence),
			// so the base UID must be a prefix for recurring events.
			got := events[0]
			if want.RRule != "" {
				if !strings.HasPrefix(got.UID, want.UID) {
					t.Errorf("reference -> ours %s: uid: want prefix %q got %q", entry.ID, want.UID, got.UID)
				}
			} else if got.UID != want.UID {
				t.Errorf("reference -> ours %s: uid: want %q got %q", entry.ID, want.UID, got.UID)
			}
			if got.Title != want.Summary {
				t.Errorf("reference -> ours %s: summary: want %q got %q", entry.ID, want.Summary, got.Title)
			}
			if got.Description != want.Description {
				t.Errorf("reference -> ours %s: description: want %q got %q", entry.ID, want.Description, got.Description)
			}
			if got.Location != want.Location {
				t.Errorf("reference -> ours %s: location: want %q got %q", entry.ID, want.Location, got.Location)
			}
			if !got.Date.UTC().Equal(want.Start) {
				t.Errorf("reference -> ours %s: dtstart: want %s got %s", entry.ID, want.Start.Format(time.RFC3339), got.Date.UTC().Format(time.RFC3339))
			}
		})
	}
}

type window struct{ start, end time.Time }

func windowFor(e differential.ICalEvent) window {
	return window{start: e.Start.Add(-24 * time.Hour), end: e.Start.Add(24 * time.Hour)}
}
