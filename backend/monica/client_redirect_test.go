package monica

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectTarget is a server that records whether anything reached it.
func redirectTarget(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"current_page":1,"last_page":1,"total":0}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestGetDoesNotFollowRedirects: the user supplies the Monica base URL, so a
// redirect must not steer the bearer token or the guarded dialer at another
// host. The redirect is reported as a non-Monica response (the wizard then
// says "check the URL"), and the redirect target is never contacted.
func TestGetDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	target, hits := redirectTarget(t)
	monicaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer monicaServer.Close()

	c := newTestClient(t, monicaServer.URL)
	err := c.TestConnection(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidData), "a redirect is a non-Monica response, got %v", err)
	assert.Contains(t, err.Error(), "302")
	assert.Zero(t, hits.Load(), "the redirect target must never be contacted")
}

// TestFetchAvatarDoesNotFollowRedirects: an avatar on the Monica host carries
// the bearer token, so a redirect from it must not be followed either.
func TestFetchAvatarDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	target, hits := redirectTarget(t)
	monicaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/avatar.jpg", http.StatusMovedPermanently)
	}))
	defer monicaServer.Close()

	c := newTestClient(t, monicaServer.URL)
	_, _, err := c.FetchAvatar(context.Background(), monicaServer.URL+"/storage/avatar.jpg")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnreachable), "got %v", err)
	assert.Zero(t, hits.Load(), "the redirect target must never be contacted")
}
