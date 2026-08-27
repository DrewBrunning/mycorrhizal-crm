package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"mycorrhizal/logger"
)

// TestRequestIDMiddleware_SeedsCorrelationID proves the request ID is bound as
// the correlation ID on the request's context.Context, so a handler that
// passes c.Request.Context() into background work or an outbound call shares
// one ID with the HTTP log lines (issue #425).
func TestRequestIDMiddleware_SeedsCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestIDMiddleware())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		seen = logger.CorrelationID(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "fixed-id-123")
	r.ServeHTTP(w, req)

	require.Equal(t, "fixed-id-123", seen)
	require.Equal(t, "fixed-id-123", w.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddleware_GeneratesWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestIDMiddleware())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		seen = logger.CorrelationID(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.NotEmpty(t, seen)
	require.Equal(t, w.Header().Get("X-Request-ID"), seen)
}
