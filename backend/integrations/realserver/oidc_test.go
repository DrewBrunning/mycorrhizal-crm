package realserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/controllers"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The realm in stack/keycloak-realm.json. The redirect/post-logout URIs below
// are registered there; the frontend origin is never contacted (the test
// intercepts the final redirect), it only has to match.
const (
	oidcClientID     = "mycorrhizal-test"
	oidcClientSecret = "mycorrhizal-test-secret"
	oidcFrontend     = "http://127.0.0.1:5173"
	oidcRedirectURL  = oidcFrontend + "/api/v1/auth/oidc/callback"
	oidcLogoutURL    = oidcFrontend + "/login"
	oidcUser         = "oidcuser"
	oidcPassword     = "contract-user-pass"
	oidcEmail        = "oidc-user@example.com"
	oidcAdminUser    = "admin"
	oidcAdminPass    = "contract-admin-pass"
)

// oidcHarness wires our real OIDC handlers (login, callback, logout) to a real
// OIDCProvider that performed discovery against the real IdP.
type oidcHarness struct {
	issuer string
	cfg    *config.Config
	router *gin.Engine
	db     *gorm.DB
}

func newOIDCHarness(t *testing.T) *oidcHarness {
	t.Helper()
	issuer := serverURL(t, envOIDCIssuer)
	waitReady(t, issuer+"/.well-known/openid-configuration", 120*time.Second)

	db, _ := newUser(t)
	cfg := &config.Config{
		JWTSecretKey:   "realserver-contract-test-secret-0123456789",
		JWTExpiryHours: 1,
		FrontendURL:    oidcFrontend,
		OIDC: config.OIDCConfig{
			Enabled:               true,
			ProviderURL:           issuer,
			ClientID:              oidcClientID,
			ClientSecret:          oidcClientSecret,
			RedirectURL:           oidcRedirectURL,
			PostLogoutRedirectURL: oidcLogoutURL,
			AllowAutoProvision:    true,
			Scopes:                []string{"openid", "email", "profile"},
		},
	}
	provider, err := services.InitOIDCProvider(context.Background(), cfg)
	require.NoError(t, err, "discovery against the real IdP")

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	router.GET("/api/v1/auth/oidc/login", controllers.OIDCLoginHandler(provider, cfg))
	router.GET("/api/v1/auth/oidc/callback", controllers.OIDCCallbackHandler(provider, cfg))
	router.POST("/api/v1/logout", func(c *gin.Context) { controllers.LogoutUser(c, cfg, provider) })
	return &oidcHarness{issuer: issuer, cfg: cfg, router: router, db: db}
}

func (h *oidcHarness) serve(method, target string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// idpBrowser is the "user agent" half of the flow: it follows the IdP's own
// redirects with a cookie jar, stops at our callback URL, and fills in the
// IdP's HTML login form like a person would.
type idpBrowser struct {
	t      *testing.T
	client *http.Client
}

func newIdPBrowser(t *testing.T) *idpBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	callback, _ := url.Parse(oidcRedirectURL)
	return &idpBrowser{t: t, client: &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Host == callback.Host { // hand the redirect back to the test
				return http.ErrUseLastResponse
			}
			return nil
		},
	}}
}

var keycloakFormAction = regexp.MustCompile(`<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"`)

// authorize walks authURL through the IdP's login form and returns the final
// redirect back to our callback (its Location carries code+state).
func (b *idpBrowser) authorize(authURL, username, password string) *url.URL {
	b.t.Helper()
	resp, err := b.client.Get(authURL)
	require.NoError(b.t, err)
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		// Already authenticated at the IdP (SSO session): straight back to us.
		loc, err := resp.Location()
		require.NoError(b.t, err)
		return loc
	}
	require.Equal(b.t, http.StatusOK, resp.StatusCode, "IdP login page: %s", page)
	m := keycloakFormAction.FindSubmatch(page)
	require.NotNil(b.t, m, "Keycloak login form not found in IdP response")
	action := html.UnescapeString(string(m[1]))

	resp, err = b.client.PostForm(action, url.Values{"username": {username}, "password": {password}, "credentialId": {""}})
	require.NoError(b.t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(b.t, http.StatusFound, resp.StatusCode, "IdP must redirect to our callback after login: %s", body)
	loc, err := resp.Location()
	require.NoError(b.t, err)
	return loc
}

