package soak

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleExposition = `# HELP go_goroutines Number of goroutines.
# TYPE go_goroutines gauge
go_goroutines 17
process_resident_memory_bytes 5.5e+07
process_open_fds 48
go_memstats_heap_inuse_bytes 7000000
db_connections_open 2
http_requests_in_flight 1
mycorrhizal_storage_bytes{kind="database"} 6000000
mycorrhizal_storage_bytes{kind="wal"} 4100000
mycorrhizal_storage_bytes{kind="other"} 1
mycorrhizal_ratelimiter_entries{limiter="api"} 3
mycorrhizal_ratelimiter_entries{limiter="account"} 4
http_requests_total{method="GET",route="/api/v1/contacts",status="200"} 90
http_requests_total{method="POST",route="/api/v1/contacts",status="500"} 2
http_requests_total{method="POST",route="/api/v1/contacts",status="503"} 1
job_runs_total{job="a",result="success"} 5
job_runs_total{job="b",result="failure"} 1
http_request_duration_seconds_bucket{method="GET",route="/api/v1/contacts",le="0.005"} 8
http_request_duration_seconds_bucket{method="GET",route="/api/v1/contacts",le="0.1"} 10
http_request_duration_seconds_bucket{method="GET",route="/api/v1/contacts",le="+Inf"} 10
http_request_duration_seconds_bucket{method="POST",route="/api/v1/login",le="bad"} 10
`

func TestParseExposition_ParsesSamplesAndSkipsComments(t *testing.T) {
	ms, err := ParseExposition(strings.NewReader(sampleExposition))
	require.NoError(t, err)
	var found bool
	for _, m := range ms {
		if m.Name == "mycorrhizal_storage_bytes" && m.Labels["kind"] == "wal" {
			found = true
			assert.Equal(t, 4100000.0, m.Value)
		}
		if m.Name == "http_request_duration_seconds_bucket" && m.Labels["le"] == "+Inf" {
			assert.Equal(t, 10.0, m.Value)
		}
	}
	assert.True(t, found)
}

func TestParseExposition_LabelEscapesAndTimestamps(t *testing.T) {
	ms, err := ParseExposition(strings.NewReader("m{a=\"x\\\"y\",b=\"l1\\nl2\\\\\"} 3 1700000000\nm2 -Inf\nm3 +Inf\n"))
	require.NoError(t, err)
	require.Len(t, ms, 3)
	assert.Equal(t, "x\"y", ms[0].Labels["a"])
	assert.Equal(t, "l1\nl2\\", ms[0].Labels["b"])
	assert.Equal(t, 3.0, ms[0].Value, "a trailing timestamp is ignored")
	assert.True(t, math.IsInf(ms[1].Value, -1))
	assert.True(t, math.IsInf(ms[2].Value, 1))
}

func TestParseExposition_MalformedLinesAreErrors(t *testing.T) {
	for name, in := range map[string]string{
		"no value":          "just_a_name\n",
		"unbalanced braces": "m{a=\"x\" 1\n",
		"unquoted label":    "m{a=x} 1\n",
		"unterminated":      "m{a=\"x} 1\n",
		"no equals":         "m{ax} 1\n",
		"bad number":        "m 12abc\n",
		"missing value":     "m{a=\"x\"}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseExposition(strings.NewReader(in))
			assert.Error(t, err)
		})
	}
}

func TestReduce_MapsFamiliesToSignals(t *testing.T) {
	ms, err := ParseExposition(strings.NewReader(sampleExposition))
	require.NoError(t, err)
	snap := Reduce(ms)
	assert.Equal(t, 17.0, snap.Values[SigGoroutines])
	assert.Equal(t, 5.5e7, snap.Values[SigRSS])
	assert.Equal(t, 48.0, snap.Values[SigOpenFDs])
	assert.Equal(t, 7e6, snap.Values[SigHeapInuse])
	assert.Equal(t, 2.0, snap.Values[SigDBConnsOpen])
	assert.Equal(t, 1.0, snap.Values[SigInFlight])
	assert.Equal(t, 6e6, snap.Values[SigDBBytes])
	assert.Equal(t, 4.1e6, snap.Values[SigWALBytes])
	assert.Equal(t, 7.0, snap.Values[SigLimiterEntries], "limiter gauges sum across limiters")
	assert.Equal(t, 3.0, snap.Values[SigServerErrors], "every 5xx status counts")
	assert.Equal(t, 1.0, snap.Values[SigJobFailures])
	assert.Equal(t, 5.0, snap.Values[SigJobSuccesses])
	assert.Contains(t, snap.hist, "read")
}

