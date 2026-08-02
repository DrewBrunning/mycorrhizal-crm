package routes

import (
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func testConfig() *config.Config {
	return &config.Config{
		DBPath:           ":memory:",
		JWTSecretKey:     "test-secret-key-that-is-long-enough-32",
		ProfilePhotoDir:  "/tmp/test-photos",
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		JWTExpiryHours:   96,
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

// TestRegisterRoutes_NoPanic verifies that route registration does not panic
// or reference nil functions. This catches regressions where a controller
// function is renamed or removed but the route registration is not updated —
// a problem that Go compilation alone cannot detect when the controller
// function is passed by reference at the wire-up site in routes.go.
func TestRegisterRoutes_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()

	assert.NotPanics(t, func() {
		RegisterRoutes(router, cfg, db, nil)
	}, "RegisterRoutes must not panic with a basic config and no OIDC provider")
}

// TestRegisterRoutes_RouteCountGuardsAgainstAccidentalDeletion asserts a
// minimum route count so that accidentally removing a route registration
// line is noticed. The count is deliberately a floor, not an exact match,
// so that adding new routes doesn't break this test — only removal does.
func TestRegisterRoutes_RouteCountGuardsAgainstAccidentalDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	RegisterRoutes(router, cfg, db, nil)

	routes := router.Routes()
	assert.GreaterOrEqual(t, len(routes), 80,
		"unexpectedly low route count — an entire route group may have been accidentally deleted")
}
