package meerkatapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The integration matrix (integrations.meerkatAPIIntegration) states a 30s
// per-request deadline; these pin it so the row cannot drift from the code.
func TestMeerkatClient_RequestTimeoutIsWired(t *testing.T) {
	c, err := NewClient("http://meerkat.example.com", false)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, requestTimeout)
	assert.Equal(t, requestTimeout, c.http.Timeout)
}

// A server that accepts the connection and never answers must fail the call as
// ErrUnreachable (the timeout surfaces like an unreachable host, and is not
// retried), not hang the import session.
func TestMeerkatClient_HungServerFailsAsUnreachable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release) // runs before srv.Close so the blocked handler returns

	c, err := NewClient(srv.URL, false)
	require.NoError(t, err)
	c.http.Timeout = 100 * time.Millisecond

	start := time.Now()
	_, err = c.Me(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.Less(t, time.Since(start), 5*time.Second, "a timed-out request must not be retried or hang")
}
