package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setBuildVersionForTest overrides the link-time build identity for the
// duration of a test. buildinfo.Version is a plain package var, so this is
// the seam the version-comparison tests need.
func setBuildVersionForTest(t *testing.T, v string) {
	t.Helper()
	orig := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = orig })
}

// resetUpdateCheckForTest clears the memoized lookup before and after a test.
func resetUpdateCheckForTest(t *testing.T) {
	t.Helper()
	ResetUpdateCheckCache()
	t.Cleanup(ResetUpdateCheckCache)
}

// pointUpdateCheckAtTestServer repoints the update check at an httptest server
// for the duration of a test (a plain client — the SSRF-guarded default would
// refuse the loopback address).
func pointUpdateCheckAtTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origURL := updateCheckAPIBaseURL
	origClient := updateCheckClient
	updateCheckAPIBaseURL = srv.URL
	updateCheckClient = srv.Client()
	t.Cleanup(func() {
		updateCheckAPIBaseURL = origURL
		updateCheckClient = origClient
	})
}

// recordingRoundTripper counts RoundTrip calls and always fails, so a test can
// assert that a disabled (or cached) update check never touches the network.
type recordingRoundTripper struct {
	calls atomic.Int32
}

func (r *recordingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, errors.New("recordingRoundTripper: unexpected outbound call")
}

func TestBuildUpdateCheckStatus_Disabled_MakesNoOutboundCall(t *testing.T) {
	resetUpdateCheckForTest(t)
	rt := &recordingRoundTripper{}
	restore := SetUpdateCheckForTest("http://example.invalid", &http.Client{Transport: rt})
	defer restore()

	status := BuildUpdateCheckStatus(context.Background(), config.Config{UpdateCheckEnabled: false})

	assert.False(t, status.Enabled)
	assert.Empty(t, status.Current)
	assert.Empty(t, status.Latest)
	assert.False(t, status.UpdateAvailable)
	assert.Nil(t, status.CheckedAt)
	assert.Zero(t, rt.calls.Load(), "a disabled update check must not dial out")
}

