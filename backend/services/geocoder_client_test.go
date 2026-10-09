package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/config"
)

// plainTransport is what tests hand newGeocoderClient: httptest servers listen
// on loopback, which the production SSRF-guarded transport (rightly) refuses.
func plainTransport() http.RoundTripper { return http.DefaultTransport }

func nominatimClient(url string) *GeocoderClient {
	return newGeocoderClient(config.GeocoderProviderNominatim, "", url, plainTransport(), nil)
}

func maptilerClient(url, key string) *GeocoderClient {
	return newGeocoderClient(config.GeocoderProviderMapTiler, key, url, plainTransport(), nil)
}

func TestGeocoderClient_NominatimSuccessAndRequestShape(t *testing.T) {
	var gotPath, gotUA, gotAccept string
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA, gotAccept = r.URL.Path, r.UserAgent(), r.Header.Get("Accept")
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[{"lat":"51.5007292","lon":"-0.1246254","display_name":"Big Ben"}]`))
	}))
	t.Cleanup(srv.Close)

	res, err := nominatimClient(srv.URL).Geocode(context.Background(), "  Westminster, London  ")
	require.NoError(t, err)
	assert.InDelta(t, 51.5007292, res.Lat, 1e-9)
	assert.InDelta(t, -0.1246254, res.Lon, 1e-9)

	assert.Equal(t, "/search", gotPath)
	assert.Equal(t, []string{"Westminster, London"}, gotQuery["q"], "query is trimmed and sent whole")
	assert.Equal(t, []string{"jsonv2"}, gotQuery["format"])
	assert.Equal(t, []string{"1"}, gotQuery["limit"], "exactly one result is requested")
	assert.True(t, strings.HasPrefix(gotUA, "Mycorrhizal-CRM/"), "Nominatim's policy requires an identifying User-Agent, got %q", gotUA)
	assert.Equal(t, "application/json", gotAccept)
	assert.Empty(t, gotQuery["key"], "no API key is ever sent to nominatim")
}

func TestGeocoderClient_MapTilerSuccessAndRequestShape(t *testing.T) {
	var gotRawPath, gotKey, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		gotKey, gotLimit = r.URL.Query().Get("key"), r.URL.Query().Get("limit")
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","features":[{"center":[16.3695,48.201],"geometry":{"type":"Point","coordinates":[1,2]}}]}`))
	}))
	t.Cleanup(srv.Close)

	res, err := maptilerClient(srv.URL, "sekret").Geocode(context.Background(), "Stephansplatz 1/2, Wien")
	require.NoError(t, err)
	assert.InDelta(t, 48.201, res.Lat, 1e-9, "MapTiler returns [lon, lat]")
	assert.InDelta(t, 16.3695, res.Lon, 1e-9)
	assert.Equal(t, "sekret", gotKey)
	assert.Equal(t, "1", gotLimit)
	assert.Equal(t, "/geocoding/Stephansplatz%201%2F2%2C%20Wien.json", gotRawPath,
		"the address is path-escaped, so a slash in it cannot change the route")
}

func TestGeocoderClient_MapTilerFallsBackToPointGeometry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":[{"geometry":{"type":"Point","coordinates":[16.5,48.25]}}]}`))
	}))
	t.Cleanup(srv.Close)

	res, err := maptilerClient(srv.URL, "k").Geocode(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, GeocodeResult{Lat: 48.25, Lon: 16.5}, res)
}

