package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/buildinfo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /health used to report `Version: "0.1.0"` as a hardcoded string literal, so
// every build ever produced claimed the same version. With several alpha
// candidates in circulation that makes a bug report untraceable to the binary
// that produced it — you cannot tell which build the user is running.
//
// The value now comes from the buildinfo package, injected via -ldflags by the
// Makefile and the Dockerfile. This asserts the endpoint actually reports what
// buildinfo carries, rather than a literal.
func TestHealthCheck_ReportsBuildInfoNotAHardcodedVersion(t *testing.T) {
	_, _, router := migratedHealthRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	expected := buildinfo.Get()
	assert.Equal(t, expected.Version, resp.Version,
		"the reported version must come from buildinfo, not a literal in the handler")
	assert.Equal(t, expected.Commit, resp.Commit)
	assert.NotEmpty(t, resp.Version)
}

// Guards the regression directly: if someone re-hardcodes a version string,
// this catches it even if buildinfo happens to hold the same value in a test
// build.
func TestHealthCheck_VersionIsNotTheOldLiteral(t *testing.T) {
	_, _, router := migratedHealthRouter(t)

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// An un-stamped test binary reports buildinfo's "dev" default. Seeing the
	// old literal here means the handler stopped consulting buildinfo.
	assert.NotEqual(t, "0.1.0", resp.Version,
		`"0.1.0" was the hardcoded literal this endpoint used to return`)
}

func TestBuildInfoGet_DefaultsToDevWhenNotStamped(t *testing.T) {
	// The package defaults are what a `go test` binary carries: deliberately
	// "dev" rather than a plausible-looking release number, so an unstamped
	// build is obvious in a bug report instead of masquerading as a release.
	assert.Equal(t, "dev", buildinfo.Version)

	info := buildinfo.Get()
	assert.NotEmpty(t, info.Version)
}
