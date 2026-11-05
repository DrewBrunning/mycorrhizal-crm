package monica

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The integration matrix (integrations.monicaIntegration) states a 30s
// per-request deadline; these pin it so the row cannot drift from the code.
func TestMonicaClient_RequestTimeoutIsWired(t *testing.T) {
	c, err := NewClient("http://monica.example.com", "tok", false)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, requestTimeout)
	assert.Equal(t, requestTimeout, c.http.Timeout)
}

// A server that accepts the connection and never answers must fail the call as
// ErrUnreachable within the client timeout, not hang the import session.
func TestMonicaClient_HungServerFailsAsUnreachable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release) // runs before srv.Close so the blocked handler returns

	c := newTestClient(t, srv.URL)
	c.http.Timeout = 100 * time.Millisecond

	start := time.Now()
	err := c.TestConnection(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.Less(t, time.Since(start), 5*time.Second, "a timed-out request must not hang")
}