func TestGeocoderClient_NoMatchIsDistinctFromFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		build func(url string) *GeocoderClient
		body  string
	}{
		"nominatim empty array": {nominatimClient, `[]`},
		"maptiler no features":  {func(u string) *GeocoderClient { return maptilerClient(u, "k") }, `{"features":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			t.Cleanup(srv.Close)
			_, err := tc.build(srv.URL).Geocode(context.Background(), "nowhere")
			assert.ErrorIs(t, err, ErrGeocoderNoResult)
		})
	}
}

func TestGeocoderClient_EmptyQueryNeverCallsProvider(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	t.Cleanup(srv.Close)

	_, err := nominatimClient(srv.URL).Geocode(context.Background(), "   \t ")
	assert.ErrorIs(t, err, ErrGeocoderNoResult)
	assert.Zero(t, atomic.LoadInt32(&hits))
}

func TestGeocoderClient_OverlongQueryIsTruncated(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	_, _ = nominatimClient(srv.URL).Geocode(context.Background(), strings.Repeat("é", maxGeocoderQueryRunes+50))
	assert.Equal(t, maxGeocoderQueryRunes, len([]rune(got)), "truncated on a rune boundary, not mid-character")
}

// TestGeocoderClient_MalformedBodiesAreInvalidData: nothing is inferred from a
// bad body — a hostile or broken upstream cannot get a bogus coordinate stored.
func TestGeocoderClient_MalformedBodiesAreInvalidData(t *testing.T) {
	cases := map[string]struct {
		provider string
		body     string
	}{
		"nominatim not json":             {config.GeocoderProviderNominatim, `<html>`},
		"nominatim object not array":     {config.GeocoderProviderNominatim, `{"lat":"1","lon":"2"}`},
		"nominatim non-numeric lat":      {config.GeocoderProviderNominatim, `[{"lat":"north","lon":"2"}]`},
		"nominatim non-numeric lon":      {config.GeocoderProviderNominatim, `[{"lat":"1","lon":"east"}]`},
		"nominatim lat out of range":     {config.GeocoderProviderNominatim, `[{"lat":"91","lon":"2"}]`},
		"nominatim lon out of range":     {config.GeocoderProviderNominatim, `[{"lat":"1","lon":"-181"}]`},
		"nominatim NaN":                  {config.GeocoderProviderNominatim, `[{"lat":"NaN","lon":"2"}]`},
		"nominatim missing fields":       {config.GeocoderProviderNominatim, `[{}]`},
		"maptiler not json":              {config.GeocoderProviderMapTiler, `nope`},
		"maptiler feature without point": {config.GeocoderProviderMapTiler, `{"features":[{"geometry":{"type":"Polygon","coordinates":[]}}]}`},
		"maptiler single-ordinate":       {config.GeocoderProviderMapTiler, `{"features":[{"center":[16.3]}]}`},
		"maptiler lat out of range":      {config.GeocoderProviderMapTiler, `{"features":[{"center":[16.3,95]}]}`},
		"maptiler lon out of range":      {config.GeocoderProviderMapTiler, `{"features":[{"center":[200,40]}]}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			t.Cleanup(srv.Close)
			c := newGeocoderClient(tc.provider, "k", srv.URL, plainTransport(), nil)
			_, err := c.Geocode(context.Background(), "1 Main St")
			assert.ErrorIs(t, err, ErrGeocoderInvalidData)
		})
	}
}

func TestGeocoderClient_RateLimitedIsDistinct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	_, err := nominatimClient(srv.URL).Geocode(context.Background(), "x")
	assert.ErrorIs(t, err, ErrGeocoderRateLimited)
}

func TestGeocoderClient_UnexpectedStatusCarriesStatusNotBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal secret detail"))
	}))
	t.Cleanup(srv.Close)

	_, err := nominatimClient(srv.URL).Geocode(context.Background(), "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGeocoderRequestFailed)
	var re *GeocoderRequestError
	require.True(t, errors.As(err, &re))
	assert.Equal(t, http.StatusInternalServerError, re.StatusCode)
	assert.NotContains(t, err.Error(), "internal secret detail", "the provider's body is diagnostic-only, never returned")
}

func TestGeocoderClient_RedirectsAreNotFollowed(t *testing.T) {
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte(`[{"lat":"1","lon":"2"}]`))
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/search?q=x", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	_, err := maptilerClient(redirector.URL, "sekret").Geocode(context.Background(), "x")
	require.ErrorIs(t, err, ErrGeocoderRequestFailed)
	var reqErr *GeocoderRequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusFound, reqErr.StatusCode, "the 302 is surfaced as an unexpected status, not followed")
	assert.Zero(t, atomic.LoadInt32(&targetHits), "a redirect must never carry the (key-bearing) request elsewhere")
}

func TestGeocoderClient_OversizedBodyIsInvalidData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"lat":"1","lon":"2","pad":"`))
		_, _ = w.Write([]byte(strings.Repeat("a", maxGeocoderBodyBytes+10)))
		_, _ = w.Write([]byte(`"}]`))
	}))
	t.Cleanup(srv.Close)

	_, err := nominatimClient(srv.URL).Geocode(context.Background(), "x")
	assert.ErrorIs(t, err, ErrGeocoderInvalidData, "the read is bounded; a truncated body cannot parse")
}

// TestGeocoderClient_APIKeyNeverAppearsInErrors: the MapTiler key rides in the
// query string, and net/http's *url.Error prints the whole URL.
func TestGeocoderClient_APIKeyNeverAppearsInErrors(t *testing.T) {
	const key = "SUPER-SECRET-KEY-123"

	t.Run("connection refused", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		require.NoError(t, ln.Close())

		_, err = maptilerClient("http://"+addr, key).Geocode(context.Background(), "1 Main St")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGeocoderUnreachable)
		assert.NotContains(t, err.Error(), key)
		assert.NotContains(t, err.Error(), "key=")
	})

	t.Run("timeout", func(t *testing.T) {
		orig := geocoderRequestTimeout
		geocoderRequestTimeout = 150 * time.Millisecond
		t.Cleanup(func() { geocoderRequestTimeout = orig })

		_, err := maptilerClient(newBlackHoleServer(t), key).Geocode(context.Background(), "1 Main St")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGeocoderUnreachable)
		assert.NotContains(t, err.Error(), key)
	})
}