func TestBuildUpdateCheckStatus_Enabled_ReportsUpdateAvailable(t *testing.T) {
	setBuildVersionForTest(t, "v0.6.2")
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		fmt.Fprintln(w, `{"tag_name":"v0.7.0","name":"unused release title","body":"ignored body"}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	status := BuildUpdateCheckStatus(context.Background(), config.Config{UpdateCheckEnabled: true})

	assert.True(t, status.Enabled)
	assert.Equal(t, "v0.6.2", status.Current)
	assert.Equal(t, "v0.7.0", status.Latest)
	assert.True(t, status.UpdateAvailable)
	require.NotNil(t, status.CheckedAt)
	assert.WithinDuration(t, time.Now(), *status.CheckedAt, time.Minute)
}

func TestBuildUpdateCheckStatus_Enabled_DevBuildNeverAvailable(t *testing.T) {
	// buildinfo.Get().Version defaults to "dev" in tests — a non-release
	// build must never read "update available", even when a newer tag exists.
	setBuildVersionForTest(t, "dev")
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	status := BuildUpdateCheckStatus(context.Background(), config.Config{UpdateCheckEnabled: true})

	assert.True(t, status.Enabled)
	assert.Equal(t, "dev", status.Current)
	assert.Equal(t, "v9.9.9", status.Latest, "latest is still reported")
	assert.False(t, status.UpdateAvailable, "a non-release running version is never 'behind'")
}

func TestBuildUpdateCheckStatus_Enabled_UpToDate(t *testing.T) {
	setBuildVersionForTest(t, "v0.6.2")
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":"v0.6.2"}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	status := BuildUpdateCheckStatus(context.Background(), config.Config{UpdateCheckEnabled: true})

	assert.False(t, status.UpdateAvailable)
	assert.Equal(t, "v0.6.2", status.Latest)
}

func TestBuildUpdateCheckStatus_Enabled_StubErrorIsUnknownNotFailure(t *testing.T) {
	setBuildVersionForTest(t, "v0.6.2")
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	status := BuildUpdateCheckStatus(context.Background(), config.Config{UpdateCheckEnabled: true})

	assert.True(t, status.Enabled)
	assert.Equal(t, "v0.6.2", status.Current)
	assert.Empty(t, status.Latest, "latest is 'unknown' on any lookup error")
	assert.False(t, status.UpdateAvailable)
	assert.Nil(t, status.CheckedAt)
}

func TestLatestRelease_MemoizesWithinTTL(t *testing.T) {
	resetUpdateCheckForTest(t)
	prev := SetUpdateCheckCacheTTL(time.Hour)
	defer func() { SetUpdateCheckCacheTTL(prev); ResetUpdateCheckCache() }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprintln(w, `{"tag_name":"v1.0.0"}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	first, err := latestRelease(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", first)
	assert.EqualValues(t, 1, calls.Load())

	second, err := latestRelease(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", second)
	assert.EqualValues(t, 1, calls.Load(), "a second call within the TTL must not hit the network")
}

func TestLatestRelease_RefetchesAfterTTLExpiry(t *testing.T) {
	resetUpdateCheckForTest(t)
	prev := SetUpdateCheckCacheTTL(-1 * time.Nanosecond)
	defer func() { SetUpdateCheckCacheTTL(prev); ResetUpdateCheckCache() }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprintln(w, `{"tag_name":"v1.0.0"}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	_, err := latestRelease(context.Background())
	require.NoError(t, err)
	_, err = latestRelease(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 2, calls.Load(), "an expired cache must refetch")
}

func TestLatestRelease_ErrorsAreNotCached(t *testing.T) {
	resetUpdateCheckForTest(t)
	prev := SetUpdateCheckCacheTTL(time.Hour)
	defer func() { SetUpdateCheckCacheTTL(prev); ResetUpdateCheckCache() }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	_, err := latestRelease(context.Background())
	require.Error(t, err, "non-200 must be an error")
	_, err = latestRelease(context.Background())
	require.Error(t, err, "a failed lookup must not be memoized — the next call retries")
	assert.EqualValues(t, 2, calls.Load())
}

func TestLatestRelease_Non200ReturnsError(t *testing.T) {
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	_, err := latestRelease(context.Background())
	require.Error(t, err)
}

func TestLatestRelease_GarbageBodyReturnsError(t *testing.T) {
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `this is not json {`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	_, err := latestRelease(context.Background())
	require.Error(t, err)
}

func TestLatestRelease_EmptyTagNameReturnsError(t *testing.T) {
	resetUpdateCheckForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":""}`)
	}))
	defer srv.Close()
	pointUpdateCheckAtTestServer(t, srv)

	_, err := latestRelease(context.Background())
	require.Error(t, err)
}

func TestIsUpdateAvailable(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "newer release", current: "v0.6.2", latest: "v0.7.0", want: true},
		{name: "same version", current: "v0.6.2", latest: "v0.6.2", want: false},
		{name: "already ahead", current: "v1.0.0", latest: "v0.6.2", want: false},
		{name: "prerelease ordering", current: "v0.6.2", latest: "v0.7.0-rc.1", want: true},
		{name: "dev build", current: "dev", latest: "v0.7.0", want: false},
		{name: "empty current", current: "", latest: "v0.7.0", want: false},
		{name: "garbage current", current: "not-a-version", latest: "v0.7.0", want: false},
		{name: "garbage latest", current: "v0.6.2", latest: "v0.6.2-abc", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUpdateAvailable(tt.current, tt.latest))
		})
	}
}

func TestUpdateCheckClient_TransportIsSSRFGuarded(t *testing.T) {
	// The default client must route through the shared SSRF-guarded dialer:
	// an outbound call from the server must never be able to reach loopback/
	// private networks (ASVS 5.2.6). The injected per-test client replaces
	// this var, so assert on a freshly built one.
	client := newUpdateCheckClient()
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "default update-check transport must be an *http.Transport")
	require.NotNil(t, transport.DialContext, "the default transport must declare an SSRF-guarded DialContext")

	_, dialErr := transport.DialContext(context.Background(), "tcp", "127.0.0.1:9999")
	assert.Error(t, dialErr, "the dialer must refuse a loopback address")
}
