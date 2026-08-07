package services

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImmichClientDo_StatusClassification pins T42: do()'s status switch must
// keep 401/403/404/410 mapped to their existing sentinels, map any other
// non-2xx *response* (a real answer from Immich) to the new
// ErrImmichRequestFailed with the real status preserved, and never confuse
// that with an actual transport failure (ErrImmichUnreachable), which is
// exercised separately below since it has no HTTP status at all.
func TestImmichClientDo_StatusClassification(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{name: "401 unauthorized unchanged", statusCode: http.StatusUnauthorized, wantErr: ErrImmichUnauthorized},
		{name: "403 forbidden unchanged", statusCode: http.StatusForbidden, wantErr: ErrImmichUnauthorized},
		{name: "404 not found unchanged", statusCode: http.StatusNotFound, wantErr: ErrImmichNotFound},
		{name: "410 gone unchanged", statusCode: http.StatusGone, wantErr: ErrImmichNotFound},
		{name: "400 bad request is a request failure, not unreachable", statusCode: http.StatusBadRequest, wantErr: ErrImmichRequestFailed},
		{name: "422 unprocessable is a request failure, not unreachable", statusCode: http.StatusUnprocessableEntity, wantErr: ErrImmichRequestFailed},
		{name: "500 internal error is a request failure, not unreachable", statusCode: http.StatusInternalServerError, wantErr: ErrImmichRequestFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{"message":"stubbed failure"}`))
			}))
			defer server.Close()

			client, err := NewImmichClient(server.URL, "k", false)
			require.NoError(t, err)

			_, err = client.ListPeople()
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.wantErr), "expected %v, got %v", tc.wantErr, err)
			assert.False(t, errors.Is(err, ErrImmichUnreachable), "a real HTTP response must never map to ErrImmichUnreachable")
		})
	}
}

// TestImmichClientDo_RequestFailedPreservesStatus pins that the real status
// code/text survive as ImmichRequestError, not just a generic sentinel — the
// controller needs it to render "Immich returned an error (400 Bad Request)"
// instead of the network-down message.
func TestImmichClientDo_RequestFailedPreservesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"size must be <= 100"}`))
	}))
	defer server.Close()

	client, err := NewImmichClient(server.URL, "k", false)
	require.NoError(t, err)

	_, err = client.ListPeople()
	require.Error(t, err)

	var reqErr *ImmichRequestError
	require.True(t, errors.As(err, &reqErr), "expected an *ImmichRequestError, got %T: %v", err, err)
	assert.Equal(t, http.StatusBadRequest, reqErr.StatusCode)
	assert.Contains(t, reqErr.Status, "400")
	assert.Contains(t, reqErr.Body, "size must be")
}

// TestImmichClientDo_TransportFailureStillUnreachable pins the other half of
// the distinction: a genuine dial failure (no HTTP response at all) must stay
// ErrImmichUnreachable, never the new sentinel.
func TestImmichClientDo_TransportFailureStillUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close() // now genuinely unreachable — nothing is listening

	client, err := NewImmichClient(url, "k", false)
	require.NoError(t, err)

	_, err = client.ListPeople()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrImmichUnreachable))
	assert.False(t, errors.Is(err, ErrImmichRequestFailed))
}

// TestImmichClientDo_MidPaginationFailureSurfaces pins the pagination trap:
// a failure on page 2 must surface as an error, not silently return the
// partial list gathered from page 1.
func TestImmichClientDo_MidPaginationFailureSurfaces(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"people":{"items":[{"id":"p1","name":"Alice"}],"hasNextPage":true}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewImmichClient(server.URL, "k", false)
	require.NoError(t, err)

	people, err := client.ListPeople()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrImmichRequestFailed))
	assert.Nil(t, people, "a mid-pagination failure must not return a partial list")
}
