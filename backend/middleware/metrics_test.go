package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/metrics"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scrape(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	require.NoError(t, metrics.Default().WritePrometheus(&sb))
	return sb.String()
}

func metricsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MetricsMiddleware())
	r.GET("/mw/thing/:id", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/mw/boom", func(c *gin.Context) { c.String(http.StatusInternalServerError, "no") })
	return r
}

func do(r http.Handler, method, path string) {
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
}

func TestMetricsMiddleware_CountsByRouteTemplateAndStatus(t *testing.T) {
	r := metricsTestRouter()
	do(r, http.MethodGet, "/mw/thing/1")
	do(r, http.MethodGet, "/mw/thing/2") // different concrete id, same template
	do(r, http.MethodGet, "/mw/boom")

	out := scrape(t)
	assert.Contains(t, out, `http_requests_total{method="GET",route="/mw/thing/:id",status="200"} 2`+"\n")
	assert.Contains(t, out, `http_requests_total{method="GET",route="/mw/boom",status="500"} 1`+"\n")
	assert.Contains(t, out, `http_request_duration_seconds_count{method="GET",route="/mw/thing/:id"} 2`+"\n")
}

func TestMetricsMiddleware_UnmatchedRouteIsLabelledUnmatched(t *testing.T) {
	r := metricsTestRouter()
	do(r, http.MethodGet, "/no/such/path/"+t.Name())

	out := scrape(t)
	assert.Contains(t, out, `http_requests_total{method="GET",route="unmatched",status="404"} `)
	assert.NotContains(t, out, "/no/such/path/")
}

func TestMetricsMiddleware_InFlightReturnsToZero(t *testing.T) {
	r := metricsTestRouter()
	do(r, http.MethodGet, "/mw/thing/9")
	assert.Contains(t, scrape(t), "\nhttp_requests_in_flight 0\n")
}
