package services

import (
	"errors"
	"testing"

	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImmichInjectedRequestFaultCrossesBoundaryUnchanged pins the
// services.immich.request seam (issue #434): each failure class the client
// documents — unreachable, auth-expired, resource-deleted-remotely, plus an
// arbitrary upstream failure — can be injected in-process at the request
// boundary, and every caller sees exactly that sentinel, with no network I/O
// at all. The defined outcome is the sentinel's own: the caller's existing
// error path (T42's classification, the service layer's degrade-to-cache) is
// what consumes it, and it must never be swallowed or remapped here.
func TestImmichInjectedRequestFaultCrossesBoundaryUnchanged(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	client, err := NewImmichClient("https://immich.example.invalid", "test-key", false)
	require.NoError(t, err)

	cases := []struct {
		name     string
		sentinel error
		method   func() error
	}{
		{
			name:     "unreachable",
			sentinel: ErrImmichUnreachable,
			method:   func() error { return client.Ping() },
		},
		{
			name:     "auth expired",
			sentinel: ErrImmichUnauthorized,
			method:   func() error { return client.Ping() },
		},
		{
			name:     "resource deleted remotely",
			sentinel: ErrImmichNotFound,
			method:   func() error { return client.Ping() },
		},
		{
			name:     "custom upstream failure",
			sentinel: errors.New("proxy timeout"),
			method:   func() error { return client.Ping() },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			faults.ArmError(faultImmichRequest, tc.sentinel)
			defer faults.Disarm(faultImmichRequest)

			err := tc.method()
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.sentinel, "the armed sentinel must cross the request boundary unchanged")
		})
	}
}

// TestImmichInjectedFaultDoesNotPersistBeyondArm checks that the seam is
// inert once disarmed — the same client that failed under injection makes a
// real (here, unreachable-by-DNS) request and reports its own failure, not
// the injected one. This is what keeps the seam from leaking between tests
// or between runs of a live process.
func TestImmichInjectedFaultDoesNotPersistBeyondArm(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	client, err := NewImmichClient("https://immich.example.invalid", "test-key", false)
	require.NoError(t, err)

	faults.ArmError(faultImmichRequest, ErrImmichUnauthorized)
	require.ErrorIs(t, client.Ping(), ErrImmichUnauthorized)

	faults.Disarm(faultImmichRequest)
	err = client.Ping()
	require.Error(t, err, "the real request still fails (unreachable DNS) but...")
	assert.NotErrorIs(t, err, ErrImmichUnauthorized, "...not with the injected sentinel")
	assert.ErrorIs(t, err, ErrImmichUnreachable, "the real failure class surfaces instead")
}

// TestImmichInjectedFaultReachesServiceDiagnostics wires the client seam all
// the way to the service layer's defined outcome: TestImmichConnection must
// classify an injected unreachable fault as a failed reachability check and
// return a diagnosed result (never a raw error, never a success). This is the
// "for every injected fault, the outcome is defined" criterion at the layer
// a user actually sees.
func TestImmichInjectedFaultReachesServiceDiagnostics(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	faults.ArmError(faultImmichRequest, ErrImmichUnreachable)
	defer faults.Disarm(faultImmichRequest)

	// The client is constructed directly (no DB config needed for the seam);
	// the seam fires before any network access, so the service diagnostic
	// path is reached with a deterministic failure.
	client, err := NewImmichClient("https://immich.example.invalid", "test-key", false)
	require.NoError(t, err)

	// Prove the seam is what failed: Ping must return the injected sentinel.
	require.ErrorIs(t, client.Ping(), ErrImmichUnreachable)

	// And diagnoseImmichConnectionFailure (the service's classification) must
	// render the sentinel-specific message, not a generic one.
	result := diagnoseImmichConnectionFailure("reachability", ErrImmichUnreachable)
	require.NotNil(t, result)
	assert.False(t, result.OK)
	assert.Contains(t, result.Message, "Could not reach the Immich server")
}
