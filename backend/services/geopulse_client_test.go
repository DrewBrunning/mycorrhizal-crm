package services

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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

	me, err := c.GetMe()
	require.NoError(t, err)
	assert.Equal(t, f.UserID, me.UserID)
	assert.Equal(t, "Test User", me.FullName)
	assert.Equal(t, "secret-key", f.LastKey)
	assert.NoError(t, c.Ping())
}

func TestGeoPulseClient_WrongKeyIsUnauthorized(t *testing.T) {
	f := newFakeGeoPulseServer(t, "right")
	c, err := NewGeoPulseClient(f.URL(), "wrong", false)
	require.NoError(t, err)

	_, err = c.GetMe()
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
	_, err = c.GetStays(time.Now().Add(-time.Hour), time.Now())
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
	_, err = c.SearchPhotos("u", 1, 2, 200, time.Now().Add(-time.Hour), time.Now(), 5)
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
}

func TestGeoPulseClient_GetMeMissingUserID(t *testing.T) {
	f := newFakeGeoPulseServer(t, "")
	f.UserID = ""
	c, _ := NewGeoPulseClient(f.URL(), "k", false)
	_, err := c.GetMe()
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
}

func TestGeoPulseClient_GetStaysQueriesTheDayAndParsesStays(t *testing.T) {
	f := newFakeGeoPulseServer(t, "")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe Nero", "Leeds", 53.8, -1.55, 3600)
	c, _ := NewGeoPulseClient(f.URL(), "k", false)

	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 20, 23, 59, 59, 0, time.UTC)
	stays, err := c.GetStays(start, end)
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
	photos, err := c.SearchPhotos("abc/../def", 53.8, -1.55, GeoPulsePhotoRadiusMeters, start, end, 12)
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
			_, err := c.GetMe()
			assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
			_, err = c.GetStays(time.Now().Add(-time.Hour), time.Now())
			assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
			_, err = c.SearchPhotos("u", 1, 2, 200, time.Now().Add(-time.Hour), time.Now(), 5)
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
	_, err := c.GetMe()
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
	_, err := c.GetMe()
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData)
}

func TestGeoPulseClient_UnexpectedStatusCarriesStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadRequest} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("boom"))
		}))
		c, _ := NewGeoPulseClient(srv.URL, "k", false)
		_, err := c.GetMe()
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
	_, err := c.GetMe()
	assert.ErrorIs(t, err, ErrGeoPulseUnreachable)
}

func TestGeoPulseClient_PrivateAddressBlockedWhenEnabled(t *testing.T) {
	f := newFakeGeoPulseServer(t, "") // listens on loopback
	blocked, err := NewGeoPulseClient(f.URL(), "k", true)
	require.NoError(t, err)
	_, err = blocked.GetMe()
	assert.ErrorIs(t, err, ErrGeoPulsePrivateAddress)
	assert.Empty(t, f.calls(), "a blocked dial must never reach the server")

	open, err := NewGeoPulseClient(f.URL(), "k", false)
	require.NoError(t, err)
	_, err = open.GetMe()
	assert.NoError(t, err, "with the guard off (the default) a LAN/loopback GeoPulse works")
}
