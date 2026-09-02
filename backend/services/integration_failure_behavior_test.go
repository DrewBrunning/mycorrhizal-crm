package services

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBlackHoleServer returns a base URL whose TCP listener accepts connections
// and then holds them open, silent — never writing an HTTP response. Unlike an
// httptest.Server, tearing it down does not wait on any in-flight handler, so a
// test that relies on the *client* timeout firing cannot itself hang at
// cleanup. Every accepted connection is closed when the test ends.
func newBlackHoleServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
	})

	return "http://" + ln.Addr().String()
}

// INT-02 (issue #465): the outbound storage/enrichment clients
// (Paperless, Seafile, WebDAV) each get their failure behavior exercised, not
// reviewed. The bar is issue #434's "the outcome is defined" — the mapped
// sentinel reaches the caller, unchanged, with no swallow and no panic — plus
// bounded failure (a black-hole host is cut off by the client timeout, not left
// hanging). Immich already has this via immich_fault_injection_test.go;
// contact/calendar sync add the observability + local-data-safety half in
// sync_failure_behavior_test.go.

// integrationClientCase is one client under test: how to build it against a
// base URL, and the small set of public calls that all route through the same
// request seam.
type integrationClientCase struct {
	name      string
	seam      string
	timeout   *time.Duration // the client's request-timeout var, shrinkable in a test
	build     func(t *testing.T, baseURL string) (call func() error)
	sentinels clientSentinels
}

type clientSentinels struct {
	unreachable  error
	unauthorized error // 401 and 403 both map here by design
	notFound     error // 404 and 410
}

func integrationClientCases() []integrationClientCase {
	return []integrationClientCase{
		{
			name:    "paperless",
			seam:    faultPaperlessRequest,
			timeout: &paperlessRequestTimeout,
			build: func(t *testing.T, baseURL string) func() error {
				c, err := NewPaperlessClient(baseURL, "tok", false)
				require.NoError(t, err)
				return c.Ping
			},
			sentinels: clientSentinels{ErrPaperlessUnreachable, ErrPaperlessUnauthorized, ErrPaperlessNotFound},
		},
		{
			name:    "seafile",
			seam:    faultSeafileRequest,
			timeout: &seafileRequestTimeout,
			build: func(t *testing.T, baseURL string) func() error {
				c, err := NewSeafileClient(baseURL, "tok", false)
				require.NoError(t, err)
				return c.PingAuth
			},
			sentinels: clientSentinels{ErrSeafileUnreachable, ErrSeafileUnauthorized, ErrSeafileNotFound},
		},
		{
			name:    "webdav",
			seam:    faultWebDAVRequest,
			timeout: &webdavRequestTimeout,
			build: func(t *testing.T, baseURL string) func() error {
				c, err := NewWebDAVClient(baseURL, "user", "pw", false)
				require.NoError(t, err)
				return c.Ping
			},
			sentinels: clientSentinels{ErrWebDAVUnreachable, ErrWebDAVUnauthorized, ErrWebDAVNotFound},
		},
	}
}

// TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged: an armed fault
// (issue #434) at the request seam surfaces to the caller as exactly that
// error — the transport-error class every one of these clients documents.
func TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	for _, tc := range integrationClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			// A live server so that, absent the fault, the call would succeed —
			// proving the fault is what produced the error.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(srv.Close)

			call := tc.build(t, srv.URL)

			sentinel := errors.New("injected upstream failure for " + tc.name)
			faults.ArmError(tc.seam, sentinel)
			t.Cleanup(func() { faults.Disarm(tc.seam) })

			err := call()
			require.Error(t, err)
			// The seam is a RoundTripper, so the client's transport-error path
			// runs: the fault presents as the "unreachable" class (which is
			// correct — an injected transport failure *is* unreachability), and
			// the injected cause is carried in the message. Never a swallow,
			// never a panic, never a success.
			assert.ErrorIs(t, err, tc.sentinels.unreachable)
			assert.ErrorContains(t, err, sentinel.Error(), "the injected cause must be carried to the caller")

			faults.Disarm(tc.seam)
		})
	}
}

// TestIntegrationClient_StatusMappingIsStable: a real HTTP failure status maps
// to the documented sentinel. 401 and 403 collapse to the "unauthorized"
// sentinel by design (the INT-01 matrix notes this); 404/410 to "not found".
func TestIntegrationClient_StatusMappingIsStable(t *testing.T) {
	for _, tc := range integrationClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, m := range []struct {
				status int
				want   error
			}{
				{http.StatusUnauthorized, tc.sentinels.unauthorized},
				{http.StatusForbidden, tc.sentinels.unauthorized},
				{http.StatusNotFound, tc.sentinels.notFound},
				{http.StatusGone, tc.sentinels.notFound},
			} {
				status := m.status
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
				}))
				call := tc.build(t, srv.URL)
				err := call()
				srv.Close()
				require.Error(t, err, "status %d must not be treated as success", status)
				assert.ErrorIs(t, err, m.want, "status %d", status)
			}
		})
	}
}

// TestIntegrationClient_BlackHoleHostIsBounded: a host that accepts the socket
// and never replies is cut off by the client's own timeout — the call returns
// an error, it does not hang. (The exact timeout value is pinned separately by
// TestIntegrationMatrixTimeoutsMatchCode.)
func TestIntegrationClient_BlackHoleHostIsBounded(t *testing.T) {
	for _, tc := range integrationClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			orig := *tc.timeout
			*tc.timeout = 500 * time.Millisecond
			t.Cleanup(func() { *tc.timeout = orig })

			url := newBlackHoleServer(t) // accepts sockets, never replies

			call := tc.build(t, url) // reads *tc.timeout into the client

			done := make(chan error, 1)
			go func() { done <- call() }()

			select {
			case err := <-done:
				require.Error(t, err, "a black-hole host must produce an error")
				assert.ErrorIs(t, err, tc.sentinels.unreachable)
			case <-time.After(5 * time.Second):
				t.Fatal("client did not return — the request is not bounded by its timeout")
			}
		})
	}
}

// TestIntegrationClient_ConnectionRefusedIsFast: nothing listening → the
// unreachable sentinel, promptly (no retry storm, no long wait).
func TestIntegrationClient_ConnectionRefusedIsFast(t *testing.T) {
	for _, tc := range integrationClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			call := tc.build(t, "http://127.0.0.1:1")

			done := make(chan error, 1)
			go func() { done <- call() }()

			select {
			case err := <-done:
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.sentinels.unreachable)
			case <-time.After(10 * time.Second):
				t.Fatal("connection-refused did not fail promptly")
			}
		})
	}
}
