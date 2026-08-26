package controllers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

// Issue #412 matrix row 7: post-login redirects must be validated against
// open-redirect. The callback handler never reads a client-supplied redirect
// target at all -- a successful login always lands on the fixed "/" -- so the
// attack this row is guarding against is a future regression that starts
// trusting one. Pin the current, safe behavior: attacker-supplied
// redirect-looking query params on the callback URL must not change the
// destination.
func TestOIDCCallbackHandler_SuccessRedirectIgnoresAttackerSuppliedRedirectParams(t *testing.T) {
	idp := newFakeCallbackIDP(t, "test-client")
	idp.IDTokenClaims["nonce"] = "matching-nonce"
	idp.IDTokenClaims["sub"] = "existing-subject"

	provider, cfg := newCallbackTestSetup(t, idp)

	db, router := setupRouter()
	router.GET("/callback", OIDCCallbackHandler(provider, cfg))

	subject := "existing-subject"
	providerURL := idp.Server.URL
	linkedUser := models.User{
		Username:     "redirect-test",
		Password:     "",
		Email:        "redirect-test@example.com",
		OIDCSubject:  &subject,
		OIDCProvider: &providerURL,
	}
	require.NoError(t, db.Create(&linkedUser).Error)

	req := callbackRequest(fullCookieSet("good-state", "matching-nonce", "good-pkce"), url.Values{
		"state":    {"good-state"},
		"code":     {"auth-code"},
		"redirect": {"https://evil.example.com"},
		"next":     {"//evil.example.com"},
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertRedirectsTo(t, w, "/")
}

// Same check for the Android deep-link flow: the client=android branch must
// also ignore any attacker-supplied redirect-looking params rather than ever
// forwarding them into the deep link it builds.
func TestOIDCCallbackHandler_AndroidSuccessRedirectIgnoresAttackerSuppliedRedirectParams(t *testing.T) {
	idp := newFakeCallbackIDP(t, "test-client")
	idp.IDTokenClaims["nonce"] = "matching-nonce"
	idp.IDTokenClaims["sub"] = "android-existing-subject"

	provider, cfg := newCallbackTestSetup(t, idp)

	db, router := setupRouter()
	router.GET("/callback", OIDCCallbackHandler(provider, cfg))

	subject := "android-existing-subject"
	providerURL := idp.Server.URL
	linkedUser := models.User{
		Username:     "android-redirect-test",
		Password:     "",
		Email:        "android-redirect-test@example.com",
		OIDCSubject:  &subject,
		OIDCProvider: &providerURL,
	}
	require.NoError(t, db.Create(&linkedUser).Error)

	req := callbackRequest(androidCookieSet("good-state", "matching-nonce", "good-pkce"), url.Values{
		"state":    {"good-state"},
		"code":     {"auth-code"},
		"redirect": {"https://evil.example.com"},
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "mycorrhizal", loc.Scheme)
	require.Equal(t, "oidc", loc.Host)
	require.Equal(t, "/callback", loc.Path)
	require.Empty(t, loc.Query().Get("redirect"), "attacker-supplied redirect param must not reach the deep link")
}
