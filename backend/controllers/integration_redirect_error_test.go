package controllers

import (
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A redirect answer from any per-user integration is a clear 503 about the base
// URL, never the generic "could not reach" fallback.
func TestAbortIntegrationServiceError_RedirectIsAClearError(t *testing.T) {
	cases := map[string]struct {
		abort func(c *gin.Context, err error)
		err   error
	}{
		"immich":    {abortImmichServiceError, services.ErrImmichRedirect},
		"paperless": {abortPaperlessServiceError, services.ErrPaperlessRedirect},
		"seafile":   {abortSeafileServiceError, services.ErrSeafileRedirect},
		"nextcloud": {abortWebDAVServiceError, services.ErrWebDAVRedirect},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.ReleaseMode)
			router := gin.New()
			router.GET("/x", func(c *gin.Context) { tc.abort(c, tc.err) })
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/x", nil)
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "redirect")
			assert.Contains(t, w.Body.String(), "base URL")
		})
	}
}
