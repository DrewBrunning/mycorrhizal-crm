// Package caldav serves the CRM's own Interactions (Activities) and
// LifeEvents out as a CalDAV/iCalendar collection that external calendar
// clients can subscribe to (T12b).
// The import direction — a subscribed remote calendar becoming Activities —
// lives in services/calendar_sync_service.go; this package is the serve half.
//
// T12b is read-only by design: PutCalendarObject/DeleteCalendarObject return
// errors. Two-way (T13, T13) is
// about pushing CRM edits out to a *subscribed* remote calendar, which is a
// CalendarSyncService concern, not a server-side PUT path.
package caldav

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mycorrhizal/models"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"gorm.io/gorm"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	userIDKey   contextKey = "userID"
	usernameKey contextKey = "username"
	dbKey       contextKey = "db"
)

// calendarPathParts mirror the CardDAV layout (carddav/backend.go) so a DAV
// client can discover both address books and calendars from one principal.
const (
	principalPrefix = "/caldav/principals/"
	calendarHome    = "/caldav/calendars/"
	calendarName    = "interactions"
)

// Backend implements the caldav.Backend interface for the CRM's own
// Activities + LifeEvents. One calendar per user.
type Backend struct {
	db *gorm.DB
}

// NewBackend creates a CalDAV backend bound to the given DB.
func NewBackend(db *gorm.DB) *Backend {
	return &Backend{db: db}
}

// ContextWithUser adds user info to the request context, mirroring
// carddav.ContextWithUser.
func ContextWithUser(ctx context.Context, userID uint, username string, db *gorm.DB) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, usernameKey, username)
	ctx = context.WithValue(ctx, dbKey, db)
	return ctx
}

func (b *Backend) getUserID(ctx context.Context) (uint, error) {
	userID, ok := ctx.Value(userIDKey).(uint)
	if !ok {
		return 0, fmt.Errorf("user not authenticated")
	}
	return userID, nil
}

func (b *Backend) getUsername(ctx context.Context) string {
	username, _ := ctx.Value(usernameKey).(string)
	return username
}

func (b *Backend) getDB(ctx context.Context) *gorm.DB {
	if db, ok := ctx.Value(dbKey).(*gorm.DB); ok {
		return db
	}
	return b.db
}

// CurrentUserPrincipal returns the current user's principal URL.
func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	username := b.getUsername(ctx)
	if username == "" {
		return "", fmt.Errorf("user not authenticated")
	}
	return principalPrefix + username + "/", nil
}

// CalendarHomeSetPath returns the path to the calendar home set.
func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	username := b.getUsername(ctx)
	if username == "" {
		return "", fmt.Errorf("user not authenticated")
	}
	return calendarHome + username + "/", nil
}

// calendar returns the single per-user calendar descriptor.
func calendarFor(username string) caldav.Calendar {
	return caldav.Calendar{
		Path:                  calendarHome + username + "/" + calendarName + "/",
		Name:                  "Interactions & Life Events",
		Description:           "Mycorrhizal CRM Interactions and Life Events",
		SupportedComponentSet: []string{ical.CompEvent},
	}
}

// ListCalendars returns the current user's calendars (one).
func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	username := b.getUsername(ctx)
	if username == "" {
		return nil, fmt.Errorf("user not authenticated")
	}
	return []caldav.Calendar{calendarFor(username)}, nil
}

// GetCalendar returns a specific calendar.
func (b *Backend) GetCalendar(ctx context.Context, urlPath string) (*caldav.Calendar, error) {
	username := b.getUsername(ctx)
	if username == "" {
		return nil, fmt.Errorf("user not authenticated")
	}
	expected := calendarFor(username).Path
	if urlPath != expected && urlPath+"/" != expected {
		return nil, fmt.Errorf("calendar not found")
	}
	cal := calendarFor(username)
	return &cal, nil
}

// CreateCalendar is unsupported: each user has exactly one calendar, served
// read-only, matching CardDAV's own single-address-book stance.
func (b *Backend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	return fmt.Errorf("creating calendars is not supported")
}

// DeleteCalendar is unsupported (see CreateCalendar).
func (b *Backend) DeleteCalendar(ctx context.Context, urlPath string) error {
	return fmt.Errorf("deleting calendars is not supported")
}

// CalendarObject is one served iCalendar resource (an Activity or a
// LifeEvent) with its stable path and ETag.
type CalendarObject struct {
	Path    string
	ModTime time.Time
	ETag    string
	Data    *ical.Calendar
}

