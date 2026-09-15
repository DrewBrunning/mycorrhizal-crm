package controllers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// androidOIDCScheme is the custom URL scheme the Android app's MainActivity
// intent filter declares (M6). When a login flow is started with ?client=android,
// the callback redirects to <scheme>://oidc/callback instead of setting the
// web's httpOnly cookies and redirecting to "/" — a native client cannot read a
// cookie set in a Custom Tab's browser context.
//
// Issue #965: the redirect carries only a short-lived, PKCE-bound exchange code
// plus the app's own state nonce, never the session JWT. The custom scheme is
// interceptable (any app can register it), so the code is bound to a PKCE
// challenge the app generated and redeems over a direct HTTPS request; see
// services.MintOIDCNativeExchangeCode and OIDCNativeExchangeHandler.
const androidOIDCScheme = "mycorrhizal"

// oidcCallbackPath is the path every OIDC handshake cookie is scoped to. Kept
// as one constant so the new native-PKCE cookies (#965) cannot drift from the
// IdP handshake cookies they sit beside.
const oidcCallbackPath = "/api/v1/auth/oidc/callback"

// Native-client PKCE cookies (issue #965): set by /auth/oidc/login?client=android
// from the app's query parameters and consumed at the callback. They bind the
// deep-link exchange code to the app that started the flow.
const (
	oidcAppStateCookie     = "oidc_app_state"
	oidcAppChallengeCookie = "oidc_app_challenge"
)

// setOIDCCookie sets one of this controller's cookies — the transient handshake
// set (login start, and again cleared at the callback) or the session cookies
// the callback mints. They all share the same deliberate flags: HttpOnly
// always, SameSite set by the caller, and Secure = cfg.CookieSecure.
//
// Secure deliberately follows config rather than being forced true:
// COOKIE_SECURE=false is a supported plain-HTTP self-hosted deployment, and
// hardcoding true there made a browser reject the cookie outright (issue #605).
// The operator owns that transport decision (docs/deployment.md), so CodeQL's
// generic finding is a false positive here.
func setOIDCCookie(c *gin.Context, cfg *config.Config, name, value, path string, maxAge int) {
	// codeql[go/cookie-secure-not-set]
	c.SetCookie(name, value, maxAge, path, cfg.CookieDomain, cfg.CookieSecure, true)
}

// oidcErrorRedirect sends the browser to the login-error target for the given
// client: the web SPA's /login?error=<code> for the default flow, or the
// Android app's custom-scheme deep link for the client=android flow — so the
// native client can surface the failure instead of landing on a dead browser
// page. The token itself is never placed in an error redirect (M6's
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

// OIDCConfigHandler is the one public, unauthenticated "what can a client do
// on this deployment" endpoint: whether OIDC is enabled (+ a provider name
// hint), and whether self-registration is disabled (DISABLE_REGISTRATION).
// Both web's LoginPage/RegisterPage and Android's login/register screens
// call it to decide what to show *before* a POST to /login or /register
// would fail — previously registration_disabled was enforced only inside
// RegisterUser's 403, so the register form/link stayed visible (and usable
// right up to submit) even with registration turned off server-side.
func OIDCConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := gin.H{
			"enabled":               cfg.OIDC.Enabled,
			"registration_disabled": cfg.RegistrationDisabled,
		}
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

		// Issue #392: these cookies must stay Lax, not Strict, unlike the
		// session cookies set below at the callback. The browser reaches
		// /auth/oidc/callback via a cross-site top-level redirect *from the
		// IdP* — a Strict cookie would never be attached to that request, so
		// the callback would always see it missing and fail every OIDC
		// login.
		c.SetSameSite(http.SameSiteLaxMode)
		setOIDCCookie(c, cfg, "oidc_state", state, oidcCallbackPath, 600)
		setOIDCCookie(c, cfg, "oidc_nonce", nonce, oidcCallbackPath, 600)
		setOIDCCookie(c, cfg, "oidc_pkce", pkceVerifier, oidcCallbackPath, 600)

		// M6: remember a native-client login start so the callback can
		// deliver back to the app's deep link instead of the web SPA.
		// The cookie (not just the query param) is what the callback trusts,
		// so an attacker cannot force a redirect they control by appending
		// client=android to a callback URL they cannot otherwise
		// authenticate.
		if c.Query("client") == "android" {
			// Issue #965: fail closed unless the app supplied its own state
			// nonce and an S256 PKCE challenge. The exchange code the callback
			// returns is only redeemable with the matching code verifier, so a
			// caller that omits these cannot get a usable credential delivered
			// to an interceptable custom-scheme URI. No verifier is sent here —
			// only its hash — so a leaked handshake URL is still not enough to
			// redeem a later code.
			appState := c.Query("state")
			codeChallenge := c.Query("code_challenge")
			challengeMethod := c.Query("code_challenge_method")
			if challengeMethod == "" {
				challengeMethod = "S256"
			}
			if appState == "" || codeChallenge == "" || challengeMethod != "S256" {
				log := logger.FromContext(c)
				log.Warn().Msg("OIDC login: android client without native state/PKCE binding")
				oidcErrorRedirect(c, true, "oidc_error")
				return
			}
			// Issue #605: these cookies follow cfg.CookieSecure via the shared
			// helper, so a plain-HTTP deployment (COOKIE_SECURE=false) does not
			// have its browser reject them.
			setOIDCCookie(c, cfg, "oidc_client", "android", oidcCallbackPath, 600)
			setOIDCCookie(c, cfg, oidcAppStateCookie, appState, oidcCallbackPath, 600)
			setOIDCCookie(c, cfg, oidcAppChallengeCookie, codeChallenge, oidcCallbackPath, 600)
		}

		c.Redirect(http.StatusFound, provider.BuildAuthURL(state, nonce, pkceVerifier))
	}
}

