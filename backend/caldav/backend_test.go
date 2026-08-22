package caldav

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBackend builds a backend over a real migrated schema (CLAUDE.md
// backend trap 1) and returns it with a context bound to user.
func newTestBackend(t *testing.T) (*Backend, context.Context, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "caldav-test.db"))
	require.NoError(t, err)

	user := models.User{Username: "caldavuser", Password: "password123!A", Email: "caldav@example.com"}
	require.NoError(t, db.Create(&user).Error)

	b := NewBackend(db)
	ctx := ContextWithUser(context.Background(), user.ID, user.Username, db)
	return b, ctx, user
}

func TestActivityServedAsVEVENT(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	activity := models.Activity{
		UserID:      user.ID,
		Title:       "Lunch with Ada",
		Description: "Discussed the project",
		Location:    "Cafe",
		Date:        time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&activity).Error)

	objects, err := b.ListCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", nil)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	obj := objects[0]
	assert.Equal(t, "interaction-"+activity.UUID, uidFromPath(obj.Path))
	assert.Equal(t, activity.ETag, obj.ETag, "the served ETag must be the Activity's own ETag (T12a)")
	require.NotNil(t, obj.Data)

	events := obj.Data.Events()
	require.Len(t, events, 1)
	event := events[0]
	uid, err := event.Props.Text(ical.PropUID)
	require.NoError(t, err)
	assert.Equal(t, "interaction-"+activity.UUID, uid, "UID must be stable and derived from Activity.UUID")
	summary, _ := event.Props.Text(ical.PropSummary)
	assert.Equal(t, "Lunch with Ada", summary)
	dtstart, err := event.DateTimeStart(time.UTC)
	require.NoError(t, err)
	assert.True(t, dtstart.Equal(time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)), "DTSTART must be the full DATE-TIME")
}

func TestLifeEventServedAsYearlyVEVENT(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	le := models.LifeEvent{
		UserID:   user.ID,
		EntityID: "00000000-0000-4000-8000-000000000001",
		Type:     "adopted_pet",
		Date:     &contactmodel.PartialDate{Year: intPtr(2015), Month: intPtr(3), Day: intPtr(14)},
	}
	require.NoError(t, db.Create(&le).Error)

	objects, err := b.ListCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", nil)
	require.NoError(t, err)
	require.Len(t, objects, 1)

	events := objects[0].Data.Events()
	require.Len(t, events, 1)
	event := events[0]

	dtstart, err := event.DateTimeStart(time.UTC)
	require.NoError(t, err)
	// The served DTSTART is a DATE value (no time) so clients don't shift it.
	assert.Equal(t, "2015-03-14", dtstart.Format("2006-01-02"), "life event DTSTART must be date-only")
	rrule, err := event.Props.Text(ical.PropRecurrenceRule)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=YEARLY", rrule)
}

func TestYearOnlyLifeEventSkipped(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	le := models.LifeEvent{
		UserID:   user.ID,
		EntityID: "00000000-0000-4000-8000-000000000001",
		Type:     "moved",
		Date:     &contactmodel.PartialDate{Year: intPtr(2020)},
	}
	require.NoError(t, db.Create(&le).Error)

	objects, err := b.ListCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", nil)
	require.NoError(t, err)
	assert.Len(t, objects, 0, "a year-only life event must be skipped, not emitted broken")
}

func TestETagChangesOnlyWhenTheObjectChanges(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	activity := models.Activity{UserID: user.ID, Title: "Before", Date: time.Now().UTC()}
	require.NoError(t, db.Create(&activity).Error)

	first, err := b.GetCalendarObject(ctx, "/caldav/calendars/"+user.Username+"/interactions/interaction-"+activity.UUID+".ics", nil)
	require.NoError(t, err)
	firstETag := first.ETag

	// An unrelated write (another activity) must not change this object's ETag.
	other := models.Activity{UserID: user.ID, Title: "Unrelated", Date: time.Now().UTC()}
	require.NoError(t, db.Create(&other).Error)

	second, err := b.GetCalendarObject(ctx, "/caldav/calendars/"+user.Username+"/interactions/interaction-"+activity.UUID+".ics", nil)
	require.NoError(t, err)
	assert.Equal(t, firstETag, second.ETag, "ETag must be stable across an unrelated write")

	// Editing the activity must change its ETag. The ETag derives from
	// UpdatedAt at second granularity, so set updated_at explicitly to a
	// later second instead of time.Sleep-ing across a second boundary — the
	// canonical pattern in models/activity_test.go. This also removes the
	// wall-clock dependency that made the test slow and timing-sensitive.
	require.NoError(t, db.Model(&activity).Updates(map[string]any{
		"title":      "After",
		"updated_at": time.Now().Add(10 * time.Second),
	}).Error)
	third, err := b.GetCalendarObject(ctx, "/caldav/calendars/"+user.Username+"/interactions/interaction-"+activity.UUID+".ics", nil)
	require.NoError(t, err)
	assert.NotEqual(t, firstETag, third.ETag, "editing the activity must rotate its ETag")
}

