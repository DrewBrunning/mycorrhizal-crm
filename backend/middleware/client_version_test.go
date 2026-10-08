package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newClientVersionTestRouter builds a router with EnforceMinClientVersion(cfg)
// in front of a recorder handler that proves whether the request passed the
// gate.
func newClientVersionTestRouter(cfg *config.Config) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.ReleaseMode)
	var passed bool
	router := gin.New()
	router.Use(EnforceMinClientVersion(cfg))
	router.GET("/probe", func(c *gin.Context) {
		passed = true
		c.JSON(http.StatusOK, gin.H{"passed": passed})
	})
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/probe", nil)
	router.ServeHTTP(rec, req)
	return router, rec
}

func doProbe(t *testing.T, router *gin.Engine, clientVersion string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "/probe", nil)
	if clientVersion != "" {
		req.Header.Set(ClientVersionHeader, clientVersion)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// --- no-floor default posture (the policy default: everything passes) -----

// assertProbePassed asserts the request reached the downstream handler.
func assertProbePassed(t *testing.T, rec *httptest.ResponseRecorder, msg string) {
	t.Helper()
	assert.Equal(t, http.StatusOK, rec.Code, msg)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), msg)
	assert.Equal(t, true, body["passed"], "downstream handler must have run: "+msg)
}

// assertProbeRefused asserts a 403 CLIENT_NOT_SUPPORTED and that the handler
// did not run.
func assertProbeRefused(t *testing.T, rec *httptest.ResponseRecorder, msg string) {
	t.Helper()
	assert.Equal(t, http.StatusForbidden, rec.Code, msg)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), msg)
	assert.NotContains(t, body, "passed", "downstream handler must not have run: "+msg)
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok, msg)
	assert.Equal(t, "CLIENT_NOT_SUPPORTED", errObj["code"], msg)
}

func TestEnforceMinClientVersion_NoFloorIsInert(t *testing.T) {
	cfg := &config.Config{MinClientVersion: ""}
	router, _ := newClientVersionTestRouter(cfg)

	for _, clientVersion := range []string{"", "0.5.0", "0.6.0", "0.9.9", "garbage", "v0.6.0", "0.6.0-rc.1"} {
		rec := doProbe(t, router, clientVersion)
		assertProbePassed(t, rec, "client version "+clientVersion+" must pass when no floor is declared")
	}
}

// --- floor declared: below-floor / absent / invalid versions are refused ---

func TestEnforceMinClientVersion_RejectsBelowFloor(t *testing.T) {
	cfg := &config.Config{MinClientVersion: "0.6.0"}
	router, _ := newClientVersionTestRouter(cfg)

	for _, clientVersion := range []string{"0.5.9", "0.5.0", "0.0.1", "0.6.0-0"} {
		rec := doProbe(t, router, clientVersion)
		assert.Equal(t, http.StatusForbidden, rec.Code, "client %q must be refused below floor 0.6.0", clientVersion)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		errObj := body["error"].(map[string]any)
		assert.Equal(t, "CLIENT_NOT_SUPPORTED", errObj["code"])
		assert.Contains(t, errObj["message"].(string), "0.6.0")
	}
}

func TestEnforceMinClientVersion_AcceptsAtOrAboveFloor(t *testing.T) {
	cfg := &config.Config{MinClientVersion: "0.6.0"}
	router, _ := newClientVersionTestRouter(cfg)

	for _, clientVersion := range []string{"0.6.0", "0.6.1", "0.7.0", "1.0.0", "10.0.0"} {
		rec := doProbe(t, router, clientVersion)
		assertProbePassed(t, rec, "client "+clientVersion+" must pass at/above floor 0.6.0")
	}
}

// --- the issue #692 "anything other than major.minor.patch is an invalid
// version" rule -------------------------------------------------------------

func TestEnforceMinClientVersion_InvalidVersionStringsAreRejected(t *testing.T) {
	cfg := &config.Config{MinClientVersion: "0.6.0"}
	router, _ := newClientVersionTestRouter(cfg)

	invalid := []string{
		"",            // absent header entirely
		"   ",         // blank after trim
		"garbage",     // not a version
		"v0.6.0",      // leading v is not strict major.minor.patch
		"0.6",         // missing patch
		"0.6.0-rc.1",  // prerelease suffix is not strict
		"0.6.0+build", // build metadata is not strict
		"0.6.0.1",     // four segments
		"0..0",        // empty segment
		".6.0",        // empty major
		"0.6.",        // empty patch
		"0.6.x",       // non-numeric
		"0.6.0x",      // trailing junk
		"0x6.0",       // non-decimal
		"abc.def.ghi",
	}
	for _, clientVersion := range invalid {
		rec := doProbe(t, router, clientVersion)
		assertProbeRefused(t, rec, "client version "+clientVersion+" must be treated as invalid")
	}
}

// --- a loose (major.minor) floor still compares numerically ----------------

func TestEnforceMinClientVersion_LooseFloorFormat(t *testing.T) {
	cfg := &config.Config{MinClientVersion: "0.6"}
	router, _ := newClientVersionTestRouter(cfg)

	assertProbePassed(t, doProbe(t, router, "0.6.0"), "0.6.0")
	assertProbePassed(t, doProbe(t, router, "0.6.9"), "0.6.9")
	assertProbeRefused(t, doProbe(t, router, "0.5.9"), "0.5.9")
}

// --- strict parsing helpers ------------------------------------------------

func TestParseStrictClientVersion(t *testing.T) {
	valid := map[string]triple{
		"0.6.0":    {major: 0, minor: 6, patch: 0},
		"1.2.3":    {major: 1, minor: 2, patch: 3},
		"10.20.30": {major: 10, minor: 20, patch: 30},
	}
	for raw, want := range valid {
		got, ok := parseStrictClientVersion(raw)
		assert.True(t, ok, raw)
		assert.Equal(t, want, got, raw)
	}

	for _, raw := range []string{"", "0.6", "0.6.0.0", "v0.6.0", "0.6.0-rc.1", "0.6.x", "abc"} {
		_, ok := parseStrictClientVersion(raw)
		assert.False(t, ok, raw)
	}
}

func TestParseClientVersionTriple_ToleratesFloorShapes(t *testing.T) {
	valid := map[string]triple{
		"0.6.0":       {major: 0, minor: 6, patch: 0},
		"0.6":         {major: 0, minor: 6, patch: 0},
		"6":           {major: 6, minor: 0, patch: 0},
		"v0.6.0":      {major: 0, minor: 6, patch: 0},
		"0.6.0-rc.1":  {major: 0, minor: 6, patch: 0},
		"0.6.0+build": {major: 0, minor: 6, patch: 0},
	}
	for raw, want := range valid {
		got, ok := parseClientVersionTriple(raw)
		assert.True(t, ok, raw)
		assert.Equal(t, want, got, raw)
	}

	for _, raw := range []string{"", "   ", "abc", "0.6.0.1", "0..0"} {
		_, ok := parseClientVersionTriple(raw)
		assert.False(t, ok, raw)
	}
}

func TestVersionTripleLess(t *testing.T) {
	a := triple{major: 0, minor: 6, patch: 0}
	assert.False(t, versionTripleLess(a, a))
	assert.True(t, versionTripleLess(triple{0, 5, 9}, a))
	assert.False(t, versionTripleLess(triple{0, 6, 1}, a))
	assert.False(t, versionTripleLess(triple{1, 0, 0}, a))
	assert.True(t, versionTripleLess(triple{0, 6, 0}, triple{0, 6, 1}))
}
