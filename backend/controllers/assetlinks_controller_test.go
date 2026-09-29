package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveAssetLinks(cfg *config.Config) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/.well-known/assetlinks.json", AssetLinksHandler(cfg))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil))
	return w
}

func TestAssetLinksHandler_EffectiveGoldenBodyAndHeaders(t *testing.T) {
	cfg := &config.Config{
		FrontendURL:               "https://crm.example.com",
		WebAuthnAndroidEnabled:    true,
		WebAuthnAndroidCertSHA256: []string{androidTestCertColon},
	}
	w := serveAssetLinks(cfg)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Empty(t, w.Header().Values("Set-Cookie"))
	assert.Empty(t, w.Header().Get("Location"))
	assert.Equal(t,
		`[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"com.mycorrhizal.crm","sha256_cert_fingerprints":["`+androidObtainiumColon+`","`+androidTestCertColon+`"]}}]`,
		w.Body.String())
}

func TestAssetLinksHandler_NotEffectiveIs404WithNoBody(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"switch off": {FrontendURL: "https://crm.example.com", WebAuthnAndroidCertSHA256: []string{androidTestCertColon}},
		"http url":   {FrontendURL: "http://crm.example.com", WebAuthnAndroidEnabled: true, WebAuthnAndroidCertSHA256: []string{androidTestCertColon}},
		"embedded":   {Deployment: config.DeploymentEmbedded, FrontendURL: "https://crm.example.com", WebAuthnAndroidEnabled: true, WebAuthnAndroidCertSHA256: []string{androidTestCertColon}},
	} {
		w := serveAssetLinks(cfg)
		assert.Equal(t, http.StatusNotFound, w.Code, name)
		assert.Empty(t, w.Body.String(), name)
		assert.Empty(t, w.Header().Values("Set-Cookie"), name)
	}
}