// calendarObjectFromActivity builds the served iCalendar object for an
// Activity, wrapped in a VCALENDAR. The UID is stable across regenerations
// (derived from Activity.UUID, never from row IDs) so clients don't
// duplicate events on every sync.
func calendarObjectFromActivity(username string, a *models.Activity) *CalendarObject {
	uid := "interaction-" + a.UUID
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Mycorrhizal CRM//CalDAV//EN")

	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, uid)
	// Activity.Date is a full time.Time — a DATE-TIME, so clients never
	// shift it by a day (the DATE-vs-DATE-TIME trap).
	event.Props.SetDateTime(ical.PropDateTimeStart, a.Date.UTC())
	event.Props.SetDateTime(ical.PropDateTimeStamp, a.UpdatedAt.UTC())
	event.Props.SetText(ical.PropSummary, a.Title)
	if a.Description != "" {
		event.Props.SetText(ical.PropDescription, a.Description)
	}
	if a.Location != "" {
		event.Props.SetText(ical.PropLocation, a.Location)
	}
	cal.Children = append(cal.Children, event.Component)

	return &CalendarObject{
		Path:    calendarHome + username + "/" + calendarName + "/" + uid + ".ics",
		ModTime: a.UpdatedAt,
		ETag:    a.ETag,
		Data:    cal,
	}
}

// calendarObjectFromLifeEvent builds the served iCalendar object for a
// LifeEvent as an annually-recurring VEVENT (a birthday, an anniversary, a
// got-a-pet date...). A year-only event (no month/day) cannot be a calendar
// event at all and is deliberately skipped by the caller (lifeEventsToObjects).
func calendarObjectFromLifeEvent(username string, le *models.LifeEvent) *CalendarObject {
	uid := "life-event-" + le.ID
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Mycorrhizal CRM//CalDAV//EN")

	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, uid)
	// RFC 5545 requires DTSTAMP on every VEVENT; the activities path sets it
	// from UpdatedAt, and this path must too (the TEST-08 iCalendar
	// differential against golang-ical surfaced the omission: a conformant
	// client validates it).
	event.Props.SetDateTime(ical.PropDateTimeStamp, le.UpdatedAt.UTC())
	// Date-only DTSTART (VALUE=DATE) with a YEARLY recurrence, so clients
	// render it as a recurring all-day event and never timezone-shift it.
	// The anchor year is the event's own year when known, else the current
	// year — the RRULE makes it recur regardless of the anchor.
	year := time.Now().Year()
	if le.Date.Year != nil {
		year = *le.Date.Year
	}
	dtstart := time.Date(year, time.Month(*le.Date.Month), *le.Date.Day, 0, 0, 0, 0, time.UTC)
	event.Props.SetDate(ical.PropDateTimeStart, dtstart)
	event.Props.SetText(ical.PropRecurrenceRule, "FREQ=YEARLY")
	// The type token ("married", "adopted_pet") is the summary; the free-text
	// description carries the detail. Display labels live in the frontend's
	// i18n, which this serve path has no access to.
	event.Props.SetText(ical.PropSummary, le.Type)
	if le.Description != "" {
		event.Props.SetText(ical.PropDescription, le.Description)
	}
	cal.Children = append(cal.Children, event.Component)

	return &CalendarObject{
		Path:    calendarHome + username + "/" + calendarName + "/" + uid + ".ics",
		ModTime: le.UpdatedAt,
		ETag:    le.ETag,
		Data:    cal,
	}
}

// lifeEventHasCalendarDate reports whether a LifeEvent has the month+day
// needed to be a calendar event. Year-only events (common — a life event is
// often known only to a year) are skipped rather than emitted broken.
func lifeEventHasCalendarDate(le *models.LifeEvent) bool {
	return le != nil && le.Date != nil && le.Date.Month != nil && le.Date.Day != nil
}

// ListCalendarObjects serves every Activity and every month/day LifeEvent for
// the user's calendar. Sensitivity gating: Activities and LifeEvents carry no
// sensitivity field today, so there is nothing to exclude — if one is added
// later, secret-sensitivity rows must be filtered here (the calendar leaves
// the instance).
func (b *Backend) ListCalendarObjects(ctx context.Context, urlPath string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	userID, err := b.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	username := b.getUsername(ctx)
	db := b.getDB(ctx)

	objects := []caldav.CalendarObject{}

	var activities []models.Activity
	if err := db.Where("user_id = ?", userID).Find(&activities).Error; err != nil {
		return nil, err
	}
	for i := range activities {
		obj := calendarObjectFromActivity(username, &activities[i])
		objects = append(objects, caldav.CalendarObject{Path: obj.Path, ModTime: obj.ModTime, ETag: obj.ETag, Data: obj.Data})
	}

	var lifeEvents []models.LifeEvent
	if err := db.Where("user_id = ?", userID).Find(&lifeEvents).Error; err != nil {
		return nil, err
	}
	for i := range lifeEvents {
		if !lifeEventHasCalendarDate(&lifeEvents[i]) {
			continue
		}
		obj := calendarObjectFromLifeEvent(username, &lifeEvents[i])
		objects = append(objects, caldav.CalendarObject{Path: obj.Path, ModTime: obj.ModTime, ETag: obj.ETag, Data: obj.Data})
	}

	return objects, nil
}

