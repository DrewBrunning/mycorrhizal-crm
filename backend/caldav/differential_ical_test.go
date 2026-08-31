package caldav

import (
	"bytes"
	"strings"
	"testing"

	golangical "github.com/arran4/golang-ical"
	"github.com/emersion/go-ical"
	"github.com/stretchr/testify/require"

	"mycorrhizal/differential"
)

// TestICalDifferential_OursToReference is the ours -> reference half of the
// TEST-08 iCalendar leg (issue #680): our caldav serve serializer
// (calendarObjectFromActivity / calendarObjectFromLifeEvent) produces the
// .ics bytes, and the pinned independent reference implementation
// (github.com/arran4/golang-ical) parses them back. A disagreement fails
// with the named VEVENT property (uid/summary/description/location/dtstart/
// rrule), never "outputs differ".
func TestICalDifferential_OursToReference(t *testing.T) {
	corpus, err := differential.ICalCorpus()
	require.NoError(t, err)
	require.NotEmpty(t, corpus)

	for _, entry := range corpus {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			want := entry.ExpectedEvent()
			var obj *CalendarObject
			if entry.Activity != nil {
				obj = calendarObjectFromActivity("user", entry.Activity)
			} else {
				obj = calendarObjectFromLifeEvent("user", entry.LifeEvent)
			}
			var buf bytes.Buffer
			require.NoError(t, ical.NewEncoder(&buf).Encode(obj.Data))
			raw := buf.Bytes()

			parsed, err := golangical.ParseCalendar(strings.NewReader(string(raw)))
			require.NoError(t, err, "golang-ical failed to parse our .ics output")
			events := parsed.Events()
			require.Len(t, events, 1, "expected exactly one VEVENT in our .ics output")
			got := differential.GoIcalEventFrom(events[0])

			for _, d := range want.Diff(got) {
				t.Errorf("ours -> reference %s: %s", entry.ID, d)
			}
		})
	}
}
