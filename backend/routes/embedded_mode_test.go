package routes

import (
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// routeSet indexes a registered router's (METHOD, path) pairs for lookup.
func routeSet(t *testing.T, router *gin.Engine) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, r := range router.Routes() {
		set[r.Method+" "+r.Path] = true
	}
	return set
}

// TestRegisterRoutes_EmbeddedOmitsNetworkSurfaces is the route-table half of
// ADR 0028's embedded mode (issue #1258): every network-only surface is absent
// from the router entirely (404, not 403), while the CRM's own surface stays.
// It drives RegisterRoutes directly rather than booting a server, so the gate
// is pinned independently of the embedded package's lifecycle test.
func TestRegisterRoutes_EmbeddedOmitsNetworkSurfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	cfg.Deployment = config.DeploymentEmbedded
	// DAV is enabled in config but must still not be served in embedded mode.
	cfg.CardDAVEnabled = true
	cfg.CalDAVEnabled = true

	RegisterRoutes(router, cfg, db, nil)
	got := routeSet(t, router)

	disabled := []string{
		"POST /api/v1/register",
		"POST /api/v1/login",
		"POST /api/v1/login/2fa",
		"POST /api/v1/webauthn/login/begin",
		"POST /api/v1/webauthn/login/finish",
		"POST /api/v1/webauthn/register/begin",
		"POST /api/v1/auth/device/session",
		"POST /api/v1/password-reset/request",
		"POST /api/v1/password-reset/confirm",
		"GET /api/v1/auth/oidc/config",
		"POST /api/v1/contacts/:id/addresses/geocode",            // ADR 0031: outbound geocoder, absent like Immich/Paperless
		"POST /api/v1/contacts/:id/addresses/:addressId/geocode", // ADR 0031: outbound geocoder, absent like Immich/Paperless
		"GET /api/v1/users/2fa/status",
		"POST /api/v1/users/2fa/setup",
		"GET /api/v1/api-tokens",
		"POST /api/v1/api-tokens",
		"GET /api/v1/feeds",
		"POST /api/v1/feeds",
		"GET /api/v1/feeds/atom",
		"GET /api/v1/contact-shares/incoming",
		"POST /api/v1/contact-shares",
		"GET /api/v1/webhooks",
		"POST /api/v1/webhooks",
		"GET /api/v1/auth/device/grants",
		"POST /api/v1/auth/device/grants",
		"GET /api/v1/notifications/devices",
		"POST /api/v1/notifications/devices",
		"GET /api/v1/notifications/push-subscriptions",
		"POST /api/v1/notifications/push-subscriptions",
		"GET /.well-known/carddav",
		"GET /.well-known/caldav",
		"PROPFIND /.well-known/carddav",
		"GET /.well-known/assetlinks.json", // ADR 0034: no domain in embedded mode
	}
	for _, route := range disabled {
		require.Falsef(t, got[route], "%s must not be registered in embedded mode", route)
	}

	enabled := []string{
		"GET /health",
		"GET /api/v1/contacts",
		"GET /api/v1/dashboard",
		"GET /api/v1/export",
		"GET /api/v1/search",
		"POST /api/v1/logout",
		"GET /api/v1/config/map", // ADR 0031: the Android map needs the tile style, and it touches no integration
	}
	for _, route := range enabled {
		require.Truef(t, got[route], "%s must stay registered in embedded mode", route)
	}
}

// TestRegisterRoutes_ServerKeepsNetworkSurfaces is the other half: server mode
// (the default zero value) is unchanged.
func TestRegisterRoutes_ServerKeepsNetworkSurfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	RegisterRoutes(router, cfg, db, nil)
	got := routeSet(t, router)

	for _, route := range []string{
		"GET /.well-known/assetlinks.json",
		"POST /api/v1/register",
		"POST /api/v1/login",
		"GET /api/v1/api-tokens",
		"GET /api/v1/webhooks",
		"GET /api/v1/config/map",
		"POST /api/v1/contacts/:id/addresses/geocode",
		"POST /api/v1/contacts/:id/addresses/:addressId/geocode",
	} {
		require.Truef(t, got[route], "server mode must keep %s", route)
	}
}

// A hand-built Config that slipped past Validate with a keyless maptiler
// provider must not take route registration down: the geocode route is still
// registered, with geocoding disabled (NewGeocoder refuses, the handler answers
// 422 "not enabled").
func TestRegisterRoutes_MisconfiguredGeocoderStillRegistersDisabledRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	cfg.GeocoderProvider = config.GeocoderProviderMapTiler // no GeocoderAPIKey
	require.NotPanics(t, func() { RegisterRoutes(router, cfg, db, nil) })
	require.True(t, routeSet(t, router)["POST /api/v1/contacts/:id/addresses/geocode"])
	require.True(t, routeSet(t, router)["POST /api/v1/contacts/:id/addresses/:addressId/geocode"])
}
