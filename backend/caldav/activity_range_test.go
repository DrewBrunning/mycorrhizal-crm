package caldav

import (
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2006: a multi-day activity is served as an all-day VEVENT whose
// DTEND is the EXCLUSIVE day after the inclusive end; a single-day activity is
// served exactly as before (DATE-TIME start, no DTEND).
func TestMultiDayActivityServedAsAllDayRangeWithExclusiveDTEND(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	start := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	trip := models.Activity{UserID: user.ID, Title: "Trip", Date: start, EndDate: &end}
	require.NoError(t, db.Create(&trip).Error)
	single := models.Activity{UserID: user.ID, Title: "Coffee", Date: time.Date(2026, 5, 12, 9, 30, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&single).Error)

	objects, err := b.ListCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", nil)
	require.NoError(t, err)
	require.Len(t, objects, 2)
	byUID := map[string]*ical.Event{}
	for _, o := range objects {
		ev := o.Data.Events()[0]
		uid, err := ev.Props.Text(ical.PropUID)
		require.NoError(t, err)
		byUID[uid] = &ev
	}

	tripEv := byUID["interaction-"+trip.UUID]
	require.NotNil(t, tripEv)
	startProp := tripEv.Props.Get(ical.PropDateTimeStart)
	require.NotNil(t, startProp)
	assert.Equal(t, ical.ValueDate, startProp.ValueType(), "DTSTART is a DATE, not a DATE-TIME")
	endProp := tripEv.Props.Get(ical.PropDateTimeEnd)
	require.NotNil(t, endProp)
	assert.Equal(t, ical.ValueDate, endProp.ValueType())
	gotEnd, err := endProp.DateTime(time.UTC)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC), gotEnd, "inclusive 15 May is served as exclusive 16 May")

	singleEv := byUID["interaction-"+single.UUID]
	require.NotNil(t, singleEv)
	assert.Nil(t, singleEv.Props.Get(ical.PropDateTimeEnd), "single-day activities carry no DTEND")
	assert.Equal(t, ical.ValueDateTime, singleEv.Props.Get(ical.PropDateTimeStart).ValueType())
}

// A trip that began before the queried window but is still running inside it
// must be returned by a time-range REPORT; one entirely before it must not.
func TestQueryCalendarObjectsReturnsRangeOverlappingWindow(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	running := models.Activity{UserID: user.ID, Title: "Running trip", Date: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC), EndDate: &end}
	oldEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	over := models.Activity{UserID: user.ID, Title: "Over trip", Date: time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), EndDate: &oldEnd}
	require.NoError(t, db.Create(&running).Error)
	require.NoError(t, db.Create(&over).Error)

	query := &caldav.CalendarQuery{CompFilter: caldav.CompFilter{
		Name: ical.CompCalendar,
		Comps: []caldav.CompFilter{{
			Name:  ical.CompEvent,
			Start: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		}},
	}}
	objects, err := b.QueryCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", query)
	require.NoError(t, err)
	require.Len(t, objects, 1)
	assert.Equal(t, "interaction-"+running.UUID, uidFromPath(objects[0].Path))
}