func TestNewGeocoderClient(t *testing.T) {
	c, err := NewGeocoderClient(config.GeocoderProviderNominatim, "ignored")
	require.NoError(t, err)
	assert.Equal(t, config.GeocoderProviderNominatim, c.Provider())
	assert.Equal(t, nominatimBaseURL, c.baseURL)
	assert.Same(t, nominatimGate, c.gate, "nominatim shares the process-wide 1 req/s gate")
	assert.Empty(t, c.apiKey, "a key set for another provider is not carried to nominatim")

	c, err = NewGeocoderClient(config.GeocoderProviderMapTiler, "k")
	require.NoError(t, err)
	assert.Equal(t, maptilerBaseURL, c.baseURL)
	assert.Nil(t, c.gate, "maptiler has no client-side ceiling")

	_, err = NewGeocoderClient(config.GeocoderProviderMapTiler, "  ")
	assert.ErrorIs(t, err, ErrGeocoderUnauthorized)

	for _, p := range []string{config.GeocoderProviderNone, "", "google"} {
		_, err = NewGeocoderClient(p, "k")
		assert.ErrorIs(t, err, ErrGeocoderDisabled, "provider %q", p)
	}
}

// TestGeocoderClient_ProductionTransportIsSSRFGuarded: clients built by
// NewGeocoderClient dial only through httputil.SafeDialContext, so a provider
// hostname that resolves to a private address (DNS rebinding, a poisoned
// resolver) is refused.
func TestGeocoderClient_ProductionTransportIsSSRFGuarded(t *testing.T) {
	c, err := NewGeocoderClient(config.GeocoderProviderNominatim, "")
	require.NoError(t, err)

	rt, ok := c.client.Transport.(faultingRoundTripper)
	require.True(t, ok, "requests go through the fault seam")
	assert.Equal(t, faultGeocoderRequest, rt.name)
	assert.Same(t, getGeocoderTransport(), rt.base, "and then the shared guarded transport")
	assert.NotNil(t, getGeocoderTransport().DialContext)

	// A literal loopback target is a "resolves only to a non-public address" case.
	_, err = geocoderPrivateBlockingDialContext(context.Background(), "tcp", "127.0.0.1:9")
	assert.ErrorIs(t, err, ErrGeocoderPrivateAddr)

	// And a full request to a loopback server fails closed, mapped to unreachable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the guarded transport must never reach a loopback server")
	}))
	t.Cleanup(srv.Close)
	guarded := newGeocoderClient(config.GeocoderProviderNominatim, "", srv.URL, getGeocoderTransport(), nil)
	_, err = guarded.Geocode(context.Background(), "x")
	assert.ErrorIs(t, err, ErrGeocoderUnreachable)
	assert.ErrorContains(t, err, ErrGeocoderPrivateAddr.Error())
}

func TestGeocoderClient_NominatimIsThrottled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) }))
	t.Cleanup(srv.Close)

	var slept []time.Duration
	now := time.Unix(1_000_000, 0)
	gate := &rateGate{
		interval: nominatimMinInterval,
		now:      func() time.Time { return now },
		sleep:    func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	c := newGeocoderClient(config.GeocoderProviderNominatim, "", srv.URL, plainTransport(), gate)

	for i := 0; i < 3; i++ {
		_, _ = c.Geocode(context.Background(), "x")
	}
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, slept,
		"the first call goes straight out; each later one is spaced a full second after the previous slot")
}

func TestGeocoderClient_CanceledWhileThrottledIsUnreachable(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	t.Cleanup(srv.Close)

	gate := newRateGate(time.Hour, 0)
	require.NoError(t, gate.Wait(context.Background())) // takes the free slot; the next waits an hour
	c := newGeocoderClient(config.GeocoderProviderNominatim, "", srv.URL, plainTransport(), gate)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Geocode(ctx, "x")
	assert.ErrorIs(t, err, ErrGeocoderUnreachable)
	assert.Zero(t, atomic.LoadInt32(&hits), "a request that never got its slot is never sent")
}

func TestRateGate(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	var slept []time.Duration
	g := &rateGate{
		interval: time.Second,
		now:      func() time.Time { return now },
		sleep:    func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	ctx := context.Background()

	require.NoError(t, g.Wait(ctx))
	require.NoError(t, g.Wait(ctx))
	require.NoError(t, g.Wait(ctx))
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, slept, "slots are reserved one interval apart")

	now = now.Add(10 * time.Second) // idle long enough that every reserved slot has passed
	slept = nil
	require.NoError(t, g.Wait(ctx))
	assert.Empty(t, slept, "after an idle gap the next call is immediate")

	// An already-canceled context with a free slot still reports the cancellation.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	now = now.Add(10 * time.Second)
	assert.ErrorIs(t, g.Wait(canceled), context.Canceled)
}

