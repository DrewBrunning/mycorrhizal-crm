package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withHIBPTestServer points hibpAPIBaseURL at a local httptest.Server for
// the duration of the test, restoring it afterward, via the same
// SetHIBPAPIBaseURLForTest seam the controllers package tests use.
func withHIBPTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Cleanup(SetHIBPAPIBaseURLForTest(server.URL))
}

// TestCheckPasswordBreached_KnownBreachedPassword pins the k-anonymity
// prefix/suffix split against a SHA-1 vector computed independently
// (Python's hashlib, not this package's crypto/sha1 call) rather than
// deriving the expected prefix/suffix with the same code under test —
// SHA1("password") = 5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8, so a hashing
// bug in CheckPasswordBreached couldn't pass by construction this way.
func TestCheckPasswordBreached_KnownBreachedPassword(t *testing.T) {
	var gotPath string
	withHIBPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, "0000000000000000000000000000000000:1\r\n"+
			"1E4C9B93F3F0682250B6CF8331B7EE68FD8:3730471\r\n"+
			"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:2\r\n")
	})

	breached, err := CheckPasswordBreached(context.Background(), "password")
	require.NoError(t, err)
	assert.True(t, breached, "SHA1(\"password\") is a textbook HIBP-listed hash")
	assert.Equal(t, "/range/5BAA6", gotPath, "only the 5-char prefix must ever be sent, never the full hash or password")
}

func TestCheckPasswordBreached_NotBreached(t *testing.T) {
	withHIBPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Real HIBP responses run to thousands of lines for a busy prefix;
		// a handful of non-matching suffixes is enough to prove the scan
		// doesn't false-positive.
		fmt.Fprint(w, "0000000000000000000000000000000000:1\r\nFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:2\r\n")
	})

	breached, err := CheckPasswordBreached(context.Background(), "a-genuinely-unique-random-passphrase-38471")
	require.NoError(t, err)
	assert.False(t, breached)
}

func TestCheckPasswordBreached_FailsOpenOnServerError(t *testing.T) {
	withHIBPTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	breached, err := CheckPasswordBreached(context.Background(), "whatever")
	assert.Error(t, err)
	assert.False(t, breached, "must fail open: an API error is never treated as a breach")
}

func TestCheckPasswordBreached_FailsOpenOnUnreachableHost(t *testing.T) {
	t.Cleanup(SetHIBPAPIBaseURLForTest("http://127.0.0.1:1")) // nothing listens here; connection refused

	breached, err := CheckPasswordBreached(context.Background(), "whatever")
	assert.Error(t, err)
	assert.False(t, breached, "must fail open: a network error must not block the caller")
}