// login performs the full browser flow against our handlers and returns the
// callback response (session + id_token cookies on success).
func (h *oidcHarness) login(t *testing.T, b *idpBrowser) *httptest.ResponseRecorder {
	t.Helper()
	w := h.serve(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	require.Equal(t, http.StatusFound, w.Code)
	oidcCookies := w.Result().Cookies()

	loc := b.authorize(w.Header().Get("Location"), oidcUser, oidcPassword)
	require.Equal(t, oidcRedirectURL, loc.Scheme+"://"+loc.Host+loc.Path, "IdP must return to the registered redirect URI")
	require.Empty(t, loc.Query().Get("error"), "IdP returned an error: %s", loc.RawQuery)
	return h.serve(http.MethodGet, "/api/v1/auth/oidc/callback?"+loc.RawQuery, oidcCookies)
}

func cookieNamed(cs []*http.Cookie, name string) *http.Cookie {
	for _, c := range cs {
		if c.Name == name && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

// jwtHeaderKid returns the `kid` of a compact JWT (no verification: the point
// is observing which of the IdP's keys signed it).
func jwtHeaderKid(t *testing.T, jwt string) string {
	t.Helper()
	parts := strings.Split(jwt, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var h struct {
		Kid string `json:"kid"`
	}
	require.NoError(t, json.Unmarshal(raw, &h))
	return h.Kid
}

// TestOIDC_AuthCodePKCELoginAgainstRealIdP is the headline contract: our
// /auth/oidc/login + /callback complete a real authorization-code + PKCE (S256)
// login against Keycloak — real discovery, real signed ID token verified from
// the real JWKS, real nonce/state round trip — and provision the user.
func TestOIDC_AuthCodePKCELoginAgainstRealIdP(t *testing.T) {
	h := newOIDCHarness(t)

	w := h.serve(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	require.Equal(t, http.StatusFound, w.Code)
	authURL, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "S256", authURL.Query().Get("code_challenge_method"), "PKCE must be offered")
	assert.NotEmpty(t, authURL.Query().Get("code_challenge"))

	b := newIdPBrowser(t)
	loc := b.authorize(authURL.String(), oidcUser, oidcPassword)
	cb := h.serve(http.MethodGet, "/api/v1/auth/oidc/callback?"+loc.RawQuery, w.Result().Cookies())

	require.Equal(t, http.StatusFound, cb.Code, cb.Body.String())
	assert.Equal(t, "/", cb.Header().Get("Location"), "a successful login lands on / (not /login?error=...)")
	require.NotNil(t, cookieNamed(cb.Result().Cookies(), "auth_token"), "session cookie")
	idTok := cookieNamed(cb.Result().Cookies(), "id_token")
	require.NotNil(t, idTok, "id_token cookie retained for RP-initiated logout")
	assert.NotEmpty(t, jwtHeaderKid(t, idTok.Value))

	var u models.User
	require.NoError(t, h.db.Where("email = ?", oidcEmail).First(&u).Error, "the OIDC user must have been provisioned from the real IdP's claims")
}

// TestOIDC_PKCEVerifierIsEnforcedByRealIdP replays the callback with a
// *different* PKCE verifier cookie: a real IdP must refuse the code exchange
// (it binds the code to the S256 challenge), and we must surface that as the
// oidc_error redirect — never a session. A fake IdP that ignores
// code_verifier would let this pass.
func TestOIDC_PKCEVerifierIsEnforcedByRealIdP(t *testing.T) {
	h := newOIDCHarness(t)

	w := h.serve(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	cookies := w.Result().Cookies()
	b := newIdPBrowser(t)
	loc := b.authorize(w.Header().Get("Location"), oidcUser, oidcPassword)

	tampered := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		cc := *c
		if cc.Name == "oidc_pkce" {
			cc.Value = services.GeneratePKCEVerifier()
		}
		tampered = append(tampered, &cc)
	}
	cb := h.serve(http.MethodGet, "/api/v1/auth/oidc/callback?"+loc.RawQuery, tampered)
	assert.Equal(t, http.StatusFound, cb.Code)
	assert.Equal(t, "/login?error=oidc_error", cb.Header().Get("Location"))
	assert.Nil(t, cookieNamed(cb.Result().Cookies(), "auth_token"), "no session may be minted on a failed exchange")
}

// TestOIDC_RPInitiatedLogoutEndsTheIdPSession covers the user_controller.go
// LogoutUser branch the fake-IdP tests cannot honestly reach: the redirect_url
// we build from the *real* discovery document's end_session_endpoint, with
// id_token_hint + post_logout_redirect_uri, must be accepted by the real IdP
// and actually end its session (prompt=none afterwards -> login_required).
func TestOIDC_RPInitiatedLogoutEndsTheIdPSession(t *testing.T) {
	h := newOIDCHarness(t)
	b := newIdPBrowser(t)

	cb := h.login(t, b)
	require.Equal(t, http.StatusFound, cb.Code, cb.Body.String())
	sessionCookies := cb.Result().Cookies()
	require.NotNil(t, cookieNamed(sessionCookies, "id_token"))

	// Sanity: the IdP session is live — prompt=none is satisfied without a form.
	silent := func() *url.URL {
		auth := h.serve(http.MethodGet, "/api/v1/auth/oidc/login", nil).Header().Get("Location")
		u, err := url.Parse(auth)
		require.NoError(t, err)
		q := u.Query()
		q.Set("prompt", "none")
		u.RawQuery = q.Encode()
		resp, err := b.client.Get(u.String())
		require.NoError(t, err)
		_ = resp.Body.Close()
		loc, err := resp.Location()
		require.NoError(t, err, "prompt=none must redirect back to the client")
		return loc
	}
	before := silent()
	assert.NotEmpty(t, before.Query().Get("code"), "IdP session should be live before logout: %s", before.RawQuery)

	out := h.serve(http.MethodPost, "/api/v1/logout", sessionCookies)
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	var body struct {
		RedirectURL string `json:"redirect_url"`
	}
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &body))
	require.NotEmpty(t, body.RedirectURL, "an SSO session must be given an RP-initiated logout redirect_url")

	redir, err := url.Parse(body.RedirectURL)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(body.RedirectURL, h.issuer+"/protocol/openid-connect/logout"), "redirect_url must come from the discovered end_session_endpoint: %s", body.RedirectURL)
	assert.Equal(t, oidcLogoutURL, redir.Query().Get("post_logout_redirect_uri"))
	assert.NotEmpty(t, redir.Query().Get("id_token_hint"))

	resp, err := b.client.Get(body.RedirectURL)
	require.NoError(t, err)
	logoutBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode, "the real IdP must accept our logout URL and redirect: %s", logoutBody)
	back, err := resp.Location()
	require.NoError(t, err)
	assert.Equal(t, oidcLogoutURL, back.Scheme+"://"+back.Host+back.Path, "IdP must honor post_logout_redirect_uri")

	after := silent()
	assert.Equal(t, "login_required", after.Query().Get("error"), "the IdP session must be gone after RP-initiated logout: %s", after.RawQuery)
}

