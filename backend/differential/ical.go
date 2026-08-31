package differential

import (
	"fmt"
	"strings"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/models"

	golangical "github.com/arran4/golang-ical"
)

// The iCalendar differential leg (issue #680) compares our caldav
// serialization/parsing against github.com/arran4/golang-ical (pinned in
// go.mod) — a genuinely independent pure-Go iCalendar implementation (our
// caldav backend is built on emersion/go-ical). The corpus is the canonical
// fixture's Activities and month/day LifeEvents: the serve-side objects our
// CalDAV collection publishes.
//
// The comparison surface is event-level, not the contact-level semanticequal
// of the other legs: an Activity/LifeEvent serializes to a VEVENT whose
// fields (UID, SUMMARY, DESCRIPTION, LOCATION, DTSTART, RRULE) are compared
// by name, so a disagreement fails with the named property.
//
// Both directions live in in-package tests (backend/caldav for our
// serializer, backend/services for our import parser), because the production
// entry points are unexported; this file holds the shared corpus, the
// comparison surface, and the golang-ical <-> surface translators.

// ICalEvent is the comparison surface for one calendar event.
type ICalEvent struct {
	// UID is the VEVENT UID (the stable external identity).
	UID string
	// Summary is the VEVENT SUMMARY (an Activity's title or a LifeEvent's
	// type token).
	Summary string
	// Description is the VEVENT DESCRIPTION (empty when absent).
	Description string
	// Location is the VEVENT LOCATION (empty when absent).
	Location string
	// Start is the DTSTART as a UTC instant.
	Start time.Time
	// DateOnly reports a VALUE=DATE DTSTART (all-day recurring LifeEvents).
	DateOnly bool
	// RRule is the VEVENT RRULE value (empty for Activities).
	RRule string
}

// ICalCorpusEntry is one event in the iCalendar differential corpus.
type ICalCorpusEntry struct {
	ID string
	// Exactly one of Activity / LifeEvent is set.
	Activity  *models.Activity
	LifeEvent *models.LifeEvent
}

// ICalCorpus assembles the iCalendar corpus from the canonical fixture's
// Activities and month/day LifeEvents (year-only LifeEvents are deliberately
// not calendar events — the serve path skips them).
func ICalCorpus() ([]ICalCorpusEntry, error) {
	m, err := canonicalfixture.Read()
	if err != nil { // # pragma: no cover — a broken checked-in manifest fails the canonicalfixture suite first
		return nil, err // # pragma: no cover
	}
	var out []ICalCorpusEntry
	for i, a := range m.Activities {
		out = append(out, ICalCorpusEntry{
			ID: fmt.Sprintf("activity/%d", i),
			Activity: &models.Activity{
				Title:       a.Title,
				Description: a.Description,
				Location:    a.Location,
				Date:        a.Date.UTC(),
			},
		})
	}
	for i, le := range m.LifeEvents {
		if le.Date == nil || le.Date.Month == nil || le.Date.Day == nil {
			continue
		}
		year := 2000
		if le.Date.Year != nil {
			year = *le.Date.Year
		}
		out = append(out, ICalCorpusEntry{
			ID: fmt.Sprintf("life-event/%d", i),
			LifeEvent: &models.LifeEvent{
				ID:          fmt.Sprintf("le-%d", i),
				Type:        le.Type,
				Description: le.Description,
				Date: &contactmodel.PartialDate{
					Year:  intPtr2(year),
					Month: intPtr2(*le.Date.Month),
					Day:   intPtr2(*le.Date.Day),
				},
			},
		})
	}
	return out, nil
}

func intPtr2(v int) *int { return &v }

// ExpectedEvent derives the comparison surface directly from the corpus
// entry: the fields the serve serializer writes and the import parser reads.
func (e ICalCorpusEntry) ExpectedEvent() ICalEvent {
	if e.Activity != nil {
		a := e.Activity
		return ICalEvent{
			UID:         "interaction-" + a.UUID,
			Summary:     a.Title,
			Description: a.Description,
			Location:    a.Location,
			Start:       a.Date.UTC(),
		}
	}
	le := e.LifeEvent
	year := 2000
	if le.Date.Year != nil {
		year = *le.Date.Year
	}
	return ICalEvent{
		UID:         "life-event-" + le.ID,
		Summary:     le.Type,
		Description: le.Description,
		Start:       time.Date(year, time.Month(*le.Date.Month), *le.Date.Day, 0, 0, 0, 0, time.UTC),
		DateOnly:    true,
		RRule:       "FREQ=YEARLY",
	}
}