func TestRateGate_ConcurrentCallersGetDistinctSlots(t *testing.T) {
	now := time.Unix(3_000_000, 0)
	var mu sync.Mutex
	var slept []time.Duration
	g := &rateGate{
		interval: time.Second,
		now:      func() time.Time { return now },
		sleep: func(_ context.Context, d time.Duration) error {
			mu.Lock()
			slept = append(slept, d)
			mu.Unlock()
			return nil
		},
	}

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = g.Wait(context.Background()) }()
	}
	wg.Wait()

	seen := map[time.Duration]bool{}
	for _, d := range slept {
		assert.False(t, seen[d], "two callers were handed the same slot %s", d)
		seen[d] = true
	}
	assert.Len(t, slept, n-1, "all but the first caller waited")
}

func TestSleepContext(t *testing.T) {
	require.NoError(t, sleepContext(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	assert.ErrorIs(t, sleepContext(ctx, time.Hour), context.Canceled)
	assert.Less(t, time.Since(start), time.Second)
}

// A response that promises more bytes than it delivers is a read error, not a
// parse: it maps to invalid data and carries no request URL (and so no key).
func TestGeocoderClient_TruncatedBodyIsInvalidData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500")
		_, _ = w.Write([]byte(`[{"lat":"1"`))
		// returning here ends the response 489 bytes short, which the client sees as an unexpected EOF
	}))
	t.Cleanup(srv.Close)

	_, err := maptilerClient(srv.URL, "SUPER-SECRET").Geocode(context.Background(), "x")
	require.ErrorIs(t, err, ErrGeocoderInvalidData)
	assert.NotContains(t, err.Error(), "SUPER-SECRET")
}

func TestRedactedTransportError(t *testing.T) {
	inner := errors.New("connection refused")
	wrapped := &url.Error{Op: "Get", URL: "https://api.maptiler.com/geocoding/x.json?key=SECRET", Err: inner}
	assert.Equal(t, "connection refused", redactedTransportError(wrapped))
	assert.Equal(t, "connection refused", redactedTransportError(fmt.Errorf("outer: %w", wrapped)), "found through wrapping too")
	assert.Equal(t, "plain", redactedTransportError(errors.New("plain")))
}

func TestRateGate_RejectsBeyondMaxWaitWithoutReservingSlot(t *testing.T) {
	now := time.Unix(4_000_000, 0)
	var slept []time.Duration
	g := &rateGate{
		interval: time.Second,
		maxWait:  3 * time.Second,
		now:      func() time.Time { return now },
		sleep:    func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	ctx := context.Background()
	// Slots at +0,+1,+2,+3 are accepted: (maxWait/interval)+1 callers.
	for i := 0; i < 4; i++ {
		require.NoError(t, g.Wait(ctx))
	}
	next := g.next
	for i := 0; i < 5; i++ {
		assert.ErrorIs(t, g.Wait(ctx), errRateGateFull)
	}
	assert.Equal(t, next, g.next, "a rejected caller must not advance the queue")
	assert.Len(t, slept, 3)

	now = now.Add(time.Second) // one slot drains; the queue accepts again
	require.NoError(t, g.Wait(ctx))
}

func TestRateGate_CanceledWaiterStillConsumesSlotWithinMaxWait(t *testing.T) {
	now := time.Unix(5_000_000, 0)
	g := &rateGate{
		interval: time.Second,
		maxWait:  10 * time.Second,
		now:      func() time.Time { return now },
		sleep:    func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}
	require.NoError(t, g.Wait(context.Background()))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, g.Wait(canceled), context.Canceled)
	assert.Equal(t, now.Add(2*time.Second), g.next, "the canceled waiter's slot stays consumed")
}

func TestGeocoderClient_FullGateMapsToRateLimitedWithoutRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	t.Cleanup(srv.Close)
	gate := newRateGate(time.Hour, time.Second)
	require.NoError(t, gate.Wait(context.Background())) // free slot; the next is an hour away
	c := newGeocoderClient(config.GeocoderProviderNominatim, "", srv.URL, plainTransport(), gate)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // a regression fails fast instead of sleeping an hour
	defer cancel()
	_, err := c.Geocode(ctx, "x")
	assert.ErrorIs(t, err, ErrGeocoderRateLimited)
	assert.Less(t, time.Since(start), time.Second, "rejected immediately, no hang")
	assert.Zero(t, atomic.LoadInt32(&hits))
}
