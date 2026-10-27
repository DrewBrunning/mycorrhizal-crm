package meerkatapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statusServer(t *testing.T, status int, body string, cookie bool) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie {
			http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: "t"})
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, false)
	require.NoError(t, err)
	return c
}

func TestLogin_StatusMapping(t *testing.T) {
	ctx := context.Background()
	assert.ErrorIs(t, statusServer(t, 404, "", false).Login(ctx, "a", "b"), ErrNotMeerkat)
	assert.ErrorIs(t, statusServer(t, 400, "", false).Login(ctx, "a", "b"), ErrUnauthorized)
	assert.ErrorIs(t, statusServer(t, 418, "", false).Login(ctx, "a", "b"), ErrInvalidData)
	assert.ErrorIs(t, statusServer(t, 200, "{}", false).Login(ctx, "a", "b"), ErrInvalidData, "200 without a session cookie")
	assert.NoError(t, statusServer(t, 200, "{}", true).Login(ctx, "a", "b"))
}

func TestGetJSON_StatusMapping(t *testing.T) {
	ctx := context.Background()
	for status, want := range map[int]error{404: ErrNotFound, 403: ErrUnauthorized, 418: ErrInvalidData} {
		_, err := statusServer(t, status, "", false).Me(ctx)
		assert.ErrorIs(t, err, want, status)
	}
	_, err := statusServer(t, 200, strings.Repeat("a", maxResponseBytes+10), false).Me(ctx)
	assert.ErrorIs(t, err, ErrInvalidData, "oversize body")
	_, err = statusServer(t, 418, "", false).CountContacts(ctx)
	assert.Error(t, err)
}

func TestFetchProfilePicture_StatusMapping(t *testing.T) {
	ctx := context.Background()
	_, _, err := statusServer(t, 401, "", false).FetchProfilePicture(ctx, 1)
	assert.ErrorIs(t, err, ErrUnauthorized)
	_, _, err = statusServer(t, 418, "", false).FetchProfilePicture(ctx, 1)
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestRetryDelay(t *testing.T) {
	mk := func(v string) *http.Response {
		return &http.Response{Header: http.Header{"Retry-After": {v}}}
	}
	assert.Equal(t, 5*time.Second, retryDelay(mk("5")))
	assert.Equal(t, maxRetryAfter, retryDelay(mk("100000")))
	assert.Equal(t, 2*time.Second, retryDelay(mk("soon")))
	assert.Equal(t, 2*time.Second, retryDelay(&http.Response{Header: http.Header{}}))
}

func TestRawStr(t *testing.T) {
	assert.Nil(t, rawStr(nil))
	assert.Nil(t, rawStr([]byte("null")))
	require.NotNil(t, rawStr([]byte("[1]")))
	assert.Equal(t, "[1]", *rawStr([]byte("[1]")))
}

func TestDetect_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	// /health ok but the contacts envelope has no "contacts" key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"healthy","database":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"nope":1}`))
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	_, err := c.Detect(ctx)
	assert.ErrorIs(t, err, ErrNotMeerkat)

	assert.ErrorIs(t, statusServer(t, 404, "", false).CheckHealth(ctx), ErrNotMeerkat)
}

func TestFetchSnapshot_NestedRowsWithoutContactIDFallBackToParent(t *testing.T) {
	c0 := contactObj(4, "Orphan")
	c0["relationships"] = []map[string]any{{"ID": 1, "name": "X", "type": "Friend"}}
	c0["notes"] = []map[string]any{{"ID": 2, "content": "n", "date": "2024-01-01T00:00:00Z"}}
	c0["reminders"] = []map[string]any{{"ID": 3, "message": "m", "remind_at": "2024-01-01T00:00:00Z", "recurrence": "once"}}
	c, _ := newTestClient(t, &fakeMeerkat{contacts: []map[string]any{c0}})
	c.SetToken(goodToken)
	snap, err := c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{}, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 4, *snap.Relationships[0].ContactID)
	assert.EqualValues(t, 4, *snap.Notes[0].ContactID)
	assert.EqualValues(t, 4, *snap.Reminders[0].ContactID)
}

func TestUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, _ := NewClient(srv.URL, false)
	srv.Close()
	ctx := context.Background()
	assert.ErrorIs(t, c.Login(ctx, "a", "b"), ErrUnreachable)
	_, _, err := c.FetchProfilePicture(ctx, 1)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.ErrorIs(t, c.CheckHealth(ctx), ErrUnreachable)
	_, err = c.Me(ctx)
	assert.ErrorIs(t, err, ErrUnreachable)
	c.Close(ctx) // must not panic or hang
}

func TestCancelledContextStopsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "x", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.Me(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
