package services

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectTarget records every request that reaches it, with the credential
// headers each integration sends.
type redirectTarget struct {
	mu   sync.Mutex
	hits []http.Header
	srv  *httptest.Server
}

func newRedirectTarget(t *testing.T) *redirectTarget {
	t.Helper()
	rt := &redirectTarget{}
	rt.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt.mu.Lock()
		rt.hits = append(rt.hits, r.Header.Clone())
		rt.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(rt.srv.Close)
	return rt
}

func (rt *redirectTarget) hitCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.hits)
}

// newRedirector answers every request with a status redirect to target.
func newRedirector(t *testing.T, target string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+r.URL.Path, status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Every per-user integration client must refuse redirects: Go forwards custom
// credential headers (Immich's x-api-key, Seafile/Paperless "Authorization:
// Token ..." on a same-host hop) to the redirect target, and a redirect can
// bounce the request to an internal address when the private-URL block is off.
// The redirect target must never be contacted, and the call must fail with the
// integration's redirect sentinel (mapped to "check the base URL").
func TestIntegrationClients_RedirectIsNotFollowedAndCredentialNotForwarded(t *testing.T) {
	statuses := []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect}
	cases := []struct {
		name     string
		sentinel error
		call     func(base string) error
	}{
		{"immich GET", ErrImmichRedirect, func(base string) error {
			c, err := NewImmichClient(base, "immich-secret", false)
			require.NoError(t, err)
			_, err = c.do("/api/users/me")
			return err
		}},
		{"immich POST", ErrImmichRedirect, func(base string) error {
			c, err := NewImmichClient(base, "immich-secret", false)
			require.NoError(t, err)
			_, err = c.doPost("/api/search/metadata", map[string]any{"q": 1})
			return err
		}},
		{"paperless", ErrPaperlessRedirect, func(base string) error {
			c, err := NewPaperlessClient(base, "paperless-secret", false)
			require.NoError(t, err)
			_, err = c.do("/api/documents/")
			return err
		}},
		{"seafile", ErrSeafileRedirect, func(base string) error {
			c, err := NewSeafileClient(base, "seafile-secret", false)
			require.NoError(t, err)
			_, err = c.do("/api2/repos/")
			return err
		}},
		{"seafile unauthenticated ping", ErrSeafileRedirect, func(base string) error {
			c, err := NewSeafileClient(base, "seafile-secret", false)
			require.NoError(t, err)
			_, err = c.doUnauthenticated("/api2/ping/")
			return err
		}},
		{"nextcloud", ErrWebDAVRedirect, func(base string) error {
			c, err := NewWebDAVClient(base, "alice", "nextcloud-secret", false)
			require.NoError(t, err)
			_, err = c.propfind("/")
			return err
		}},
	}
	for _, tc := range cases {
		for _, status := range statuses {
			t.Run(tc.name+"/"+http.StatusText(status), func(t *testing.T) {
				b := newRedirectTarget(t)
				a := newRedirector(t, b.srv.URL, status)

				err := tc.call(a.URL)
				require.ErrorIs(t, err, tc.sentinel)
				assert.Equal(t, 0, b.hitCount(), "the redirect target must never be contacted")
			})
		}
	}
}

func TestIntegrationRedirectDiagnosis(t *testing.T) {
	assert.Contains(t, diagnoseImmichConnectionFailure("reachability", ErrImmichRedirect).Message, "redirect")
	assert.Contains(t, diagnosePaperlessConnectionFailure("reachability", ErrPaperlessRedirect).Message, "redirect")
	assert.Contains(t, diagnoseSeafileConnectionFailure("reachability", ErrSeafileRedirect).Message, "redirect")
	assert.Contains(t, diagnoseWebDAVConnectionFailure(ErrWebDAVRedirect).Message, "redirect")
}
