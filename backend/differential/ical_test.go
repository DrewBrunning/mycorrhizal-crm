package differential

import (
	"strings"
	"testing"
	"time"

	golangical "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/require"
)

// TestICalCorpus verifies the iCalendar corpus assembly: the canonical
// fixture's Activities are all present, and year-only LifeEvents (which the
// serve path deliberately skips) are excluded.
func TestICalCorpus(t *testing.T) {
	corpus, err := ICalCorpus()
	require.NoError(t, err)
	require.NotEmpty(t, corpus)

	var activities, lifeEvents int
	seen := map[string]bool{}
	for _, e := range corpus {
		require.False(t, seen[e.ID], "duplicate corpus ID %s", e.ID)
		seen[e.ID] = true
		if e.Activity != nil {
			activities++
			require.Nil(t, e.LifeEvent)
		} else {
			lifeEvents++
			require.NotNil(t, e.LifeEvent)
			require.NotNil(t, e.LifeEvent.Date)
			require.NotNil(t, e.LifeEvent.Date.Month, "life event %s must have a month to be a calendar event", e.ID)
			require.NotNil(t, e.LifeEvent.Date.Day, "life event %s must have a day to be a calendar event", e.ID)
		}
	}
	require.Positive(t, activities, "corpus must include the fixture Activities")
	require.Positive(t, lifeEvents, "corpus must include the month/day LifeEvents")
}

// TestExpectedEvent verifies the expected-surface derivation for both event
// kinds (activity vs recurring life event).
func TestExpectedEvent(t *testing.T) {
	corpus, err := ICalCorpus()
	require.NoError(t, err)
	for _, e := range corpus {
		want := e.ExpectedEvent()
		require.NotEmpty(t, want.Summary)
		if e.Activity != nil {
			require.True(t, strings.HasPrefix(want.UID, "interaction-"))
			require.False(t, want.DateOnly)
			require.Empty(t, want.RRule)
		} else {
			require.True(t, strings.HasPrefix(want.UID, "life-event-"))
			require.True(t, want.DateOnly)
			require.Equal(t, "FREQ=YEARLY", want.RRule)
		}
		require.False(t, want.Start.IsZero())
	}
}

// TestICalEventDiff verifies the named-field comparison: identical events
// yield no diffs; each differing field is named.
func TestICalEventDiff(t *testing.T) {
	base := ICalEvent{UID: "u", Summary: "s", Description: "d", Location: "l", Start: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC), RRule: "FREQ=YEARLY"}
	require.Empty(t, base.Diff(base))

	got := base
	got.UID = "other"
	got.Summary = "other"
	got.Description = "other"
	got.Location = "other"
	got.Start = got.Start.Add(time.Hour)
	got.RRule = ""
	diffs := base.Diff(got)
	require.Len(t, diffs, 6)
	for _, want := range []string{"uid", "summary", "description", "location", "dtstart", "rrule"} {
		require.Contains(t, strings.Join(diffs, "\n"), want+":")
	}
}

// TestGoIcalRoundTrip verifies the golang-ical translators used by both
// iCalendar legs: an event serialized by the reference and parsed back
// through its own parser reproduces the surface (a sanity check on the
// translator pair, and the coverage for the differential package's own copy
// of the helpers).
func TestGoIcalRoundTrip(t *testing.T) {
	for _, want := range []ICalEvent{
		{UID: "interaction-a", Summary: "Coffee", Description: "Notes", Location: "Blue Bottle", Start: time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)},
		{UID: "life-event-1", Summary: "had_child", Description: "Eve", Start: time.Date(2010, 6, 15, 0, 0, 0, 0, time.UTC), DateOnly: true, RRule: "FREQ=YEARLY"},
	} {
		cal := GoIcalFromEvent(want)
		serialized := cal.Serialize()
		parsed, err := golangical.ParseCalendar(strings.NewReader(serialized))
		require.NoError(t, err)
		events := parsed.Events()
		require.Len(t, events, 1)
		got := GoIcalEventFrom(events[0])
		require.Empty(t, want.Diff(got))
	}
}

// TestParseGoIcalDtStart covers both DTSTART forms golang-ical emits.
func TestParseGoIcalDtStart(t *testing.T) {
	dt, dateOnly := parseGoIcalDtStart("20260510T090000Z")
	require.False(t, dateOnly)
	require.Equal(t, time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC), dt)

	d, dateOnly := parseGoIcalDtStart("20260510")
	require.True(t, dateOnly)
	require.Equal(t, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), d)

	// Offset form (TZID'd DTSTART) parses to the instant.
	off, _ := parseGoIcalDtStart("20260510T090000-0400")
	require.Equal(t, time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC), off)

	// Garbage yields a zero time without panicking.
	z, _ := parseGoIcalDtStart("not-a-date")
	require.True(t, z.IsZero())
}
