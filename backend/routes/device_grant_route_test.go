package routes

// The device-grant surface is the one place where an endpoint is deliberately
// mounted with BOTH ValidateJSONMiddleware and a handler that reads its input.
// controllers/device_grant_controller_test.go pins the handler against the
// validation middleware in isolation; this file goes one level further and
// drives the request through RegisterRoutes — the exact middleware chain,
// ordering, and route registration production uses. It is the regression guard
// for the bug where the handler re-read the request body after
// ValidateJSONMiddleware had already consumed it, so every real enroll failed
// with a 400 INVALID_INPUT / "reason":"EOF" even though the handler's own
// unit tests passed (they mounted the handler without the middleware).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func TestDeviceGrantRoutes_EnrollThenExchangeThroughLiveRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	// An empty floor keeps EnforceMinClientVersion inert; the seed requests
	// below don't carry an X-Client-Version header.
	cfg := &config.Config{
		JWTSecretKey:     "device-grant-route-test-secret-key-long-enough",
		JWTExpiryHours:   96,
		ProfilePhotoDir:  t.TempDir(),
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
	}

	owner := models.User{Username: "device-grant-route-owner", Email: "device-grant-route@example.com", Password: "password123"}
	require.NoError(t, db.Create(&owner).Error)
	bearer, err := services.IssueSession(db, owner, cfg, "", "")
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(router, cfg, db, nil)

	// Distinct X-Forwarded-For values put each request in its own process-global
	// rate-limiter bucket, so another test in this package cannot pre-throttle
	// these two calls into a spurious 429.
	enroll := postJSONWithBearer(t, router, "/api/v1/auth/device/grants", `{"label":"route-e2e"}`, bearer, "198.51.100.10")
	require.Equal(t, http.StatusCreated, enroll.Code, enroll.Body.String())

	var created models.DeviceGrantCreateResponse
	require.NoError(t, json.Unmarshal(enroll.Body.Bytes(), &created))
	require.NotEmpty(t, created.Token, "enroll must return the one-time plaintext grant")
	require.Equal(t, "route-e2e", created.Label)

	// Possession of the grant exchanges for a fresh session — the public,
	// rate-limited route. A 200 here proves the body survived the middleware
	// chain and the token was actually looked up.
	exchanged := postJSONWithBearer(t, router, "/api/v1/auth/device/session", `{"device_token":"`+created.Token+`"}`, "", "198.51.100.11")
	require.Equal(t, http.StatusOK, exchanged.Code, exchanged.Body.String())
	require.Contains(t, exchanged.Header().Get("Set-Cookie"), "auth_token=")
}

func postJSONWithBearer(t *testing.T, router http.Handler, path, body, bearer, forwardedFor string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
