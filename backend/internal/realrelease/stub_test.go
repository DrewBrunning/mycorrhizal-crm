package realrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
		_, _ = rec.WriteString(`{"contact":{"id":1,"uid":"u1"},"note":{"ID":1},"activity":{"ID":1},` +
			`"circle":{"id":"c"},"tag":{"id":"t"},"token":"tok","secret":"JBSWY3DPEHPK3PXP",` +
			`"recovery_codes":["a","b","c"],"two_factor_required":false,` +
			`"audit_events":[{"id":` + strconv.Itoa(s.calls) + `,"operation":"update","entity_type":"contact"}],` +
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

func TestSeedAndCaptureSurfaceEveryStepFailure(t *testing.T) {
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
	require.Error(t, undoAnUpdate(ctx, NewClient("http://stub", rt), 1))
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
