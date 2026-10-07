package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/internal/clock"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1494: RegisterRoutes puts the system clock on every request, and a
// clock a test pre-installed on the router is never overwritten.
func TestClockMiddleware_DefaultsToSystemAndKeepsInstalled(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	pinned := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	var seen clock.Clock
	probe := func(c *gin.Context) { seen = clock.FromContext(c); c.Status(http.StatusNoContent) }

	// No clock installed -> the system clock.
	plain := gin.New()
	plain.Use(clockMiddleware())
	plain.GET("/p", probe)
	w := httptest.NewRecorder()
	plain.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	_, isSystem := seen.(clock.System)
	assert.True(t, isSystem, "default clock is clock.System")
	assert.WithinDuration(t, time.Now(), seen.Now(), time.Second)

	// A Fake installed first survives the production wiring.
	faked := gin.New()
	faked.Use(func(c *gin.Context) { clock.Install(c, clock.NewFake(pinned)); c.Next() })
	faked.Use(clockMiddleware())
	faked.GET("/p", probe)
	w = httptest.NewRecorder()
	faked.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, seen.Now().Equal(pinned), "the pre-installed Fake is kept")
}
