package controllers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// androidOIDCScheme is the custom URL scheme the Android app's MainActivity
// intent filter declares (M6 §4 — docs/fork-plan/tickets/85-M6-photo-url-
// user-prefs-oidc.md). When a login flow is started with ?client=android, the
// callback delivers the JWT to <scheme>://oidc/callback?token=…&language=…
// &date_format=… instead of setting the web's httpOnly cookies and redirecting
// to "/" — a native client cannot read a cookie set in a Custom Tab's browser
// context, so the token must travel in the deep link instead.
const androidOIDCScheme = "mycorrhizal"

// oidcErrorRedirect sends the browser to the login-error target for the given
// client: the web SPA's /login?error=<code> for the default flow, or the
// Android app's custom-scheme deep link for the client=android flow — so the
// native client can surface the failure instead of landing on a dead browser
// page. The token itself is never placed in an error redirect (M6 §4's
// security note); only the error code travels, exactly as on web.
func oidcErrorRedirect(c *gin.Context, android bool, code string) {
	target := "/login?error=" + code
	if android {
		target = (&url.URL{
			Scheme:   androidOIDCScheme,
			Host:     "oidc",
			Path:     "callback",
			RawQuery: url.Values{"error": {code}}.Encode(),
		}).String()
	}
	c.Redirect(http.StatusFound, target)
}

// returns whether OIDC is enabled and a provider name hint.
func OIDCConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := gin.H{"enabled": cfg.OIDC.Enabled}
		if cfg.OIDC.Enabled {
			resp["provider_name"] = services.ProviderName(cfg.OIDC.ProviderURL)
		}
		c.JSON(http.StatusOK, resp)
	}
}

// generates a random state and nonce, stores in cookies, then redirects the browser
func OIDCLoginHandler(provider *services.OIDCProvider, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		state, err := services.GenerateStateToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}
		nonce, err := services.GenerateStateToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
			return
		}
		pkceVerifier := services.GeneratePKCEVerifier()

		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("oidc_state", state, 600, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		c.SetCookie("oidc_nonce", nonce, 600, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		c.SetCookie("oidc_pkce", pkceVerifier, 600, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)

		// M6 §4: remember a native-client login start so the callback can
		// deliver the token to the app's deep link instead of the web SPA.
		// The cookie (not just the query param) is what the callback trusts,
		// so an attacker cannot force the token into a redirect they control
		// by appending client=android to a callback URL they cannot otherwise
		// authenticate.
		if c.Query("client") == "android" {
			c.SetCookie("oidc_client", "android", 600, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		}

		c.Redirect(http.StatusFound, provider.BuildAuthURL(state, nonce, pkceVerifier))
	}
}

// handles the provider redirect (validates state, exchanges code, finds/creates user, sets the auth cookie, redirects to /).
func OIDCCallbackHandler(provider *services.OIDCProvider, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.FromContext(c)

		// M6 §4: was this flow started by the native client? The oidc_client
		// cookie is set by /auth/oidc/login?client=android and cleared here
		// like the other one-time cookies. The token is delivered via the app
		// deep link for this flow only — the default flow keeps its exact
		// httpOnly-cookie + "/" behavior.
		android := false
		if client, err := c.Cookie("oidc_client"); err == nil && client == "android" {
			android = true
		}

		// Provider-side errors (e.g. user denied consent)
		if errParam := c.Query("error"); errParam != "" {
			oidcErrorRedirect(c, android, "oidc_denied")
			return
		}

		// Retrieve and immediately clear the state and nonce cookies.
		stateCookie, err := c.Cookie("oidc_state")
		nonceCookie, nonceErr := c.Cookie("oidc_nonce")
		pkceCookie, pkceErr := c.Cookie("oidc_pkce")
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("oidc_state", "", -1, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		c.SetCookie("oidc_nonce", "", -1, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		c.SetCookie("oidc_pkce", "", -1, "/api/v1/auth/oidc/callback", cfg.CookieDomain, cfg.CookieSecure, true)
		c.SetCookie("oidc_client", "", -1, "/api/v1/auth/oidc/callback", cfg.CookieDomain, true, true)

		if err != nil || stateCookie == "" {
			log.Warn().Msg("OIDC callback: missing state cookie")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}
		if nonceErr != nil || nonceCookie == "" {
			log.Warn().Msg("OIDC callback: missing nonce cookie")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}
		if pkceErr != nil || pkceCookie == "" {
			log.Warn().Msg("OIDC callback: missing PKCE verifier cookie")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		stateParam := c.Query("state")
		if subtle.ConstantTimeCompare([]byte(stateCookie), []byte(stateParam)) != 1 {
			log.Warn().Msg("OIDC callback: state mismatch")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		code := c.Query("code")
		if code == "" {
			log.Warn().Msg("OIDC callback: missing code")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		idToken, oauthToken, rawIDToken, err := provider.ExchangeAndVerify(c.Request.Context(), code, pkceCookie)
		if err != nil {
			log.Error().Err(err).Msg("OIDC token exchange/verification failed")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonceCookie)) != 1 {
			log.Warn().Msg("OIDC callback: nonce mismatch")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		claims, err := provider.ClaimsFor(c.Request.Context(), idToken, oauthToken, cfg.OIDC.ProviderURL)
		if err != nil {
			log.Error().Err(err).Msg("OIDC: failed to extract claims")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		db := c.MustGet("db").(*gorm.DB)

		user, err := services.FindOrProvisionUser(db, claims, cfg)
		if err != nil {
			if errors.Is(err, services.ErrOIDCUserNotFound) {
				oidcErrorRedirect(c, android, "oidc_no_account")
				return
			}
			if errors.Is(err, services.ErrOIDCNoEmail) {
				log.Error().Msg("OIDC: provider returned no email; check that the 'email' scope is granted and the UserInfo endpoint is reachable")
				oidcErrorRedirect(c, android, "oidc_no_email")
				return
			}
			log.Error().Err(err).Msg("OIDC: failed to find or provision user")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		tokenString, err := services.GenerateToken(*user, cfg)
		if err != nil {
			log.Error().Err(err).Uint("user_id", user.ID).Msg("OIDC: failed to generate JWT")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		if android {
			// M6 §4: deliver the token to the app's own intent filter via the
			// custom-scheme deep link. The httpOnly auth_token/id_token
			// cookies are deliberately NOT set here — a native client cannot
			// read a cookie set in a Custom Tab's browser context, and
			// minting a browser session it cannot manage would be worse than
			// not minting one. The token is scoped to this one branch: the
			// default flow below never places it in a redirect (M6 §4's
			// security note). state/nonce/PKCE were all verified above,
			// identically for both clients.
			params := url.Values{}
			params.Set("token", tokenString)
			params.Set("language", user.Language)
			params.Set("date_format", user.DateFormat)
			c.Redirect(http.StatusFound, (&url.URL{
				Scheme:   androidOIDCScheme,
				Host:     "oidc",
				Path:     "callback",
				RawQuery: params.Encode(),
			}).String())
			return
		}

		maxAge := cfg.JWTExpiryHours * 3600
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("auth_token", tokenString, maxAge, "/", cfg.CookieDomain, cfg.CookieSecure, true)
		// Retained for RP-Initiated Logout's id_token_hint (LogoutUser) — its
		// presence is also how logout knows this session came via SSO at all.
		c.SetCookie("id_token", rawIDToken, maxAge, "/", cfg.CookieDomain, cfg.CookieSecure, true)

		c.Redirect(http.StatusFound, "/")
	}
}
