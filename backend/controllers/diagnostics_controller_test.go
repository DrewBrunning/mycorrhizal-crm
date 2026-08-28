package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diagnosticsTestConfig is a config that passes config.Validate() so the
// endpoint's sweep starts from an all-ok baseline (the authz boundary itself is
// the authorization-matrix test's job; this test owns the endpoint shape).
func diagnosticsTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		JWTSecretKey:     "diagnostics-controller-secret-key-that-is-long-enough",
		JWTExpiryHours:   96,
		DBPath:           filepath.Join(t.TempDir(), "myco.db"),
		ProfilePhotoDir:  t.TempDir(),
		AttachmentsDir:   t.TempDir(),
		Port:             "8080",
		ReminderTime:     "06:00",
		ReminderTimezone: "UTC",
		FrontendURL:      "http://localhost:5173",
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

func TestRunDiagnosticsEndpoint(t *testing.T) {
	db := dbtest.New(t)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", diagnosticsTestConfig(t))
		c.Next()
	})
	router.GET("/admin/diagnostics", RunDiagnostics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/diagnostics", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp services.Diagnostics
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Summary.Status)
	assert.Zero(t, resp.Summary.Errors)
	assert.Zero(t, resp.Summary.Warnings)
	assert.NotEmpty(t, resp.Summary.OK)
	assert.NotEmpty(t, resp.Checks)

	// Every check row has the shape the frontend consumes.
	for _, c := range resp.Checks {
		assert.NotEmpty(t, c.Name)
		assert.Contains(t, []string{services.DiagStatusOK, services.DiagStatusWarning, services.DiagStatusError}, c.Status)
		assert.NotEmpty(t, c.Message)
	}
}

func TestRunDiagnosticsEndpointReportsProblems(t *testing.T) {
	db := dbtest.New(t)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		cfg := diagnosticsTestConfig(t)
		cfg.AttachmentsDir = filepath.Join(t.TempDir(), "missing-attachments")
		c.Set("cfg", cfg)
		c.Next()
	})
	router.GET("/admin/diagnostics", RunDiagnostics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/diagnostics", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// The endpoint still reports 200 when the install is unhealthy — the
	// diagnosis IS the payload, not the status code (degraded-but-alive).
	var resp services.Diagnostics
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Summary.Status)
	assert.Equal(t, 1, resp.Summary.Errors)
	var filesystem services.DiagnosticCheck
	for _, c := range resp.Checks {
		if c.Name == "filesystem" {
			filesystem = c
		}
	}
	assert.Equal(t, services.DiagStatusError, filesystem.Status)
}
