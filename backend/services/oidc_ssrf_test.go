package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOIDCClient_TimeoutIsWired pins that every OIDC call is bounded by
// oidcRequestTimeout — before INT-02 (#465) the go-oidc / x/oauth2 default
// transports imposed no timeout and a stalled provider could hang the login
// callback for the whole request-context deadline.
func TestOIDCClient_TimeoutIsWired(t *testing.T) {
	idp := newFakeIDP(t, "test-client")
	defer idp.Close()

	provider, err := InitOIDCProvider(context.Background(), testOIDCConfig(idp.Server.URL, "test-client"))
	require.NoError(t, err)
	require.NotNil(t, provider.httpClient)
	assert.Equal(t, oidcRequestTimeout, provider.httpClient.Timeout,
		"the client threaded through discovery/token/JWKS/UserInfo must carry oidcRequestTimeout")
}

// TestOIDCClient_AllowsLoopbackByDefault pins the deliberate default: with
// OIDC_BLOCK_PRIVATE_URLS unset, an identity provider on a private/loopback
// address (Authentik/Keycloak on the same Docker network) still works. A
// future flip of the default to "on" must break this test.
func TestOIDCClient_AllowsLoopbackByDefault(t *testing.T) {
	idp := newFakeIDP(t, "test-client")
	defer idp.Close() // httptest.Server listens on 127.0.0.1 — a private address

	cfg := testOIDCConfig(idp.Server.URL, "test-client")
	require.False(t, cfg.OIDC.BlockPrivateURLs, "default must be off")

	provider, err := InitOIDCProvider(context.Background(), cfg)
	require.NoError(t, err, "loopback IdP must be reachable when the guard is off")
	require.NotNil(t, provider)
}

// TestOIDCClient_BlocksPrivateAddressWhenEnabled pins the guard: with
// OIDC_BLOCK_PRIVATE_URLS on, the discovery dial to a loopback IdP is refused
// at dial time by httputil.SafeDialContext, so InitOIDCProvider fails closed
// rather than reaching an internal address.
func TestOIDCClient_BlocksPrivateAddressWhenEnabled(t *testing.T) {
	idp := newFakeIDP(t, "test-client")
	defer idp.Close()

	cfg := testOIDCConfig(idp.Server.URL, "test-client")
	cfg.OIDC.BlockPrivateURLs = true

	provider, err := InitOIDCProvider(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.ErrorContains(t, err, "failed to initialize OIDC provider")
}

// TestNewOIDCHTTPClient_GuardOnlyWhenEnabled is the unit-level companion: the
// dialer is guarded exactly when the flag is set, and the timeout is always
// wired.
func TestNewOIDCHTTPClient_GuardOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name         string
		blockPrivate bool
		wantGuard    bool
	}{
		{"guard off", false, false},
		{"guard on", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newOIDCHTTPClient(tc.blockPrivate)
			assert.Equal(t, oidcRequestTimeout, c.Timeout)

			rt, ok := c.Transport.(oidcRoundTripper)
			require.True(t, ok, "transport must be the fault-seam RoundTripper")
			base, ok := rt.base.(*http.Transport)
			require.True(t, ok, "base must be *http.Transport")

			if tc.wantGuard {
				assert.NotNil(t, base.DialContext, "guard on: DialContext must be wired to SafeDialContext")
			} else {
				assert.Nil(t, base.DialContext, "guard off: DialContext must be the library default (nil)")
			}
		})
	}
}
