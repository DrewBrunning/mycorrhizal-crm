package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetCalendar(t *testing.T) {
	b, ctx, user := newTestBackend(t)

	cal, err := b.GetCalendar(ctx, "/caldav/calendars/"+user.Username+"/interactions/")
	require.NoError(t, err)
	require.NotNil(t, cal)
	assert.Equal(t, "/caldav/calendars/"+user.Username+"/interactions/", cal.Path)
	assert.Equal(t, []string{ical.CompEvent}, cal.SupportedComponentSet)

	// Missing trailing slash also resolves.
	cal, err = b.GetCalendar(ctx, "/caldav/calendars/"+user.Username+"/interactions")
	require.NoError(t, err)
	require.NotNil(t, cal)

	// A different path is not found.
	_, err = b.GetCalendar(ctx, "/caldav/calendars/"+user.Username+"/other/")
	require.Error(t, err)

	// Another user's calendar path is not found.
	_, err = b.GetCalendar(ctx, "/caldav/calendars/bob/interactions/")
	require.Error(t, err)

	// No username in context: error.
	_, err = b.GetCalendar(context.Background(), "/caldav/calendars/"+user.Username+"/interactions/")
	require.Error(t, err)
}

func TestCreateDeleteCalendarUnsupported(t *testing.T) {
	b, ctx, user := newTestBackend(t)

	err := b.CreateCalendar(ctx, &caldav.Calendar{Path: "/caldav/calendars/" + user.Username + "/other/"})
	require.Error(t, err)

	err = b.DeleteCalendar(ctx, "/caldav/calendars/"+user.Username+"/other/")
	require.Error(t, err)
}

func TestHandlerDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	handler := NewHandler(db)
	h := handler.GinHandler()

	// A minimal, valid PROPFIND body requesting the current-user-principal.
	propfindBody := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:current-user-principal/>
    <D:resourcetype/>
  </D:prop>
</D:propfind>`

	cases := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		// Principal discovery endpoints are served by servePrincipal.
		{name: "principal PROPFIND", method: "PROPFIND", path: "/caldav/principals/alice/", wantCode: http.StatusMultiStatus},
		{name: "root PROPFIND", method: "PROPFIND", path: "/caldav/", wantCode: http.StatusMultiStatus},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(propfindBody))
			req.Header.Set("Depth", "0")
			req.Header.Set("Content-Type", "application/xml")
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Set("userID", uint(1))
			c.Set("username", "alice")
			h(c)
			assert.Equal(t, tc.wantCode, w.Code)
		})
	}
}

func TestWellKnownRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/.well-known/caldav", nil)

	WellKnownRedirect(c)

	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/caldav/", w.Header().Get("Location"))
}
