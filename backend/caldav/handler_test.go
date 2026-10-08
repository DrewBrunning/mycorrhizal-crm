package caldav

// Issue #1439: the CalDAV backend used to return a bare fmt.Errorf from every
// failure branch, which go-webdav's internal.ServeError maps to HTTP 500 — so
// a missing event read as "server is broken" and an unsupported write read as
// a server fault instead of a clean client error. CardDAV was fixed the same
// way under #874; these are the CalDAV wire-level counterparts, driven through
// the real go-webdav handlers over a real migrated schema (CLAUDE.md trap 1).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWireHandler builds a CalDAV handler over a real migrated schema seeded
// with one user and one Activity, and returns the pieces a wire test needs.
func newWireHandler(t *testing.T) (*Handler, models.User, models.Activity) {
	t.Helper()
	db := dbtest.New(t)

	user := models.User{Username: "wireuser", Password: "password123!A", Email: "wire@example.com"}
	require.NoError(t, db.Create(&user).Error)

	activity := models.Activity{
		UserID: user.ID,
		Title:  "Wire lunch",
		Date:   time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&activity).Error)

	return NewHandler(db), user, activity
}

// doCalDAV drives one request through the handler with the authenticated
// principal already in the Gin context (as the auth middleware leaves it).
func doCalDAV(t *testing.T, h gin.HandlerFunc, user models.User, method, target, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()

	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, target, nil)
	} else {
		req, _ = http.NewRequest(method, target, strings.NewReader(body))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", user.ID)
	c.Set("username", user.Username)
	h(c)
	return w
}

func TestHandlerGetCalendarObjectStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, user, activity := newWireHandler(t)
	h := handler.GinHandler()

	base := "/caldav/calendars/" + user.Username + "/interactions/"

	// A present event serves 200 with its iCalendar body.
	w := doCalDAV(t, h, user, http.MethodGet, base+"interaction-"+activity.UUID+".ics", "", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/calendar")
	assert.Contains(t, w.Body.String(), "BEGIN:VCALENDAR")
	assert.Contains(t, w.Body.String(), "SUMMARY:Wire lunch", "the 200 must carry the event itself")

	// A missing event is a 404 ("the event is gone"), not a 500 ("server broken").
	w = doCalDAV(t, h, user, http.MethodGet, base+"interaction-does-not-exist.ics", "", "")
	assert.Equal(t, http.StatusNotFound, w.Code,
		"GET of a missing event must be 404, not 500 (issue #1439)")
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
	assert.True(t, strings.HasPrefix(w.Body.String(), "404 Not Found"), w.Body.String())
	assert.NotContains(t, w.Body.String(), "BEGIN:VCALENDAR", "a 404 must not carry calendar data")

	// A path with no resource UID is a malformed request: 400.
	w = doCalDAV(t, h, user, http.MethodGet, base, "", "")
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"GET of the calendar collection path is an invalid path")
	assert.True(t, strings.HasPrefix(w.Body.String(), "400 Bad Request"), w.Body.String())
}

func TestHandlerWritePathsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, user, activity := newWireHandler(t)
	h := handler.GinHandler()

	target := "/caldav/calendars/" + user.Username + "/interactions/new-event.ics"
	existing := "/caldav/calendars/" + user.Username + "/interactions/interaction-" + activity.UUID + ".ics"
	icalBody := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//mycorrhizal//caldav-test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:wire-new\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T000000Z\r\nSUMMARY:New\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	// PUT is refused by design (read-only serve): a 403, not a 500.
	w := doCalDAV(t, h, user, http.MethodPut, target, icalBody, "text/calendar")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"PUT of a calendar object is unsupported and must be 403, not 500")
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
	assert.True(t, strings.HasPrefix(w.Body.String(), "403 Forbidden"), w.Body.String())

	// DELETE is refused by design (read-only serve): a 403, not a 500. Aim it at
	// a real event so a regression that executed it would be visible.
	w = doCalDAV(t, h, user, http.MethodDelete, existing, "", "")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"DELETE of a calendar object is unsupported and must be 403, not 500")
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
	assert.True(t, strings.HasPrefix(w.Body.String(), "403 Forbidden"), w.Body.String())

	// Neither refused write changed anything: the PUT created no event and the
	// DELETE removed none.
	w = doCalDAV(t, h, user, http.MethodGet, target, "", "")
	assert.Equal(t, http.StatusNotFound, w.Code, "the refused PUT must not have created the event")
	w = doCalDAV(t, h, user, http.MethodGet, existing, "", "")
	require.Equal(t, http.StatusOK, w.Code, "the refused DELETE must not have removed the event")
	assert.Contains(t, w.Body.String(), "SUMMARY:Wire lunch")
}

func TestHandlerCalendarQueryReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, user, activity := newWireHandler(t)
	h := handler.GinHandler()

	reportBody := `<?xml version="1.0" encoding="utf-8" ?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"/></C:comp-filter></C:filter>
</C:calendar-query>`

	w := doCalDAV(t, h, user, "REPORT",
		"/caldav/calendars/"+user.Username+"/interactions/", reportBody, "application/xml")

	require.Equal(t, http.StatusMultiStatus, w.Code)
	assert.Contains(t, w.Body.String(), "interaction-"+activity.UUID+".ics",
		"a calendar-query REPORT must return the served event over the wire")
}

// TestGetCalendarObjectDatabaseErrorsStay500 pins the other half of the
// mapping rule: a genuine server-side failure is not a missing event, so it
// must stay a 500 rather than being misreported as a 404. The table is hidden
// with the real-schema error seam (CLAUDE.md trap 1).
func TestGetCalendarObjectDatabaseErrorsStay500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := dbtest.New(t)
	user := models.User{Username: "dberruser", Password: "password123!A", Email: "dberr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dbtest.HideTable(t, db, "activities")
	dbtest.HideTable(t, db, "life_events")

	h := NewHandler(db).GinHandler()
	base := "/caldav/calendars/" + user.Username + "/interactions/"

	w := doCalDAV(t, h, user, http.MethodGet, base+"interaction-whatever.ics", "", "")
	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a DB failure on the activity lookup is a server fault, not a 404")

	w = doCalDAV(t, h, user, http.MethodGet, base+"life-event-1.ics", "", "")
	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a DB failure on the life-event lookup is a server fault, not a 404")
}

// TestUnauthenticatedMapsToHTTP401 pins every auth-error branch. They cannot
// be driven through the Gin handler (which type-asserts the middleware-provided
// userID), so this asserts the webdav error each backend method returns
// carries the 401 status rather than falling through to a 500.
func TestUnauthenticatedMapsToHTTP401(t *testing.T) {
	b := NewBackend(dbtest.New(t))
	ctx := context.Background() // no principal in context

	checks := []struct {
		name string
		call func() error
	}{
		{"CurrentUserPrincipal", func() error { _, err := b.CurrentUserPrincipal(ctx); return err }},
		{"CalendarHomeSetPath", func() error { _, err := b.CalendarHomeSetPath(ctx); return err }},
		{"ListCalendars", func() error { _, err := b.ListCalendars(ctx); return err }},
		{"GetCalendar", func() error {
			_, err := b.GetCalendar(ctx, "/caldav/calendars/whoever/interactions/")
			return err
		}},
		{"ListCalendarObjects", func() error {
			_, err := b.ListCalendarObjects(ctx, "/caldav/calendars/whoever/interactions/", nil)
			return err
		}},
		{"GetCalendarObject", func() error {
			_, err := b.GetCalendarObject(ctx, "/caldav/calendars/whoever/interactions/interaction-x.ics", nil)
			return err
		}},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "401 Unauthorized",
				"a missing principal must map to 401, not 500")
		})
	}
}