func TestQueryCalendarObjectsWindowFiltering(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	inWindow := models.Activity{UserID: user.ID, Title: "In window", Date: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}
	outOfWindow := models.Activity{UserID: user.ID, Title: "Out of window", Date: time.Date(2027, 1, 1, 10, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&inWindow).Error)
	require.NoError(t, db.Create(&outOfWindow).Error)

	query := &caldav.CalendarQuery{
		CompFilter: caldav.CompFilter{
			Name: ical.CompCalendar,
			Comps: []caldav.CompFilter{{
				Name:  ical.CompEvent,
				Start: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			}},
		},
	}
	objects, err := b.QueryCalendarObjects(ctx, "/caldav/calendars/"+user.Username+"/interactions/", query)
	require.NoError(t, err)
	require.Len(t, objects, 1)
	assert.Equal(t, "interaction-"+inWindow.UUID, uidFromPath(objects[0].Path))
}

func TestServingIsReadOnly(t *testing.T) {
	b, ctx, _ := newTestBackend(t)

	_, err := b.PutCalendarObject(ctx, "/caldav/calendars/x/interactions/x.ics", ical.NewCalendar(), nil)
	assert.Error(t, err, "PUT must be unsupported (read-only serve)")

	err = b.DeleteCalendarObject(ctx, "/caldav/calendars/x/interactions/x.ics")
	assert.Error(t, err, "DELETE must be unsupported (read-only serve)")
}

func TestCalendarDiscoveryPaths(t *testing.T) {
	b, ctx, user := newTestBackend(t)

	principal, err := b.CurrentUserPrincipal(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/caldav/principals/"+user.Username+"/", principal)

	home, err := b.CalendarHomeSetPath(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/caldav/calendars/"+user.Username+"/", home)

	calendars, err := b.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, calendars, 1)
	assert.Equal(t, "/caldav/calendars/"+user.Username+"/interactions/", calendars[0].Path)
	assert.Equal(t, []string{ical.CompEvent}, calendars[0].SupportedComponentSet)
}

func intPtr(i int) *int { return &i }

// TestGetCalendarObject covers the read path of incremental calendar sync
// (ETag/ModTime/404 semantics) in a table: the resolved-object success cases
// for each entity type plus every not-found branch.
func TestGetCalendarObject(t *testing.T) {
	b, ctx, user := newTestBackend(t)
	db := b.getDB(ctx)

	activity := models.Activity{UserID: user.ID, Title: "Lunch", Date: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&activity).Error)

	le := models.LifeEvent{
		UserID:   user.ID,
		EntityID: "00000000-0000-4000-8000-000000000002",
		Type:     "birthday",
		Date:     &contactmodel.PartialDate{Year: intPtr(1990), Month: intPtr(6), Day: intPtr(15)},
	}
	require.NoError(t, db.Create(&le).Error)

	yearOnly := models.LifeEvent{
		UserID:   user.ID,
		EntityID: "00000000-0000-4000-8000-000000000003",
		Type:     "moved",
		Date:     &contactmodel.PartialDate{Year: intPtr(2020)},
	}
	require.NoError(t, db.Create(&yearOnly).Error)

	tests := []struct {
		name     string
		path     string
		wantErr  string // substring expected in the error; empty means success
		wantUID  string // expected object UID on success
		wantETag string // expected ETag on success
		wantMod  time.Time
	}{
		{name: "activity resolves", path: "/caldav/calendars/" + user.Username + "/interactions/interaction-" + activity.UUID + ".ics",
			wantUID: "interaction-" + activity.UUID, wantETag: activity.ETag, wantMod: activity.UpdatedAt},
		{name: "life event resolves", path: "/caldav/calendars/" + user.Username + "/interactions/life-event-" + le.ID + ".ics",
			wantUID: "life-event-" + le.ID, wantETag: le.ETag, wantMod: le.UpdatedAt},
		{name: "year-only life event is not found", path: "/caldav/calendars/" + user.Username + "/interactions/life-event-" + yearOnly.ID + ".ics", wantErr: "event not found"},
		{name: "unknown activity uid is not found", path: "/caldav/calendars/" + user.Username + "/interactions/interaction-does-not-exist.ics", wantErr: "event not found"},
		{name: "unknown life event is not found", path: "/caldav/calendars/" + user.Username + "/interactions/life-event-does-not-exist.ics", wantErr: "event not found"},
		{name: "unrecognized prefix is not found", path: "/caldav/calendars/" + user.Username + "/interactions/something-else.ics", wantErr: "event not found"},
		{name: "path without uid is invalid", path: "/caldav/calendars/" + user.Username + "/interactions/", wantErr: "invalid path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := b.GetCalendarObject(ctx, tt.path, nil)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Nil(t, obj)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, obj)
			assert.Equal(t, tt.wantUID, uidFromPath(obj.Path))
			assert.Equal(t, tt.wantETag, obj.ETag, "the served ETag must be the source row's own ETag (incremental sync key)")
			assert.True(t, obj.ModTime.Equal(tt.wantMod), "ModTime must be the source row's UpdatedAt")
			require.NotNil(t, obj.Data)
		})
	}
}

// uidFromPath extracts the last path segment (the resource UID, without the
// .ics extension).
func uidFromPath(p string) string {
	trimmed := p
	for len(trimmed) > 0 && trimmed[len(trimmed)-1] == '/' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] == '/' {
			return strings.TrimSuffix(trimmed[i+1:], ".ics")
		}
	}
	return strings.TrimSuffix(trimmed, ".ics")
}
