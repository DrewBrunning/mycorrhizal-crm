package routes

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testClientIPSeq hands out a distinct client IP to every request this
// package's tests issue. The auth rate limiter (middleware/rate_limiter.go) is
// a process-wide bucket keyed by client IP, and gin's ClientIP() returns ""
// for a request whose RemoteAddr is empty — which http.NewRequest leaves it —
// so all such requests share a single bucket regardless of any
// X-Forwarded-For they carry. A test that mints many sessions then exhausts
// the shared burst and the next test gets a spurious 429 (issue #1186). Giving
// every request its own source IP puts it in its own bucket, so an outcome
// never depends on how many auth requests earlier tests made.
var testClientIPSeq atomic.Uint32

// uniqueTestClientIP returns a fresh, never-reused client IP for one test
// request. Assign it to req.RemoteAddr (with a port) before ServeHTTP.
func uniqueTestClientIP() string {
	n := testClientIPSeq.Add(1)
	return net.IPv4(10, byte(n>>16), byte(n>>8), byte(n)).String()
}

func testConfig() *config.Config {
	return &config.Config{
		DBPath:           ":memory:",
		JWTSecretKey:     "test-secret-key-that-is-long-enough-32",
		ProfilePhotoDir:  "/tmp/test-photos",
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		JWTExpiryHours:   96,
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

// TestRegisterRoutes_NoPanic verifies that route registration does not panic
// or reference nil functions. This catches regressions where a controller
// function is renamed or removed but the route registration is not updated —
// a problem that Go compilation alone cannot detect when the controller
// function is passed by reference at the wire-up site in routes.go.
func TestRegisterRoutes_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()

	assert.NotPanics(t, func() {
		RegisterRoutes(router, cfg, db, nil)
	}, "RegisterRoutes must not panic with a basic config and no OIDC provider")
}

// TestRegisterRoutes_RouteCountGuardsAgainstAccidentalDeletion asserts a
// minimum route count so that accidentally removing a route registration
// line is noticed. The count is deliberately a floor, not an exact match,
// so that adding new routes doesn't break this test — only removal does.
func TestRegisterRoutes_RouteCountGuardsAgainstAccidentalDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	RegisterRoutes(router, cfg, db, nil)

	routes := router.Routes()
	assert.GreaterOrEqual(t, len(routes), 80,
		"unexpectedly low route count — an entire route group may have been accidentally deleted")
}

// TestWellKnownDAVDiscovery_AcceptsPROPFIND pins issue #917's live interop
// finding: a real DAVx5 client issues PROPFIND (not GET) directly against
// the /.well-known/{carddav,caldav} URIs during account autodiscovery, and
// expects the same redirect GET gets. The handler itself (WellKnownRedirect)
// is method-agnostic, so a unit test that calls it directly can't catch
// this — the bug was the router.GET-only registration. This test goes
// through the real router, the way the client does.
func TestWellKnownDAVDiscovery_AcceptsPROPFIND(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	cfg.CardDAVEnabled = true
	cfg.CalDAVEnabled = true
	RegisterRoutes(router, cfg, db, nil)

	cases := []struct {
		path         string
		wantLocation string
	}{
		{"/.well-known/carddav", "/carddav/"},
		{"/.well-known/caldav", "/caldav/"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("PROPFIND", tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusMovedPermanently, w.Code,
				"PROPFIND to %s must redirect like GET does, not 404", tc.path)
			assert.Equal(t, tc.wantLocation, w.Header().Get("Location"))
		})
	}
}
