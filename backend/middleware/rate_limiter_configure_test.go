package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// TestConfigureAPIRateLimiter pins that ConfigureAPIRateLimiter replaces the
// package-level general-API limiter with one built from the supplied settings,
// and that zero/negative values are ignored (a partially-set environment keeps
// the safe defaults rather than disabling rate limiting outright).
func TestConfigureAPIRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	before := apiLimiter
	// Restore the original limiter even if an assertion fails mid-test, so a
	// failure here can never poison later tests (TestAPIRateLimitMiddleware
	// assumes the lenient default burst of 1000).
	defer func() { apiLimiter = before }()

	// Zero values must not touch the existing limiter.
	ConfigureAPIRateLimiter(0, 0)
	if apiLimiter != before {
		t.Error("ConfigureAPIRateLimiter(0,0) must leave the global limiter untouched")
	}

	// Negative values likewise.
	ConfigureAPIRateLimiter(-time.Second, -1)
	if apiLimiter != before {
		t.Error("ConfigureAPIRateLimiter(negative) must leave the global limiter untouched")
	}

	// A valid call replaces it with a limiter carrying the new settings.
	ConfigureAPIRateLimiter(time.Second, 3)
	if apiLimiter == before {
		t.Fatal("ConfigureAPIRateLimiter(valid) must replace the global limiter")
	}
	if apiLimiter.r != rate.Every(time.Second) {
		t.Errorf("rate = %v, want rate.Every(time.Second)", apiLimiter.r)
	}
	if apiLimiter.b != 3 {
		t.Errorf("burst = %d, want 3", apiLimiter.b)
	}
}

// TestCardDAVRateLimitMiddleware exercises CardDAVRateLimitMiddleware's
// returned middleware: a burst-sized run of requests from one IP must all pass
// (bulk sync from a client like vdirsyncer is the exact use case the higher
// CardDAV burst exists for). The 429 branch is exercised against
// RateLimitMiddleware directly in TestRateLimitMiddleware_BlocksExcessiveRequests,
// so it is not re-proven here by mutating the shared global.
func TestCardDAVRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CardDAVRateLimitMiddleware())
	router.GET("/carddav/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Well under the 2500 burst — all pass.
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/carddav/test", nil)
		req.RemoteAddr = "192.168.9.1:1234"
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d got %d, want 200 — CardDAV burst should absorb bulk sync", i+1, w.Code)
		}
	}
}