// GetCalendarObject returns one served calendar object by path.
func (b *Backend) GetCalendarObject(ctx context.Context, urlPath string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	userID, err := b.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	username := b.getUsername(ctx)
	db := b.getDB(ctx)

	uid := extractUIDFromPath(urlPath)
	if uid == "" {
		return nil, fmt.Errorf("invalid path")
	}

	if strings.HasPrefix(uid, "interaction-") {
		var a models.Activity
		if err := db.Where("user_id = ? AND uuid = ?", userID, strings.TrimPrefix(uid, "interaction-")).First(&a).Error; err != nil {
			return nil, fmt.Errorf("event not found")
		}
		obj := calendarObjectFromActivity(username, &a)
		return &caldav.CalendarObject{Path: obj.Path, ModTime: obj.ModTime, ETag: obj.ETag, Data: obj.Data}, nil
	}

	if strings.HasPrefix(uid, "life-event-") {
		var le models.LifeEvent
		if err := db.Where("user_id = ? AND id = ?", userID, strings.TrimPrefix(uid, "life-event-")).First(&le).Error; err != nil {
			return nil, fmt.Errorf("event not found")
		}
		if !lifeEventHasCalendarDate(&le) {
			return nil, fmt.Errorf("event not found")
		}
		obj := calendarObjectFromLifeEvent(username, &le)
		return &caldav.CalendarObject{Path: obj.Path, ModTime: obj.ModTime, ETag: obj.ETag, Data: obj.Data}, nil
	}

	return nil, fmt.Errorf("event not found")
}

// QueryCalendarObjects handles calendar-query REPORTs, filtering the served
// set by time range when the client asks for one. ETag comparison is what
// gives clients incremental sync (go-webdav v0.7.0 has no RFC 6578
// sync-collection REPORT, the same reality as the CardDAV side).
func (b *Backend) QueryCalendarObjects(ctx context.Context, urlPath string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	objects, err := b.ListCalendarObjects(ctx, urlPath, &query.CompRequest)
	if err != nil {
		return nil, err
	}
	return filterObjectsByWindow(objects, query), nil
}

// filterObjectsByWindow keeps only calendar objects overlapping the query's
// time window. A VEVENT without a DTSTART (shouldn't happen here) always
// survives, since there is nothing to test it against.
func filterObjectsByWindow(objects []caldav.CalendarObject, query *caldav.CalendarQuery) []caldav.CalendarObject {
	if query == nil {
		return objects
	}
	var start, end time.Time
	hasWindow := false
	for _, comp := range query.CompFilter.Comps {
		if comp.Name == ical.CompEvent {
			start, end = comp.Start, comp.End
			hasWindow = true
			break
		}
	}
	if !hasWindow {
		return objects
	}

	out := make([]caldav.CalendarObject, 0, len(objects))
	for _, obj := range objects {
		if obj.Data == nil {
			continue
		}
		keep := false
		for _, event := range obj.Data.Events() {
			date, err := event.DateTimeStart(time.UTC)
			if err != nil || date.IsZero() {
				keep = true // no DTSTART: nothing to test, keep it
				continue
			}
			if start.IsZero() || end.IsZero() {
				keep = true
				continue
			}
			if eventInWindow(&event, date, start, end) {
				keep = true
			}
		}
		if keep {
			out = append(out, obj)
		}
	}
	return out
}

// eventInWindow tests whether an event's DTSTART (or, for an annual
// recurrence, any occurrence) falls inside [start, end]. A single DTSTART is
// exact; an annually-recurring event like a birthday has its anchor year
// arbitrarily far from the queried window, so its month/day is tested across
// every year in the window instead.
func eventInWindow(event *ical.Event, dtstart, start, end time.Time) bool {
	if !dtstart.Before(start) && !dtstart.After(end) {
		return true
	}
	rrule, err := event.Props.Text(ical.PropRecurrenceRule)
	if err != nil || !strings.Contains(strings.ToUpper(rrule), "FREQ=YEARLY") {
		return false
	}
	// Cap the scan so a pathological multi-decade window stays cheap.
	for year := start.Year(); year <= end.Year() && year <= start.Year()+50; year++ {
		candidate := time.Date(year, dtstart.Month(), dtstart.Day(), 0, 0, 0, 0, time.UTC)
		if !candidate.Before(start) && !candidate.After(end) {
			return true
		}
	}
	return false
}

// PutCalendarObject is intentionally unsupported: T12b serves read-only. A
// T13-style write-back targets *subscribed* remote calendars via
// CalendarSyncService, not this endpoint.
func (b *Backend) PutCalendarObject(ctx context.Context, urlPath string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	return nil, fmt.Errorf("writing calendar objects is not supported")
}

// DeleteCalendarObject is intentionally unsupported (see PutCalendarObject).
func (b *Backend) DeleteCalendarObject(ctx context.Context, urlPath string) error {
	return fmt.Errorf("deleting calendar objects is not supported")
}

// extractUIDFromPath extracts the stable UID from a CalDAV path, e.g.
// /caldav/calendars/user/interactions/interaction-<uuid>.ics.
func extractUIDFromPath(urlPath string) string {
	trimmed := strings.TrimSuffix(urlPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	base := parts[len(parts)-1]
	base = strings.TrimSuffix(base, ".ics")
	if base == "" || base == "interactions" || base == calendarName {
		return ""
	}
	return base
}

// compile-time assertion that Backend satisfies caldav.Backend.
var _ caldav.Backend = (*Backend)(nil)
