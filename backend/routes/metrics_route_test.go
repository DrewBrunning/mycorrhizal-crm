package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The /metrics route is conditional on METRICS_TOKEN (issue #389), the same
// pattern as the OIDC login/callback routes — so it is deliberately absent
// from the six-persona authorization matrix and the OpenAPI drift test, both
// of which build the router from a config with no token. This test owns the
// conditional-registration + bearer-auth contract instead.

func metricsRouteRouter(t *testing.T, token string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := dbtest.New(t)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	cfg := &config.Config{MetricsToken: token, DBPath: t.TempDir() + "/x.db"}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(r, cfg, db, nil)
	return r
}

func hasRoute(r *gin.Engine, method, path string) bool {
	for _, ri := range r.Routes() {
		if ri.Method == method && ri.Path == path {
			return true
		}
	}
	return false
}

func TestMetricsRoute_NotRegisteredWithoutToken(t *testing.T) {
	r := metricsRouteRouter(t, "")

	assert.False(t, hasRoute(r, http.MethodGet, "/metrics"),
		"/metrics must not be registered when METRICS_TOKEN is unset")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMetricsRoute_RegisteredAndGatedWithToken(t *testing.T) {
	const token = "route-test-metrics-token-0123"
	r := metricsRouteRouter(t, token)

	require.True(t, hasRoute(r, http.MethodGet, "/metrics"))

	// no credential -> 401
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// correct credential -> 200 Prometheus text
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain; version=0.0.4")
	assert.Contains(t, w.Body.String(), "# TYPE http_requests_total counter")
}
