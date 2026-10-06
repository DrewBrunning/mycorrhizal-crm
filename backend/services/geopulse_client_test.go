package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGeoPulseClient_RejectsBadURLs(t *testing.T) {
	for _, in := range []string{"", "   ", "geopulse.example", "ftp://geopulse.example", "http://"} {
		_, err := NewGeoPulseClient(in, "k", false)
		assert.ErrorIs(t, err, ErrGeoPulseInvalidURL, "%q", in)
	}
	c, err := NewGeoPulseClient("  https://geopulse.example/  ", "k", false)
	require.NoError(t, err)
	assert.Equal(t, "https://geopulse.example", c.baseURL)
}

func TestGeoPulseClient_GetMeSendsKeyAndParsesEnvelope(t *testing.T) {
	f := newFakeGeoPulseServer(t, "secret-key")
	c, err := NewGeoPulseClient(f.URL(), "secret-key", false)
	require.NoError(t, err)

	me, err := c.GetMe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, f.UserID, me.UserID)
	assert.Equal(t, "Test User", me.FullName)
	assert.Equal(t, "secret-key", f.LastKey)
	assert.NoError(t, c.Ping(context.Background()))
}

func TestGeoPulseClient_WrongKeyIsUnauthorized(t *testing.T) {
	f := newFakeGeoPulseServer(t, "right")
	c, err := NewGeoPulseClient(f.URL(), "wrong", false)
	require.NoError(t, err)

	_, err = c.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
	_, err = c.GetStays(context.Background(), time.Now().Add(-time.Hour), time.Now())
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
	_, err = c.SearchPhotos(context.Background(), "u", 1, 2, 200, time.Now().Add(-time.Hour), time.Now(), 5)
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
}

func TestGeoPulseClient_GetMeMissingUserID(t *testing.T) {
	f := newFakeGeoPulseServer(t, "")
	f.UserID = ""
	c, _ := NewGeoPulseClient(f.URL(), "k", false)
	_, err := c.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
}

func TestGeoPulseClient_GetStaysQueriesTheDayAndParsesStays(t *testing.T) {
	f := newFakeGeoPulseServer(t, "")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe Nero", "Leeds", 53.8, -1.55, 3600)
	c, _ := NewGeoPulseClient(f.URL(), "k", false)

	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 20, 23, 59, 59, 0, time.UTC)
	stays, err := c.GetStays(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, stays, 1)
	assert.Equal(t, GeoPulseStay{ID: 7, Timestamp: "2026-09-20T14:00:00Z", LocationName: "Cafe Nero", City: "Leeds", Country: "Testland", Latitude: 53.8, Longitude: -1.55, StayDuration: 3600}, stays[0])

	require.Len(t, f.calls(), 1)
	q, err := url.ParseQuery(strings.SplitN(f.calls()[0], "?", 2)[1])
	require.NoError(t, err)
	assert.Equal(t, "2026-09-20T00:00:00Z", q.Get("startTime"))
	assert.Equal(t, "2026-09-20T23:59:59Z", q.Get("endTime"))
}

func TestGeoPulseClient_SearchPhotosBuildsQueryAndEscapesUserID(t *testing.T) {
	f := newFakeGeoPulseServer(t, "")
	f.addPhoto(53.8, "p1", "IMG_1.jpg", "2026-09-20T14:05:00Z")
	c, _ := NewGeoPulseClient(f.URL(), "k", false)

	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	end := start.Add(24*time.Hour - time.Second)
	photos, err := c.SearchPhotos(context.Background(), "abc/../def", 53.8, -1.55, GeoPulsePhotoRadiusMeters, start, end, 12)
	require.NoError(t, err)
	require.Len(t, photos, 1)
	assert.Equal(t, "p1", photos[0].ID)
	assert.Equal(t, "IMG_1.jpg", photos[0].OriginalFileName)

	call := f.calls()[0]
	assert.True(t, strings.HasPrefix(call, "/api/users/abc%2F..%2Fdef/immich/photos/search?"),
		"the user id must be path-escaped, got %s", call)
	q, _ := url.ParseQuery(strings.SplitN(call, "?", 2)[1])
	assert.Equal(t, "200", q.Get("radiusMeters"))
	assert.Equal(t, "53.8", q.Get("latitude"))
	assert.Equal(t, "-1.55", q.Get("longitude"))
	assert.Equal(t, "12", q.Get("limit"))
	assert.Equal(t, "2026-09-20T00:00:00Z", q.Get("startDate"))
}

