package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2101 (ADR 0038): the sign-in, 2FA and passkey handlers carry a
// reason_code in the raw JSON so clients can localize the message. These tests
// read the wire body, not the Go value, so a dropped .WithReason fails them.

// wireReason decodes the envelope's reason_code/reason_params from a response.
func wireReason(t *testing.T, w *httptest.ResponseRecorder) (string, map[string]any) {
	t.Helper()
	var body struct {
		Error struct {
			ReasonCode   string         `json:"reason_code"`
			ReasonParams map[string]any `json:"reason_params"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	return body.Error.ReasonCode, body.Error.ReasonParams
}

func requireReason(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, w.Code, "body: %s", w.Body.String())
	got, _ := wireReason(t, w)
	assert.Equal(t, code, got, "body: %s", w.Body.String())
}

func rawRequest(method, path, body, token string) *http.Request {
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	}
	return req
}

func TestSignInReasonCodes_LockoutBodyIsAdditive(t *testing.T) {
	assertLocked := func(t *testing.T, w *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusTooManyRequests, w.Code, "body: %s", w.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "Account temporarily locked", body["error"], "existing key kept")
		assert.NotEmpty(t, body["message"], "existing key kept")
		assert.NotNil(t, body["retry_after"], "existing key kept")
		assert.NotEmpty(t, body["retry_after_at"], "existing key kept")
		assert.Equal(t, "auth_account_locked", body["reason_code"])
		params, ok := body["reason_params"].(map[string]any)
		require.True(t, ok, "reason_params must be an object: %s", w.Body.String())
		assert.Equal(t, body["retry_after"], params["retry_after"], "param mirrors retry_after")
		assert.Len(t, params, 1)
	}

	t.Run("password login", func(t *testing.T) {
		cfg := config.Config{JWTSecretKey: "mysecretkey", JWTExpiryHours: 24}
		db, router := setupRouter(t)
		router.POST("/login", func(c *gin.Context) { LoginUser(c, &cfg) })
		u := models.User{Username: "lock2101", Email: "lock2101@example.com"}
		u.Password, _ = services.HashPassword(strongPassword)
		require.NoError(t, db.Create(&u).Error)
		const ip = "203.0.113.101"
		var w *httptest.ResponseRecorder
		for i := 0; i < middleware.MaxLoginAttempts; i++ {
			w = loginFrom(router, ip, "lock2101", "nope")
		}
		assertLocked(t, w)                                         // the attempt that trips the limit
		assertLocked(t, loginFrom(router, ip, "lock2101", "nope")) // already locked
	})

	t.Run("2fa code step", func(t *testing.T) {
		e := newWAEnv(t)
		_, tok := e.enableTOTP(e.session())
		_ = tok
		pending, _ := e.passwordStep()
		var w *httptest.ResponseRecorder
		for i := 0; i < middleware.MaxLoginAttempts; i++ {
			w, _ = doRequest(e.router, withPending(mustPost("/login/2fa", map[string]string{"code": "000000"}), pending))
		}
		assertLocked(t, w)
		w, _ = doRequest(e.router, withPending(mustPost("/login/2fa", map[string]string{"code": "000000"}), pending))
		assertLocked(t, w)
	})

	t.Run("session proof (abortLocked)", func(t *testing.T) {
		e, tok, _ := proofEnv(t)
		var w *httptest.ResponseRecorder
		for i := 0; i < middleware.MaxLoginAttempts; i++ {
			w, _ = e.do("POST", "/users/2fa/disable", map[string]string{"code": wrongProof}, tok)
		}
		assertLocked(t, w)
	})
}

func TestSignInReasonCodes_UserController(t *testing.T) {
	t.Run("breached password on register", func(t *testing.T) {
		server := newHIBPServer(t, strongPassword)
		defer server.Close()
		t.Cleanup(services.SetHIBPAPIBaseURLForTest(server.URL))
		_, router := setupRouter(t)
		cfg := &config.Config{HIBPCheckEnabled: true}
		router.POST("/register", middleware.ValidateJSONMiddleware(&models.UserRegistrationInput{}), RegisterUser(cfg))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, rawRequest("POST", "/register",
			`{"username":"r2101","email":"r2101@example.com","password":"`+strongPassword+`"}`, ""))
		requireReason(t, w, http.StatusBadRequest, "auth_password_breached")
	})

	t.Run("password too long on register", func(t *testing.T) {
		_, router := setupRouter(t)
		cfg := &config.Config{}
		router.POST("/register", func(c *gin.Context) {
			c.Set("validated", &models.UserRegistrationInput{
				Username: "long2101", Email: "long2101@example.com", Password: strings.Repeat("aB3!", 19),
			})
			RegisterUser(cfg)(c)
		})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, rawRequest("POST", "/register", `{}`, ""))
		requireReason(t, w, http.StatusBadRequest, "auth_password_too_long")
	})

	t.Run("demo mode blocks password reset", func(t *testing.T) {
		_, router := setupRouter(t)
		cfg := &config.Config{DemoMode: true}
		router.POST("/request", func(c *gin.Context) { RequestPasswordReset(c, cfg) })
		router.POST("/confirm", func(c *gin.Context) { ConfirmPasswordReset(c, cfg) })
		for _, path := range []string{"/request", "/confirm"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, rawRequest("POST", path, `{}`, ""))
			requireReason(t, w, http.StatusForbidden, "auth_password_change_disabled_demo")
		}
	})

	t.Run("reset token invalid and breached password on confirm", func(t *testing.T) {
		db, router := setupRouter(t)
		cfg := &config.Config{}
		in := &models.PasswordResetConfirmInput{Token: strings.Repeat("u", 24), Password: strongPassword}
		router.POST("/confirm", func(c *gin.Context) {
			c.Set("validated", in)
			ConfirmPasswordReset(c, cfg)
		})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, rawRequest("POST", "/confirm", `{}`, ""))
		requireReason(t, w, http.StatusBadRequest, "auth_reset_token_invalid")

		// A live token reaches the password checks.
		token := strings.Repeat("k", 24)
		hash := services.HashPasswordResetToken(token)
		exp := services.Now().Add(time.Hour)
		require.NoError(t, db.Create(&models.User{
			Username: "rs2101", Email: "rs2101@example.com", Password: "x",
			PasswordResetTokenHash: &hash, PasswordResetExpiresAt: &exp,
		}).Error)
		in.Token = token
		in.Password = strings.Repeat("aB3!", 19)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, rawRequest("POST", "/confirm", `{}`, ""))
		requireReason(t, w, http.StatusBadRequest, "auth_password_too_long")

		server := newHIBPServer(t, strongPassword)
		defer server.Close()
		t.Cleanup(services.SetHIBPAPIBaseURLForTest(server.URL))
		cfg.HIBPCheckEnabled = true
		in.Password = strongPassword
		w = httptest.NewRecorder()
		router.ServeHTTP(w, rawRequest("POST", "/confirm", `{}`, ""))
		requireReason(t, w, http.StatusBadRequest, "auth_password_breached")
	})
}

func TestSignInReasonCodes_TwoFactor(t *testing.T) {
	t.Run("already enabled", func(t *testing.T) {
		e := newWAEnv(t)
		_, tok := e.enableTOTP(e.session())
		w, _ := e.do("POST", "/users/2fa/setup", nil, tok)
		requireReason(t, w, http.StatusConflict, "auth_two_factor_already_enabled")
	})

	t.Run("oidc account", func(t *testing.T) {
		e := newWAEnv(t)
		sub := "oidc-sub-2101"
		require.NoError(t, e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("oidc_subject", sub).Error)
		w, _ := e.do("POST", "/users/2fa/setup", nil, e.session())
		requireReason(t, w, http.StatusForbidden, "auth_two_factor_unavailable_oidc")
		w, _ = e.do("POST", "/webauthn/register/begin", nil, e.session())
		requireReason(t, w, http.StatusForbidden, "auth_two_factor_unavailable_oidc")
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := doRequest(e.router, rawRequest("POST", "/users/2fa/setup", "{bad", e.session()))
		requireReason(t, w, http.StatusBadRequest, "validation_invalid_json")
		w, _ = doRequest(e.router, rawRequest("POST", "/webauthn/register/begin", "{bad", e.session()))
		requireReason(t, w, http.StatusBadRequest, "validation_invalid_json")
		w, _ = doRequest(e.router, rawRequest("PATCH", "/webauthn/credentials/x", "{bad", e.session()))
		requireReason(t, w, http.StatusBadRequest, "validation_invalid_json")
	})

	t.Run("confirm before setup and with a wrong code", func(t *testing.T) {
		e := newWAEnv(t)
		tok := e.session()
		w, _ := e.do("POST", "/users/2fa/confirm", map[string]string{"code": "000000"}, tok)
		requireReason(t, w, http.StatusConflict, "auth_two_factor_setup_required")
		w, _ = e.do("POST", "/users/2fa/setup", nil, tok)
		require.Equal(t, http.StatusOK, w.Code)
		w, _ = e.do("POST", "/users/2fa/confirm", map[string]string{"code": "000000"}, tok)
		requireReason(t, w, http.StatusBadRequest, "auth_two_factor_code_invalid")
	})

	t.Run("not enabled", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := e.do("POST", "/users/2fa/disable", map[string]string{"code": "000000"}, e.session())
		requireReason(t, w, http.StatusConflict, "auth_two_factor_not_enabled")
		w, _ = e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": "000000"}, e.session())
		requireReason(t, w, http.StatusConflict, "auth_two_factor_not_enabled")
	})

	t.Run("login step: no pending challenge, bad session, wrong code, factor removed", func(t *testing.T) {
		e := newWAEnv(t)
		_, _ = e.enableTOTP(e.session())

		w, _ := doRequest(e.router, mustPost("/login/2fa", map[string]string{"code": "123456"}))
		requireReason(t, w, http.StatusUnauthorized, "auth_two_factor_login_not_pending")

		bad := &http.Cookie{Name: "2fa_pending", Value: "garbage"}
		w, _ = doRequest(e.router, withPending(mustPost("/login/2fa", map[string]string{"code": "123456"}), bad))
		requireReason(t, w, http.StatusUnauthorized, "auth_two_factor_session_invalid")

		pending, _ := e.passwordStep()
		w, _ = doRequest(e.router, withPending(mustPost("/login/2fa", map[string]string{"code": "000000"}), pending))
		requireReason(t, w, http.StatusBadRequest, "auth_two_factor_code_invalid")

		require.NoError(t, e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("totp_enabled", false).Error)
		w, _ = doRequest(e.router, withPending(mustPost("/login/2fa", map[string]string{"code": "123456"}), pending))
		requireReason(t, w, http.StatusUnauthorized, "auth_two_factor_no_longer_enabled")
	})
}

func TestSignInReasonCodes_Passkeys(t *testing.T) {
	t.Run("name too long and registration not started", func(t *testing.T) {
		e := newWAEnv(t)
		tok := e.session()
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"name": strings.Repeat("n", 101)}, tok)
		requireReason(t, w, http.StatusBadRequest, "auth_passkey_name_too_long")

		w, _ = e.do("POST", "/webauthn/register/finish", map[string]string{}, tok)
		requireReason(t, w, http.StatusConflict, "auth_passkey_registration_not_started")
	})

	t.Run("registration could not be verified", func(t *testing.T) {
		e := newWAEnv(t)
		tok := e.session()
		w, _ := e.do("POST", "/webauthn/register/begin", nil, tok)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w, _ = doRequest(e.router, rawRequest("POST", "/webauthn/register/finish", `{"id":"x"}`, tok))
		requireReason(t, w, http.StatusBadRequest, "auth_passkey_registration_failed")
	})

	t.Run("totp-only account has no passkey at login", func(t *testing.T) {
		e := newWAEnv(t)
		_, _ = e.enableTOTP(e.session())

		w, _ := doRequest(e.router, mustPost("/webauthn/login/begin", map[string]string{}))
		requireReason(t, w, http.StatusBadRequest, "auth_two_factor_login_not_pending")
		bad := &http.Cookie{Name: "2fa_pending", Value: "garbage"}
		w, _ = doRequest(e.router, withPending(mustPost("/webauthn/login/begin", map[string]string{}), bad))
		requireReason(t, w, http.StatusUnauthorized, "auth_two_factor_session_invalid")

		pending, _ := e.passwordStep()
		w, _ = doRequest(e.router, withPending(mustPost("/webauthn/login/begin", map[string]string{}), pending))
		requireReason(t, w, http.StatusConflict, "auth_passkey_none_registered")
		w, _ = doRequest(e.router, withPending(mustPost("/webauthn/login/finish", map[string]string{}), pending))
		requireReason(t, w, http.StatusUnauthorized, "auth_passkey_none_registered_sign_in_again")
	})

	t.Run("passkey login not started and not verified", func(t *testing.T) {
		e := newWAEnv(t)
		e.enroll(e.auth(), "Key", e.session())
		pending, _ := e.passwordStep()
		w, _ := doRequest(e.router, withPending(mustPost("/webauthn/login/finish", map[string]string{}), pending))
		requireReason(t, w, http.StatusConflict, "auth_passkey_login_not_started")

		w, _ = e.passkeyLogin(e.auth(), pending) // a different, unenrolled authenticator
		requireReason(t, w, http.StatusUnauthorized, "auth_passkey_verification_failed")
	})

	t.Run("rename validation", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := e.do("PATCH", "/webauthn/credentials/x", map[string]string{"name": "  "}, e.session())
		requireReason(t, w, http.StatusBadRequest, "auth_passkey_name_length_invalid")
	})

	t.Run("proof begin", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := e.do("POST", "/webauthn/assert/begin", nil, e.session())
		requireReason(t, w, http.StatusConflict, "auth_passkey_none_registered")

		id, _, tok := e.enroll(e.auth(), "Only", e.session())
		w, _ = doRequest(e.router, rawRequest("POST", "/webauthn/assert/begin", "{bad", tok))
		requireReason(t, w, http.StatusBadRequest, "validation_invalid_request_body")
		w, _ = e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": id}, tok)
		requireReason(t, w, http.StatusConflict, "auth_passkey_no_other_registered")
	})
}
