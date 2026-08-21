package carddav

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHandlerDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Contact{}))

	handler := NewHandler(db, t.TempDir())
	h := handler.GinHandler()

	propfindBody := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:current-user-principal/>
    <D:resourcetype/>
  </D:prop>
</D:propfind>`

	cases := []struct {
		name     string
		path     string
		wantCode int
	}{
		{name: "principal PROPFIND", path: "/carddav/principals/alice/", wantCode: http.StatusMultiStatus},
		{name: "root PROPFIND", path: "/carddav/", wantCode: http.StatusMultiStatus},
		{name: "bare root", path: "/carddav", wantCode: http.StatusMultiStatus},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("PROPFIND", tc.path, strings.NewReader(propfindBody))
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
	c.Request, _ = http.NewRequest(http.MethodGet, "/.well-known/carddav", nil)

	WellKnownRedirect(c)

	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/carddav/", w.Header().Get("Location"))
}

func TestHandlerDelegatesToWebDAV(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Contact{}))

	handler := NewHandler(db, t.TempDir())
	h := handler.GinHandler()

	// A non-principal, non-root path falls through to go-webdav's own handler.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PROPFIND", "/carddav/addressbooks/alice/contacts/", strings.NewReader(
		`<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>`))
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", uint(1))
	c.Set("username", "alice")
	h(c)

	// go-webdav returns 207 Multi-Status for a valid addressbook PROPFIND.
	assert.Equal(t, http.StatusMultiStatus, w.Code)
}
