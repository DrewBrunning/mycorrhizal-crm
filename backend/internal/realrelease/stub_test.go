package realrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubAPI answers every request with one JSON document that satisfies every
// decoder in Seed/Capture, and fails the failAt-th request (1-based; 0 = never)
// with HTTP 500. Walking failAt over every call index drives each error branch
// of the long seed/capture sequences without a live server.
type stubAPI struct {
	failAt int
	calls  int
	failed string
}

func (s *stubAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	rec := httptest.NewRecorder()
	if s.failAt != 0 && s.calls == s.failAt {
		s.failed = req.URL.Path
		rec.WriteHeader(http.StatusInternalServerError)
		_, _ = rec.WriteString(`{"error":"boom"}`)
	} else {
		rec.Header().Set("Content-Type", "application/json")
		auditID := s.calls
		if req.URL.RawQuery == "limit=1" {
			auditID = 1 // a stable newest event, so Capture's audit-settle wait completes
		}
		_, _ = rec.WriteString(`{"contact":{"id":1,"uid":"u1"},"note":{"ID":1},"activity":{"ID":1},` +
			`"circle":{"id":"c"},"tag":{"id":"t"},"token":"tok","secret":"JBSWY3DPEHPK3PXP",` +
			`"recovery_codes":["a","b","c"],"two_factor_required":false,` +
			`"audit_events":[{"id":` + strconv.Itoa(auditID) + `,"operation":"update","entity_type":"contact"}],` +
			`"contacts":[{"id":1}],"notes":[],"activities":[],"circles":[],"tags":[],` +
			`"relationship_edges":[],"life_events":[],"attachments":[{"id":1}],"next_cursor":""}`)
	}
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := f(r)
	if resp != nil {
		resp.Request = r
	}
	return resp, err
}

// fastAuditSettle shortens Capture's audit-settle polling for stub-driven
// tests, whose stub has no asynchronous writes to wait for.
func fastAuditSettle(t *testing.T) {
	t.Helper()
	iv, polls, to := auditSettleInterval, auditSettlePolls, auditSettleTimeout
	auditSettleInterval, auditSettlePolls, auditSettleTimeout = time.Millisecond, 2, time.Second
	t.Cleanup(func() { auditSettleInterval, auditSettlePolls, auditSettleTimeout = iv, polls, to })
}

func TestSeedAndCaptureSurfaceEveryStepFailure(t *testing.T) {
	fastAuditSettle(t)
	ctx := context.Background()

	ok := &stubAPI{}
	creds, err := Seed(ctx, NewClient("http://stub", ok))
	require.NoError(t, err)
	require.NoError(t, EnableTwoFactor(ctx, NewClient("http://stub", &stubAPI{}), creds))
	seedCalls := ok.calls
	require.Greater(t, seedCalls, 20)

	for n := 1; n <= seedCalls; n++ {
		_, err := Seed(ctx, NewClient("http://stub", &stubAPI{failAt: n}))
		require.Errorf(t, err, "seed call %d failing must fail the seed", n)
	}

	capOK := &stubAPI{}
	_, err = Capture(ctx, NewClient("http://stub", capOK))
	require.NoError(t, err)
	for n := 1; n <= capOK.calls; n++ {
		stub := &stubAPI{failAt: n}
		_, err := Capture(ctx, NewClient("http://stub", stub))
		if err == nil {
			// Only the soft-delete id probes record a status instead of failing.
			require.Regexp(t, `^/api/v1/contacts/\d+$`, stub.failed)
			continue
		}
		require.Error(t, err)
	}

	for n := 1; n <= 2; n++ {
		require.Error(t, EnableTwoFactor(ctx, NewClient("http://stub", &stubAPI{failAt: n}), &Credentials{}))
	}
	short := NewClient("http://stub", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"secret":"JBSWY3DPEHPK3PXP","recovery_codes":["only-one"]}`)
		return rec.Result(), nil
	}))
	require.ErrorContains(t, EnableTwoFactor(ctx, short, &Credentials{}), "recovery codes")

	n := 0
	noTok := NewClient("http://stub", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n++
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"contact":{"id":1,"uid":"u"},"note":{"ID":1},"activity":{"ID":1},"circle":{"id":"c"},"tag":{"id":"t"},"audit_events":[{"id":` + strconv.Itoa(n) + `,"operation":"update"}]}`)
		return rec.Result(), nil
	}))
	_, err = Seed(ctx, noTok)
	require.ErrorContains(t, err, "no plaintext token")
}