func TestRouteClass(t *testing.T) {
	assert.Equal(t, "auth", routeClass("POST", "/api/v1/login"))
	assert.Equal(t, "auth", routeClass("POST", "/api/v1/logout"))
	assert.Equal(t, "auth", routeClass("POST", "/api/v1/register"))
	assert.Equal(t, "read", routeClass("GET", "/api/v1/contacts"))
	assert.Equal(t, "read", routeClass("HEAD", "/api/v1/contacts"))
	assert.Equal(t, "write", routeClass("DELETE", "/api/v1/contacts/:id"))
	assert.Equal(t, SigP95Auth, p95Signal("auth"))
	assert.Equal(t, SigP95Write, p95Signal("write"))
	assert.Equal(t, SigP95Read, p95Signal("read"))
}

func snapWith(class string, h histogram) Snapshot {
	return Snapshot{Values: map[string]float64{}, hist: map[string]histogram{class: h}}
}

func TestP95Tracker_EmitsOnceEnoughRequestsAccumulate(t *testing.T) {
	inf := math.Inf(1)
	base := snapWith("auth", histogram{0.05: 0, 1: 0, inf: 0})
	tr := NewP95Tracker(base)

	// 3 requests: below the minimum, no point, anchor not advanced.
	assert.Empty(t, tr.Update(snapWith("auth", histogram{0.05: 3, 1: 3, inf: 3})))

	// 12 requests since the anchor, 11 of them <= 0.05s: p95 falls in 1s? 95% of
	// 12 = 11.4, so the 0.05 bucket (11) is not enough; the 1s bucket is.
	got := tr.Update(snapWith("auth", histogram{0.05: 11, 1: 12, inf: 12}))
	assert.Equal(t, 1.0, got["auth"])

	// Anchor advanced: the next 4 requests are again below the minimum.
	assert.Empty(t, tr.Update(snapWith("auth", histogram{0.05: 15, 1: 16, inf: 16})))
}

func TestHistP95_EdgeCases(t *testing.T) {
	inf := math.Inf(1)
	_, ok := histP95(nil, histogram{})
	assert.False(t, ok, "an empty histogram has no p95")

	// The 95th percentile lands past the last finite bucket: report that
	// bucket's bound rather than +Inf so the series stays finite.
	p, ok := histP95(nil, histogram{0.1: 5, inf: 20})
	require.True(t, ok)
	assert.Equal(t, 0.1, p)

	// Only the +Inf bucket exists.
	p, ok = histP95(nil, histogram{inf: 20})
	require.True(t, ok)
	assert.Zero(t, p)

	// Everything in the first bucket.
	p, ok = histP95(nil, histogram{0.005: 20, inf: 20})
	require.True(t, ok)
	assert.Equal(t, 0.005, p)
}

func TestScraper_Scrape(t *testing.T) {
	var gotAuth string
	var prepared int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(sampleExposition))
		case "/bad":
			_, _ = w.Write([]byte("garbage line\n"))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	sc := &Scraper{Client: srv.Client(), URL: srv.URL + "/ok", Token: "tok", prepare: func() { prepared++ }}
	snap, err := sc.Scrape(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok", gotAuth)
	assert.Equal(t, 1, prepared, "the pre-scrape hook (in-process GC) runs once per scrape")
	assert.Equal(t, 17.0, snap.Values[SigGoroutines])

	sc.URL = srv.URL + "/denied"
	_, err = sc.Scrape(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "METRICS_TOKEN")

	sc.URL = srv.URL + "/bad"
	_, err = sc.Scrape(context.Background())
	assert.Error(t, err)

	sc.URL = "http://127.0.0.1:1/unreachable"
	_, err = sc.Scrape(context.Background())
	assert.Error(t, err)

	sc.URL = "://bad url"
	_, err = sc.Scrape(context.Background())
	assert.Error(t, err)
}
