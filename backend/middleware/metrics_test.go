package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

// sample returns the value of the exposition line that starts with series
// (name plus label set), or 0 when the series has not been emitted yet.
//
// The registry behind scrape is the process-global metrics.Default(), so any
// earlier test in the package that requests the same route leaves its counts
// behind (issue #1492: order-dependent under -shuffle). Tests therefore assert
// the DELTA across their own requests, never an absolute counter value.
func sample(t *testing.T, series string) float64 {
	t.Helper()
	for _, line := range strings.Split(scrape(t), "\n") {
		if rest, ok := strings.CutPrefix(line, series+" "); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			require.NoError(t, err, "unparseable sample %q", line)
			return v
		}
	}
	return 0
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
	const (
		thing    = `http_requests_total{method="GET",route="/mw/thing/:id",status="200"}`
		boom     = `http_requests_total{method="GET",route="/mw/boom",status="500"}`
		thingDur = `http_request_duration_seconds_count{method="GET",route="/mw/thing/:id"}`
	)
	thing0, boom0, dur0 := sample(t, thing), sample(t, boom), sample(t, thingDur)

	r := metricsTestRouter()
	do(r, http.MethodGet, "/mw/thing/1")
	do(r, http.MethodGet, "/mw/thing/2") // different concrete id, same template
	do(r, http.MethodGet, "/mw/boom")

	assert.Equal(t, 2.0, sample(t, thing)-thing0)
	assert.Equal(t, 1.0, sample(t, boom)-boom0)
	assert.Equal(t, 2.0, sample(t, thingDur)-dur0)
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