// keycloakAdminToken fetches a master-realm admin token for the rotation test.
func keycloakAdminToken(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.PostForm(base+"/realms/master/protocol/openid-connect/token", url.Values{ //nolint:noctx // fixed local test server
		"grant_type": {"password"}, "client_id": {"admin-cli"},
		"username": {oidcAdminUser}, "password": {oidcAdminPass},
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(raw, &tok))
	return tok.AccessToken
}

func keycloakAdmin(t *testing.T, method, target, token string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target, rdr)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// TestOIDC_SigningKeyRotationIsPickedUp rotates the realm's signing key on the
// real IdP *after* our provider cached the JWKS, then logs in again with the
// same long-lived OIDCProvider. The new ID token carries an unknown kid; the
// verifier must refetch the JWKS rather than rejecting every login until the
// process restarts — the failure an in-test static RSA key can never show.
func TestOIDC_SigningKeyRotationIsPickedUp(t *testing.T) {
	h := newOIDCHarness(t)
	base := strings.TrimSuffix(h.issuer, "/realms/mycorrhizal")

	first := h.login(t, newIdPBrowser(t))
	require.Equal(t, http.StatusFound, first.Code, first.Body.String())
	oldKid := jwtHeaderKid(t, cookieNamed(first.Result().Cookies(), "id_token").Value)

	adminTok := keycloakAdminToken(t, base)
	status, raw := keycloakAdmin(t, http.MethodGet, base+"/admin/realms/mycorrhizal", adminTok, nil)
	require.Equal(t, http.StatusOK, status, string(raw))
	var realm struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(raw, &realm))

	// A new, higher-priority RSA key provider: Keycloak signs with it from now on.
	name := fmt.Sprintf("rotated-rsa-%d", time.Now().UnixNano())
	status, raw = keycloakAdmin(t, http.MethodPost, base+"/admin/realms/mycorrhizal/components", adminTok, map[string]any{
		"name": name, "providerId": "rsa-generated", "providerType": "org.keycloak.keys.KeyProvider",
		"parentId": realm.ID,
		"config":   map[string][]string{"priority": {"1000000"}, "enabled": {"true"}, "active": {"true"}, "keySize": {"2048"}},
	})
	require.Equal(t, http.StatusCreated, status, string(raw))

	second := h.login(t, newIdPBrowser(t))
	require.Equal(t, http.StatusFound, second.Code, "login after key rotation: %s", second.Body.String())
	assert.Equal(t, "/", second.Header().Get("Location"), "rotation must not turn into oidc_error")
	newKid := jwtHeaderKid(t, cookieNamed(second.Result().Cookies(), "id_token").Value)
	assert.NotEqual(t, oldKid, newKid, "the IdP must actually have signed with the rotated key")
}
