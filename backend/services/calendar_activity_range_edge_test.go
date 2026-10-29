package services

import (
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A malformed DTEND must not drop the event or invent a range.
func TestExtractEventsMalformedDTENDHasNoRange(t *testing.T) {
	events := parseEvents(t, "BEGIN:VEVENT\nUID:bad\nSUMMARY:Bad end\nDTSTART;VALUE=DATE:20260512\nDTEND;VALUE=DATE:notadate\nEND:VEVENT")
	require.Len(t, events, 1)
	assert.Nil(t, events[0].EndDate)
}

// A reversed stored range (should be unreachable via the API) is exported as a
// single all-day, never a negative-length event.
func TestActivityToICalendarClampsReversedRange(t *testing.T) {
	end := day(2026, 5, 1)
	a := &models.Activity{Title: "Odd", Date: day(2026, 5, 12), EndDate: &end}
	out := encodeCal(t, activityToICalendar("uid", a))
	assert.Contains(t, out, "DTSTART;VALUE=DATE:20260512")
	assert.Contains(t, out, "DTEND;VALUE=DATE:20260513")
}
