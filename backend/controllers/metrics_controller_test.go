package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMetricsToken = "metrics-token-abcdefghij" // >= 16 chars

func metricsRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := dbtest.New(t)
	cfg := &config.Config{MetricsToken: testMetricsToken, DBPath: t.TempDir() + "/x.db"}
	r := gin.New()
	r.GET("/metrics", MetricsHandler(cfg, db))
	return r
}

func getMetrics(t *testing.T, r http.Handler, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMetricsHandler_RejectsMissingOrBadToken(t *testing.T) {
	r := metricsRouter(t)

	cases := map[string]string{
		"no header":            "",
		"wrong token":          "Bearer not-the-token-at-all-xxxx",
		"wrong scheme":         "Basic " + testMetricsToken,
		"prefix of real token": "Bearer " + testMetricsToken[:len(testMetricsToken)-1],
		"real token + suffix":  "Bearer " + testMetricsToken + "x",
	}
	for name, auth := range cases {
		t.Run(name, func(t *testing.T) {
			w := getMetrics(t, r, auth)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, w.Body.String())
		})
	}
}

func TestMetricsHandler_ServesExpositionWithValidToken(t *testing.T) {
	r := metricsRouter(t)

	w := getMetrics(t, r, "Bearer "+testMetricsToken)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; version=0.0.4; charset=utf-8", w.Header().Get("Content-Type"))

	body := w.Body.String()
	for _, want := range []string{
		"# TYPE http_requests_total counter",
		"go_goroutines ",
		"mycorrhizal_build_info{version=",
		"db_connections_open ",
		`mycorrhizal_storage_bytes{kind="database"}`,
		"process_uptime_seconds ",
	} {
		assert.Contains(t, body, want)
	}
}

func TestMetricsHandler_CaseInsensitiveBearerScheme(t *testing.T) {
	r := metricsRouter(t)
	w := getMetrics(t, r, "bEaReR   "+testMetricsToken)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMetricsHandler_ReflectsSystemEventCounters(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{MetricsToken: testMetricsToken, DBPath: t.TempDir() + "/x.db"}
	r := gin.New()
	r.GET("/metrics", MetricsHandler(cfg, db))

	models.RecordSystemEvent(context.Background(), db, models.SystemEvent{
		EventType: models.SysEventSyncFailed,
		Component: "contact_sync",
		Result:    models.SysResult("failure"),
	})

	w := getMetrics(t, r, "Bearer "+testMetricsToken)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(),
		`system_events_total{event_type="sync_failed",component="contact_sync",result="failure"}`)
}

func TestBearerToken(t *testing.T) {
	assert.Equal(t, "", bearerToken(""))
	assert.Equal(t, "", bearerToken("Bearer"))
	assert.Equal(t, "", bearerToken("Basic abc"))
	assert.Equal(t, "abc", bearerToken("Bearer abc"))
	assert.Equal(t, "abc", bearerToken("bearer   abc  "))
}