// Diff reports the named fields on which got differs from want, or nil when
// they describe the same event. Values are compared semantically: the DTSTART
// is an instant (offset-insensitive), and an empty RRule on one side is
// treated as absent.
func (want ICalEvent) Diff(got ICalEvent) []string {
	var diffs []string
	if want.UID != "" && got.UID != "" && want.UID != got.UID {
		diffs = append(diffs, fmt.Sprintf("uid: want %q got %q", want.UID, got.UID))
	}
	if want.Summary != got.Summary {
		diffs = append(diffs, fmt.Sprintf("summary: want %q got %q", want.Summary, got.Summary))
	}
	if want.Description != got.Description {
		diffs = append(diffs, fmt.Sprintf("description: want %q got %q", want.Description, got.Description))
	}
	if want.Location != got.Location {
		diffs = append(diffs, fmt.Sprintf("location: want %q got %q", want.Location, got.Location))
	}
	if !want.Start.Equal(got.Start) {
		diffs = append(diffs, fmt.Sprintf("dtstart: want %s got %s", want.Start.Format(time.RFC3339), got.Start.Format(time.RFC3339)))
	}
	if want.RRule != "" || got.RRule != "" {
		if strings.TrimSpace(want.RRule) != strings.TrimSpace(got.RRule) {
			diffs = append(diffs, fmt.Sprintf("rrule: want %q got %q", want.RRule, got.RRule))
		}
	}
	return diffs
}

// GoIcalFromEvent builds a golang-ical calendar (the reference's emission)
// from the comparison surface, for the reference -> ours direction.
func GoIcalFromEvent(e ICalEvent) *golangical.Calendar {
	cal := golangical.NewCalendar()
	cal.SetVersion("2.0")
	ev := golangical.NewEvent(e.UID)
	ev.SetSummary(e.Summary)
	if e.Description != "" {
		ev.SetDescription(e.Description)
	}
	if e.Location != "" {
		ev.SetLocation(e.Location)
	}
	if e.DateOnly {
		ev.SetProperty(golangical.ComponentPropertyDtStart, e.Start.Format("20060102"), golangical.WithValue("DATE"))
	} else {
		ev.SetProperty(golangical.ComponentPropertyDtStart, e.Start.UTC().Format("20060102T150405Z"))
	}
	if e.RRule != "" {
		ev.SetProperty(golangical.ComponentPropertyRrule, e.RRule)
	}
	cal.AddVEvent(ev)
	return cal
}

// GoIcalEventFrom reads a golang-ical VEVENT back into the comparison
// surface (the reference's reading of our .ics), for the ours -> reference
// direction.
func GoIcalEventFrom(ev *golangical.VEvent) ICalEvent {
	out := ICalEvent{}
	if p := ev.GetProperty(golangical.ComponentPropertyUniqueId); p != nil {
		out.UID = p.Value
	}
	if p := ev.GetProperty(golangical.ComponentPropertySummary); p != nil {
		out.Summary = p.Value
	}
	if p := ev.GetProperty(golangical.ComponentPropertyDescription); p != nil {
		out.Description = p.Value
	}
	if p := ev.GetProperty(golangical.ComponentPropertyLocation); p != nil {
		out.Location = p.Value
	}
	if p := ev.GetProperty(golangical.ComponentPropertyDtStart); p != nil {
		out.Start, out.DateOnly = parseGoIcalDtStart(p.Value)
	}
	if p := ev.GetProperty(golangical.ComponentPropertyRrule); p != nil {
		out.RRule = p.Value
	}
	return out
}

func parseGoIcalDtStart(raw string) (time.Time, bool) {
	if i := strings.IndexByte(raw, 'T'); i >= 0 {
		t, err := time.Parse("20060102T150405Z", raw)
		if err == nil {
			return t.UTC(), false
		}
		// Offset form (e.g. DTSTART;TZID=...:20260510T090000).
		for _, layout := range []string{"20060102T150405Z0700", "20060102T150405-0700"} {
			if t, err := time.Parse(layout, raw); err == nil {
				return t.UTC(), false
			}
		}
	}
	t, err := time.Parse("20060102", raw)
	if err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}