func TestGeoPulseClient_MalformedBodiesAreInvalidData(t *testing.T) {
	cases := map[string]string{
		"not json":         `<html>`,
		"error envelope":   `{"status":"error","message":"nope"}`,
		"missing data":     `{"status":"success"}`,
		"null data":        `{"status":"success","data":null}`,
		"wrong data shape": `{"status":"success","data":"a string"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeGeoPulseServer(t, "")
			f.RawBody = body
			c, _ := NewGeoPulseClient(f.URL(), "k", false)
			_, err := c.GetMe(context.Background())
			assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
			_, err = c.GetStays(context.Background(), time.Now().Add(-time.Hour), time.Now())
			assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
			_, err = c.SearchPhotos(context.Background(), "u", 1, 2, 200, time.Now().Add(-time.Hour), time.Now(), 5)
			assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
		})
	}
}

func TestGeoPulseClient_OversizedBodyIsInvalidData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"userId":"` + strings.Repeat("a", maxGeoPulseBodyBytes) + `"}}`))
	}))
	t.Cleanup(srv.Close)
	c, _ := NewGeoPulseClient(srv.URL, "k", false)
	_, err := c.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData, "a body past the cap is truncated and so fails to parse")
}

func TestGeoPulseClient_TruncatedBodyIsInvalidData(t *testing.T) {
	// The server promises more bytes than it sends and drops the connection, so
	// reading the body fails mid-stream — a cut-off response must never parse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500")
		_, _ = w.Write([]byte(`{"status":"succ`))
	}))
	t.Cleanup(srv.Close)
	c, _ := NewGeoPulseClient(srv.URL, "k", false)
	_, err := c.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
}

func TestGeoPulseClient_UnexpectedStatusCarriesStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadRequest} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("boom"))
		}))
		c, _ := NewGeoPulseClient(srv.URL, "k", false)
		_, err := c.GetMe(context.Background())
		srv.Close()
		require.ErrorIs(t, err, ErrGeoPulseRequestFailed, "status %d", status)
		var reqErr *GeoPulseRequestError
		require.ErrorAs(t, err, &reqErr)
		assert.Equal(t, status, reqErr.StatusCode)
		assert.Equal(t, "boom", reqErr.Body)
		assert.Contains(t, err.Error(), http.StatusText(status))
	}
}

func TestGeoPulseClient_UnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, _ := NewGeoPulseClient(url, "k", false)
	_, err := c.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulseUnreachable)
}

func TestGeoPulseClient_PrivateAddressBlockedWhenEnabled(t *testing.T) {
	f := newFakeGeoPulseServer(t, "") // listens on loopback
	blocked, err := NewGeoPulseClient(f.URL(), "k", true)
	require.NoError(t, err)
	_, err = blocked.GetMe(context.Background())
	assert.ErrorIs(t, err, ErrGeoPulsePrivateAddress)
	assert.Empty(t, f.calls(), "a blocked dial must never reach the server")

	open, err := NewGeoPulseClient(f.URL(), "k", false)
	require.NoError(t, err)
	_, err = open.GetMe(context.Background())
	assert.NoError(t, err, "with the guard off (the default) a LAN/loopback GeoPulse works")
}