func TestMergeToleratesOnlyAnUnresolvableConflict(t *testing.T) {
	ctx := context.Background()
	stub := &stubAPI{}
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/v1/contacts/merge" {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusBadRequest)
			_, _ = rec.WriteString(`{"error":"unresolved"}`)
			return rec.Result(), nil
		}
		return stub.RoundTrip(req)
	})
	_, err := Seed(ctx, NewClient("http://stub", rt))
	require.NoError(t, err, "a 400 merge conflict must not fail the seed")
}

func TestUndoNeedsAnUpdateEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"uid":"u","card":{},"crm":{},"audit_events":[]}`)
		if req.URL.Query().Get("entity_id") != "" {
			cancel() // end the polling loop promptly
		}
		return rec.Result(), nil
	})
	require.ErrorIs(t, undoAnUpdate(ctx, NewClient("http://stub", rt), 1), context.Canceled)
}

func TestUndoSurfacesUndoFailure(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		if req.Method == http.MethodPost {
			rec.WriteHeader(http.StatusGone)
		}
		_, _ = rec.WriteString(`{"uid":"u","card":{},"crm":{},"audit_events":[{"id":50,"operation":"update"}]}`)
		return rec.Result(), nil
	})
	// newestBefore is 50 (from /audit?limit=1) so the same row is not "new";
	// bump it by serving a higher id once the PUT happened.
	put := false
	rt2 := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPut {
			put = true
		}
		rec := httptest.NewRecorder()
		if req.Method == http.MethodPost {
			rec.WriteHeader(http.StatusGone)
		}
		id := "50"
		if put {
			id = "51"
		}
		_, _ = rec.WriteString(`{"uid":"u","card":{},"crm":{},"audit_events":[{"id":` + id + `,"operation":"update"}]}`)
		return rec.Result(), nil
	})
	_ = rt
	err := undoAnUpdate(context.Background(), NewClient("http://stub", rt2), 1)
	require.ErrorContains(t, err, "audit undo")
}

// auditFeed serves /audit?limit=1 from ids (repeating the last one), and an
// empty-but-valid document for every other path.
func auditFeed(ids ...int) roundTripFunc {
	n := 0
	return func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		id := ids[len(ids)-1]
		if n < len(ids) {
			id = ids[n]
		}
		n++
		_, _ = rec.WriteString(`{"audit_events":[{"id":` + strconv.Itoa(id) + `}]}`)
		return rec.Result(), nil
	}
}

func TestAwaitAuditSettled_WaitsForLateEvents(t *testing.T) {
	fastAuditSettle(t)
	// Events still landing (1, 2, 3) must not count as settled; the wait ends
	// only once the newest id has held for auditSettlePolls reads.
	calls := 0
	feed := auditFeed(1, 2, 3, 3)
	c := NewClient("http://stub", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return feed(r)
	}))
	require.NoError(t, awaitAuditSettled(context.Background(), c))
	require.Equal(t, 4, calls, "settled only after the newest id repeated")
}

func TestAwaitAuditSettled_EmptyLogSettles(t *testing.T) {
	fastAuditSettle(t)
	c := NewClient("http://stub", roundTripFunc(func(*http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"audit_events":[]}`)
		return rec.Result(), nil
	}))
	require.NoError(t, awaitAuditSettled(context.Background(), c))
}

func TestAwaitAuditSettled_GivesUpWhenNeverSettled(t *testing.T) {
	fastAuditSettle(t)
	auditSettleTimeout = 20 * time.Millisecond
	n := 0
	c := NewClient("http://stub", roundTripFunc(func(*http.Request) (*http.Response, error) {
		n++
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"audit_events":[{"id":` + strconv.Itoa(n) + `}]}`)
		return rec.Result(), nil
	}))
	err := awaitAuditSettled(context.Background(), c)
	require.ErrorContains(t, err, "still changing")
}

func TestAwaitAuditSettled_HonoursContextCancellation(t *testing.T) {
	fastAuditSettle(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewClient("http://stub", roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel() // cancelled while waiting for the next poll
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"audit_events":[{"id":1}]}`)
		return rec.Result(), nil
	}))
	require.ErrorIs(t, awaitAuditSettled(ctx, c), context.Canceled)
}