// handles the provider redirect (validates state, exchanges code, finds/creates user, sets the auth cookie, redirects to /).
func OIDCCallbackHandler(provider *services.OIDCProvider, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.FromContext(c)

		// M6: was this flow started by the native client? The oidc_client
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

		// Retrieve and immediately clear the state and nonce cookies. The
		// native-PKCE cookies (#965) are read here too so they are cleared on
		// every callback and available to the android success branch below.
		stateCookie, err := c.Cookie("oidc_state")
		nonceCookie, nonceErr := c.Cookie("oidc_nonce")
		pkceCookie, pkceErr := c.Cookie("oidc_pkce")
		appState, appStateErr := c.Cookie(oidcAppStateCookie)
		appChallenge, appChallengeErr := c.Cookie(oidcAppChallengeCookie)
		c.SetSameSite(http.SameSiteLaxMode)
		for _, name := range []string{
			"oidc_state", "oidc_nonce", "oidc_pkce", "oidc_client",
			oidcAppStateCookie, oidcAppChallengeCookie,
		} {
			setOIDCCookie(c, cfg, name, "", oidcCallbackPath, -1)
		}

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
		// Issue #965: a native flow without the app's state + PKCE binding
		// cannot produce a redeemable code, so fail it here rather than
		// handing anything usable to an interceptable custom-scheme redirect.
		if android && (appStateErr != nil || appState == "" || appChallengeErr != nil || appChallenge == "") {
			log.Warn().Msg("OIDC callback: android flow missing native state/PKCE binding")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		stateParam := c.Query("state")
		if subtle.ConstantTimeCompare([]byte(stateCookie), []byte(stateParam)) != 1 {
			log.Warn().Msg("OIDC callback: state mismatch")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		// OAuth2/OIDC authorization code callback (RFC 6749 4.1.2): the IdP
		// redirects here with ?code=... by spec, so this can't move to the
		// body/header. The code is single-use and short-lived, and is
		// immediately exchanged for tokens server-side below rather than
		// stored or logged.
		code := c.Query("code") // nosemgrep: mycorrhizal-query-string-auth-material
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

		// T18 audit: authentication via the identity provider (issue #381).
		// The web flow completes here (it mints the session below); the native
		// flow completes when the app redeems its code at
		// OIDCNativeExchangeHandler, which records the same event.
		if !android {
			models.RecordAuditEvent(models.AuditEntityAuth, user.Username, models.AuditOpLogin, user.ID)
		}

		if android {
			// #965: hand the app a short-lived code bound to the PKCE challenge
			// it supplied — never the session JWT. The custom scheme is
			// interceptable, so even a stolen redirect is useless without the
			// code verifier that never leaves the app. The
			// httpOnly auth_token/id_token cookies are deliberately NOT set:
			// a native client cannot read a cookie set in a Custom Tab's
			// browser context, and minting a browser session it cannot manage
			// would be worse than not minting one. state/nonce/PKCE between
			// this server and the IdP were all verified above, identically for
			// both clients.
			nativeCode, err := services.MintOIDCNativeExchangeCode(*user, appChallenge, cfg)
			if err != nil {
				log.Error().Err(err).Uint("user_id", user.ID).Msg("OIDC: failed to mint native exchange code")
				oidcErrorRedirect(c, android, "oidc_error")
				return
			}
			params := url.Values{}
			params.Set("code", nativeCode)
			params.Set("state", appState)
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

		tokenString, err := services.IssueSession(db, *user, cfg, c.Request.UserAgent(), c.ClientIP())
		if err != nil {
			log.Error().Err(err).Uint("user_id", user.ID).Msg("OIDC: failed to generate JWT")
			oidcErrorRedirect(c, android, "oidc_error")
			return
		}

		maxAge := cfg.JWTExpiryHours * 3600
		// Issue #392: Strict, unlike the oidc_state/nonce/pkce cookies above.
		// This redirect to "/" is same-origin (the callback is already on
		// this app's own domain), and every read of this cookie afterward is
		// a same-origin XHR/fetch from the loaded SPA — never a top-level
		// cross-site navigation — so Strict costs nothing here.
		c.SetSameSite(http.SameSiteStrictMode)
		setOIDCCookie(c, cfg, "auth_token", tokenString, "/", maxAge)
		// Retained for RP-Initiated Logout's id_token_hint (LogoutUser) — its
		// presence is also how logout knows this session came via SSO at all.
		setOIDCCookie(c, cfg, "id_token", rawIDToken, "/", maxAge)

		c.Redirect(http.StatusFound, "/")
	}
}

// OIDCNativeExchangeHandler redeems the Android OIDC native-return code for a
// session JWT (issue #965). It is public and rate-limited like /login: the code
// is short-lived and bound to the app's PKCE verifier, so possession alone is
// not enough to redeem it (a stolen deep link yields an unusable code). On success it mints the session here — the callback no longer
// does for the native flow — and returns the JWT in the body rather than a
// cookie, since Android has no cookie jar.
func OIDCNativeExchangeHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.FromContext(c)

		input, appErr := middleware.GetValidated[models.OIDCNativeExchangeInput](c)
		if appErr != nil {
			apperrors.AbortWithError(c, appErr)
			return
		}

		userID, codeChallenge, ok := services.ParseOIDCNativeExchangeCode(input.Code, cfg)
		if !ok {
			apperrors.AbortWithError(c, apperrors.ErrInvalidCredentials())
			return
		}
		// The code verifier is the whole point: an interceptor of the deep link
		// has the code but not this, so it cannot redeem. Constant-time inside.
		if !services.VerifyOIDCNativePKCE(input.CodeVerifier, codeChallenge) {
			log.Warn().Uint("user_id", userID).Msg("OIDC native exchange: PKCE verifier mismatch")
			apperrors.AbortWithError(c, apperrors.ErrInvalidCredentials())
			return
		}

		db := c.MustGet("db").(*gorm.DB)
		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidCredentials())
			return
		}

		tokenString, err := services.IssueSession(db, user, cfg, c.Request.UserAgent(), c.ClientIP())
		if err != nil {
			log.Error().Err(err).Uint("user_id", user.ID).Msg("OIDC native exchange: failed to issue session")
			apperrors.AbortWithError(c, apperrors.ErrInternal("Could not generate token").WithError(err)) // # pragma: no cover — a session INSERT failing after the user row was read needs a failing store
			return
		}

		// T18 audit: the native flow completes here, so the login event is
		// recorded at the point the session actually exists (the callback left
		// it unrecorded for this client — see OIDCCallbackHandler).
		models.RecordAuditEvent(models.AuditEntityAuth, user.Username, models.AuditOpLogin, user.ID)

		c.JSON(http.StatusOK, models.OIDCNativeExchangeResponse{
			Token:      tokenString,
			Language:   user.Language,
			DateFormat: user.DateFormat,
		})
	}
}
