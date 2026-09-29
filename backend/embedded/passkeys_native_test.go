package embedded

import (
	"context"
	"io"
	"net/http"
	"testing"

	"mycorrhizal/config"

	"github.com/stretchr/testify/require"
)

// ADR 0034 (issue #1293): the native-Android-passkeys surface through the real
// server stack (middleware, router, /health), served over a Unix socket.

const nativePasskeyCert = "A3:65:E6:D6:28:35:03:7A:56:FD:DC:C8:88:80:F5:5F:EC:99:F4:FD:02:1B:A4:C0:AF:50:91:72:34:2E:6E:C0"

func noRedirectClient(socket string) *http.Client {
	c := unixHTTPClient(socket)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func bootNativePasskeys(t *testing.T, deployment string, mutate func(*config.Config)) (*http.Client, func()) {
	t.Helper()
	cfg := newTestConfig(t, deployment)
	mutate(cfg)
	ln, socket := listenUnix(t, "native.sock")
	srv, err := Start(context.Background(), cfg, Options{Listener: ln, disableInitialTriggers: true})
	require.NoError(t, err)
	return noRedirectClient(socket), func() { require.NoError(t, srv.Stop(context.Background())) }
}

func healthCaps(t *testing.T, client *http.Client) map[string]bool {
	t.Helper()
	_, health := getHealth(t, client)
	set := map[string]bool{}
	for _, c := range health["capabilities"].([]any) {
		set[c.(string)] = true
	}
	return set
}

func TestNativePasskeys_EffectiveServesAssetLinksAndCapability(t *testing.T) {
	client, stop := bootNativePasskeys(t, config.DeploymentServer, func(c *config.Config) {
		c.FrontendURL = "https://crm.example.com"
		c.CookieSecure = true
		c.WebAuthnAndroidEnabled = true
		c.WebAuthnAndroidCertSHA256 = []string{nativePasskeyCert}
	})
	defer stop()

	resp, err := client.Get("http://unix/.well-known/assetlinks.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "no redirect, no auth")
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Empty(t, resp.Header.Values("Set-Cookie"))
	require.JSONEq(t, `[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"com.mycorrhizal.crm","sha256_cert_fingerprints":["24:CF:16:6F:59:36:A6:AD:05:B4:4B:6B:56:9D:71:17:7A:03:8D:4A:06:0F:A4:85:E1:50:2E:73:29:EF:06:5E","`+nativePasskeyCert+`"]}}]`, string(body))

	require.True(t, healthCaps(t, client)[config.CapabilityWebAuthnAndroid], "/health must advertise the token")
}

func TestNativePasskeys_SwitchOnButInvalidBootsWithFeatureOff(t *testing.T) {
	// http + IP-ish/localhost FRONTEND_URL: the operator set the switch, the
	// server still boots, the route is 404 (not the SPA, not a placeholder) and
	// the capability is absent.
	client, stop := bootNativePasskeys(t, config.DeploymentServer, func(c *config.Config) {
		c.WebAuthnAndroidEnabled = true
		c.WebAuthnAndroidCertSHA256 = []string{nativePasskeyCert}
	})
	defer stop()

	resp, err := client.Get("http://unix/.well-known/assetlinks.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NotContains(t, string(body), "delegate_permission")
	require.Empty(t, resp.Header.Values("Set-Cookie"))
	require.False(t, healthCaps(t, client)[config.CapabilityWebAuthnAndroid])
}

func TestNativePasskeys_DefaultOffAndEmbeddedNeverAdvertise(t *testing.T) {
	// Default server: switch unset.
	client, stop := bootNativePasskeys(t, config.DeploymentServer, func(c *config.Config) {
		c.FrontendURL = "https://crm.example.com"
		c.CookieSecure = true
	})
	resp, err := client.Get("http://unix/.well-known/assetlinks.json")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.False(t, healthCaps(t, client)[config.CapabilityWebAuthnAndroid])
	stop()

	// Embedded, even with the switch and an otherwise valid setup: no route, no token.
	client, stop = bootNativePasskeys(t, config.DeploymentEmbedded, func(c *config.Config) {
		c.FrontendURL = "https://crm.example.com"
		c.CookieSecure = true
		c.WebAuthnAndroidEnabled = true
		c.WebAuthnAndroidCertSHA256 = []string{nativePasskeyCert}
	})
	defer stop()
	resp, err = client.Get("http://unix/.well-known/assetlinks.json")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.False(t, healthCaps(t, client)[config.CapabilityWebAuthnAndroid])
}