// TestGeoPulseClient_CoordinateBearingURLNeverAppearsInErrors pins issue #1445:
// SearchPhotos sends latitude/longitude and the day's date range in the query
// string, and net/http wraps a transport failure in a *url.Error that prints the
// whole URL. Every error path must redact it, or the retention claim in
// docs/security/data-retention-lifecycle.md §13 ("not logged") is false.
func TestGeoPulseClient_CoordinateBearingURLNeverAppearsInErrors(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	const (
		lat = 53.8
		lon = -1.55
	)
	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	end := start.Add(24*time.Hour - time.Second)

	assertNoCoordinate := func(t *testing.T, err error) {
		t.Helper()
		msg := err.Error()
		assert.NotContains(t, msg, strconv.FormatFloat(lat, 'f', -1, 64), "the latitude must not appear in the error")
		assert.NotContains(t, msg, strconv.FormatFloat(lon, 'f', -1, 64), "the longitude must not appear in the error")
		assert.NotContains(t, msg, "latitude=")
		assert.NotContains(t, msg, "longitude=")
		assert.NotContains(t, msg, "startDate=")
		assert.NotContains(t, msg, "/immich/photos/search", "the coordinate-bearing URL must not appear at all")
	}

	// An armed transport fault: the client's own unreachable path runs, with the
	// coordinate-bearing URL in scope.
	t.Run("injected transport fault", func(t *testing.T) {
		c, err := NewGeoPulseClient("https://geopulse.example", "k", false)
		require.NoError(t, err)
		faults.ArmError(faultGeoPulseRequest, errors.New("injected upstream failure"))
		t.Cleanup(func() { faults.Disarm(faultGeoPulseRequest) })

		_, err = c.SearchPhotos(context.Background(), "u", lat, lon, GeoPulsePhotoRadiusMeters, start, end, 5)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGeoPulseUnreachable)
		assert.ErrorContains(t, err, "injected upstream failure", "the underlying cause must survive redaction")
		assertNoCoordinate(t, err)
	})

	// A genuinely refused connection: the real *url.Error path.
	t.Run("connection refused", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		require.NoError(t, ln.Close())

		c, err := NewGeoPulseClient("http://"+addr, "k", false)
		require.NoError(t, err)
		_, err = c.SearchPhotos(context.Background(), "u", lat, lon, GeoPulsePhotoRadiusMeters, start, end, 5)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGeoPulseUnreachable)
		assertNoCoordinate(t, err)
	})

	// The SSRF guard's private-address sentinel must survive redaction.
	t.Run("private address still detectable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("the guarded transport must never reach a loopback server")
		}))
		t.Cleanup(srv.Close)
		c, err := NewGeoPulseClient(srv.URL, "k", true)
		require.NoError(t, err)

		_, err = c.SearchPhotos(context.Background(), "u", lat, lon, GeoPulsePhotoRadiusMeters, start, end, 5)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGeoPulsePrivateAddress)
		assertNoCoordinate(t, err)
	})
}

// A 3xx must never be followed: the API key rides a custom header that
// net/http would forward cross-host. Server B must see nothing, and the call
// must fail with the redirect sentinel.
func TestGeoPulseClient_RedirectIsNotFollowedAndKeyNotForwarded(t *testing.T) {
	b := newFakeGeoPulseServer(t, "")
	a := newFakeGeoPulseServer(t, "")
	a.RedirectTo = b.URL() + "/api/users/me"

	c, err := NewGeoPulseClient(a.URL(), "secret-token", false)
	require.NoError(t, err)

	_, err = c.GetMe(context.Background())
	require.ErrorIs(t, err, ErrGeoPulseRedirect)
	_, err = c.GetStays(context.Background(), time.Now().Add(-time.Hour), time.Now())
	require.ErrorIs(t, err, ErrGeoPulseRedirect)

	assert.Empty(t, b.calls(), "the redirect target must never be contacted")
	assert.Empty(t, b.LastKey, "the API key must not reach the redirect target")

	res := diagnoseGeoPulseConnectionFailure("reachability", err)
	assert.Contains(t, res.Message, "redirect")
}

func TestRedactedGeoPulseTransportError_NonURLErrorPassesThrough(t *testing.T) {
	plain := errors.New("plain")
	assert.Equal(t, plain, redactedGeoPulseTransportError(plain))
}
