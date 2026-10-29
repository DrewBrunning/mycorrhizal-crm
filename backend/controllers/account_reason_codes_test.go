package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2102 (ADR 0038): account, admin, API-token and contact-share errors
// carry a reason_code in the raw JSON so clients can localize the message.
// These tests read the wire body (requireReason/wireReason in
// sign_in_reason_codes_test.go), not the Go value, so a dropped .WithReason
// fails them.

// tooLongPassword is longer than bcrypt's 72-byte limit.
var tooLongPassword = strings.Repeat("aB3!", 19)

func serveRaw(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, rawRequest(method, path, body, ""))
	return w
}

func TestAccountReasonCodes_Preferences(t *testing.T) {
	t.Run("language", func(t *testing.T) {
		db, router := setupRouter(t)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		router.PATCH("/language", func(c *gin.Context) { c.Set("username", me.Username); UpdateLanguage(c) })

		requireReason(t, serveRaw(router, "PATCH", "/language", `{"language":5}`),
			http.StatusBadRequest, "account_language_invalid")

		w := serveRaw(router, "PATCH", "/language", `{"language":"xx"}`)
		requireReason(t, w, http.StatusBadRequest, "account_language_unsupported")
		_, params := wireReason(t, w)
		assert.Equal(t, "en, de, it, es, fr", params["supported"])
	})

	t.Run("language without a signed-in user", func(t *testing.T) {
		_, router := setupRouter(t)
		router.PATCH("/language", UpdateLanguage)
		requireReason(t, serveRaw(router, "PATCH", "/language", `{"language":"en"}`),
			http.StatusUnauthorized, "auth_required")
	})

	t.Run("date format", func(t *testing.T) {
		db, router := setupRouter(t)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		router.PATCH("/date", func(c *gin.Context) { c.Set("username", me.Username); UpdateDateFormat(c) })

		requireReason(t, serveRaw(router, "PATCH", "/date", `{"date_format":5}`),
			http.StatusBadRequest, "account_date_format_invalid")
		requireReason(t, serveRaw(router, "PATCH", "/date", `{"date_format":"bogus"}`),
			http.StatusBadRequest, "account_date_format_unsupported")
	})

	t.Run("time format", func(t *testing.T) {
		db, router := setupRouter(t)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		router.PATCH("/time", func(c *gin.Context) { c.Set("username", me.Username); UpdateTimeFormat(c) })

		requireReason(t, serveRaw(router, "PATCH", "/time", `{"time_format":5}`),
			http.StatusBadRequest, "account_time_format_invalid")
		requireReason(t, serveRaw(router, "PATCH", "/time", `{"time_format":"bogus"}`),
			http.StatusBadRequest, "account_time_format_unsupported")
	})

	t.Run("dav password policy blocked by two-factor", func(t *testing.T) {
		db, router := setupRouter(t)
		require.NoError(t, db.Model(&models.User{}).Where("1 = 1").Updates(map[string]any{
			"totp_enabled":          true,
			"totp_secret_encrypted": "encrypted-secret",
		}).Error)
		router.PATCH("/dav", middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}), UpdateDAVPasswordPolicy)
		requireReason(t, serveRaw(router, "PATCH", "/dav", `{"allow":true}`),
			http.StatusConflict, "account_dav_password_requires_no_two_factor")
	})
}

func TestAccountReasonCodes_RegistrationDisabledEnvelope(t *testing.T) {
	_, router := newRegisterRouter(t, &config.Config{RegistrationDisabled: true})
	w := serveRaw(router, "POST", "/register", `{"username":"blocked","email":"blocked@example.com","password":"`+strongPassword+`"}`)
	requireReason(t, w, http.StatusForbidden, "auth_registration_disabled")
}

func TestAccountReasonCodes_ChangePassword(t *testing.T) {
	setup := func(t *testing.T, cfg *config.Config, in *models.ChangePasswordInput, username string) *gin.Engine {
		db, router := setupRouter(t)
		hash, err := services.HashPassword(strongPassword)
		require.NoError(t, err)
		require.NoError(t, db.Model(&models.User{}).Where("1 = 1").Update("password", hash).Error)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		if username == "me" {
			username = me.Username
		}
		router.POST("/change", func(c *gin.Context) {
			if username != "" {
				c.Set("username", username)
			}
			c.Set("validated", in)
			ChangePassword(c, cfg)
		})
		return router
	}

	t.Run("demo mode", func(t *testing.T) {
		router := setup(t, &config.Config{DemoMode: true}, &models.ChangePasswordInput{}, "me")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusForbidden, "auth_password_change_disabled_demo")
	})

	t.Run("not signed in", func(t *testing.T) {
		router := setup(t, &config.Config{}, &models.ChangePasswordInput{}, "")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusUnauthorized, "auth_required")
	})

	t.Run("wrong current password", func(t *testing.T) {
		router := setup(t, &config.Config{}, &models.ChangePasswordInput{CurrentPassword: "nope", NewPassword: strongPasswordAnother}, "me")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusBadRequest, "account_current_password_incorrect")
	})

	t.Run("new password equals current", func(t *testing.T) {
		router := setup(t, &config.Config{}, &models.ChangePasswordInput{CurrentPassword: strongPassword, NewPassword: strongPassword}, "me")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusBadRequest, "account_new_password_same_as_current")
	})

	t.Run("password too long", func(t *testing.T) {
		router := setup(t, &config.Config{}, &models.ChangePasswordInput{CurrentPassword: strongPassword, NewPassword: tooLongPassword}, "me")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusBadRequest, "auth_password_too_long")
	})

	t.Run("breached password", func(t *testing.T) {
		server := newHIBPServer(t, strongPasswordAnother)
		defer server.Close()
		t.Cleanup(services.SetHIBPAPIBaseURLForTest(server.URL))
		router := setup(t, &config.Config{HIBPCheckEnabled: true},
			&models.ChangePasswordInput{CurrentPassword: strongPassword, NewPassword: strongPasswordAnother}, "me")
		requireReason(t, serveRaw(router, "POST", "/change", `{}`), http.StatusBadRequest, "auth_password_breached")
	})
}

func TestAccountReasonCodes_DeleteOwnAccount(t *testing.T) {
	t.Run("demo mode", func(t *testing.T) {
		db, _, _ := deleteAccountTestEnv(t)
		user := seedDeleteAccountUser(t, db, "demodel", false)
		cfg := &config.Config{DemoMode: true}
		router := selfDeleteRouter(db, cfg, user.ID, true)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, deleteAccountRequest("", map[string]string{"current_password": strongPassword}))
		requireReason(t, w, http.StatusForbidden, "account_deletion_disabled_demo")
	})

	t.Run("wrong password", func(t *testing.T) {
		db, _, cfg := deleteAccountTestEnv(t)
		user := seedDeleteAccountUser(t, db, "wrongpw", false)
		router := selfDeleteRouter(db, cfg, user.ID, true)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, deleteAccountRequest("", map[string]string{"current_password": "nope"}))
		requireReason(t, w, http.StatusBadRequest, "account_current_password_incorrect")
	})

	t.Run("sole admin must name a successor", func(t *testing.T) {
		db, _, cfg := deleteAccountTestEnv(t)
		admin := seedDeleteAccountUser(t, db, "soleadmin2102", true)
		seedDeleteAccountUser(t, db, "other2102", false)
		router := selfDeleteRouter(db, cfg, admin.ID, true)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, deleteAccountRequest("", map[string]string{"current_password": strongPassword}))
		requireReason(t, w, http.StatusConflict, "account_sole_admin_promotion_required")
		// The candidates list the web dialog reads is unchanged.
		assert.Contains(t, w.Body.String(), `"candidates"`)
	})
}

func TestAdminReasonCodes(t *testing.T) {
	t.Run("invalid user id", func(t *testing.T) {
		_, router := setupRouter(t)
		router.GET("/users/:id", GetUser)
		router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)
		router.POST("/users/:id/reset-2fa", ResetUserTwoFactor)
		router.DELETE("/users/:id", DeleteUser)
		for _, tc := range []struct{ method, path string }{
			{"GET", "/users/abc"}, {"PATCH", "/users/abc"}, {"POST", "/users/abc/reset-2fa"}, {"DELETE", "/users/abc"},
		} {
			requireReason(t, serveRaw(router, tc.method, tc.path, `{}`), http.StatusBadRequest, "admin_user_id_invalid")
		}
	})

	t.Run("create with a too-long password", func(t *testing.T) {
		_, router := setupRouter(t)
		router.POST("/users", func(c *gin.Context) {
			c.Set("validated", &models.AdminUserCreateInput{
				Username: "newbie2102", Email: "newbie2102@example.com", Password: tooLongPassword,
			})
			CreateUser(c)
		})
		requireReason(t, serveRaw(router, "POST", "/users", `{}`), http.StatusBadRequest, "auth_password_too_long")
	})

	t.Run("update guard rails", func(t *testing.T) {
		db, router := setupRouter(t)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		require.NoError(t, db.Model(&me).Update("is_admin", true).Error)
		peer := models.User{Username: "peer2102", Email: "peer2102@example.com", Password: "x", IsAdmin: true}
		require.NoError(t, db.Create(&peer).Error)

		no := false
		long := tooLongPassword
		in := &models.AdminUserUpdateInput{}
		router.PATCH("/users/:id", func(c *gin.Context) { c.Set("validated", in); UpdateUser(c) })
		patch := func(id uint) *httptest.ResponseRecorder {
			return serveRaw(router, "PATCH", "/users/"+strconv.Itoa(int(id)), `{}`)
		}

		in.IsAdmin = &no
		requireReason(t, patch(peer.ID), http.StatusForbidden, "admin_role_change_other_admin_forbidden")

		require.NoError(t, db.Model(&peer).Update("is_admin", false).Error)
		requireReason(t, patch(me.ID), http.StatusForbidden, "admin_last_admin_removal_forbidden")

		in.IsAdmin = nil
		in.Password = &long
		requireReason(t, patch(peer.ID), http.StatusBadRequest, "auth_password_too_long")
	})

	t.Run("delete guard rails", func(t *testing.T) {
		db, router := setupRouter(t)
		var me models.User
		require.NoError(t, db.First(&me).Error)
		peer := models.User{Username: "peerdel2102", Email: "peerdel2102@example.com", Password: "x", IsAdmin: true}
		require.NoError(t, db.Create(&peer).Error)
		router.DELETE("/users/:id", DeleteUser)

		requireReason(t, serveRaw(router, "DELETE", "/users/"+strconv.Itoa(int(me.ID)), ""),
			http.StatusForbidden, "admin_delete_self_forbidden")
		requireReason(t, serveRaw(router, "DELETE", "/users/"+strconv.Itoa(int(peer.ID)), ""),
			http.StatusForbidden, "admin_delete_other_admin_forbidden")
	})
}

func TestAPITokenReasonCodes(t *testing.T) {
	router, _ := setupApiTokenRouter(t)

	// The validator rejects most bad scope input before the handler runs, so
	// the handler's own defense-in-depth arms are driven with a pre-validated
	// input.
	in := &models.ApiTokenInput{Name: "t"}
	router.POST("/validated-tokens", func(c *gin.Context) { c.Set("validated", in); CreateApiToken(c) })

	t.Run("scopes", func(t *testing.T) {
		in.Scopes = &[]string{}
		requireReason(t, serveRaw(router, "POST", "/validated-tokens", `{}`),
			http.StatusBadRequest, "account_api_token_scopes_required")

		// NormalizeTokenScopes drops unknown scopes, so the list form's own
		// "Unknown scope" arm is unreachable; the legacy single scope is not.
		in.Scopes = nil
		in.Scope = "bogus"
		w := serveRaw(router, "POST", "/validated-tokens", `{}`)
		requireReason(t, w, http.StatusBadRequest, "account_api_token_scope_unknown")
		_, params := wireReason(t, w)
		assert.Equal(t, "bogus", params["scope"])
	})

	t.Run("expiry options", func(t *testing.T) {
		requireReason(t, serveRaw(router, "POST", "/api-tokens", `{"name":"t","expires_in_days":5,"never_expires":true,"scope":"carddav"}`),
			http.StatusBadRequest, "account_api_token_expiry_conflict")
		requireReason(t, serveRaw(router, "POST", "/api-tokens", `{"name":"t","never_expires":true}`),
			http.StatusBadRequest, "account_api_token_never_expires_not_allowed")
	})

	t.Run("ids and rotation", func(t *testing.T) {
		requireReason(t, serveRaw(router, "DELETE", "/api-tokens/abc", ""),
			http.StatusBadRequest, "validation_id_not_positive_integer")
		requireReason(t, serveRaw(router, "POST", "/api-tokens/abc/rotate", ""),
			http.StatusBadRequest, "validation_id_not_positive_integer")

		created := serveRaw(router, "POST", "/api-tokens", `{"name":"to-revoke"}`)
		require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
		var tok struct {
			ID uint `json:"id"`
		}
		require.NoError(t, json.Unmarshal(created.Body.Bytes(), &tok))
		id := strconv.Itoa(int(tok.ID))
		require.Equal(t, http.StatusOK, serveRaw(router, "DELETE", "/api-tokens/"+id, "").Code)
		requireReason(t, serveRaw(router, "POST", "/api-tokens/"+id+"/rotate", ""),
			http.StatusConflict, "account_api_token_already_revoked")
	})
}

func TestContactShareReasonCodes(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	t.Run("share with yourself", func(t *testing.T) {
		router := contactShareRouterFor(db, cfg, sender.ID)
		w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
			ToUserID: sender.ID, VCardUID: "any", Sections: []string{"names"},
		})
		requireReason(t, w, http.StatusBadRequest, "contact_share_with_self")
	})

	t.Run("unknown section", func(t *testing.T) {
		contact := models.Contact{UserID: sender.ID, Firstname: "Sec", Lastname: "Tion"}
		require.NoError(t, db.Create(&contact).Error)
		router := contactShareRouterFor(db, cfg, sender.ID)
		w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
			ToUserID: recipient.ID, VCardUID: contact.VCardUID, Sections: []string{"not-a-section"},
		})
		requireReason(t, w, http.StatusBadRequest, "contact_share_section_unknown")
		_, params := wireReason(t, w)
		assert.Equal(t, "not-a-section", params["section"])
	})

	t.Run("already responded and wrong session", func(t *testing.T) {
		share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Resp", []string{models.SectionEmails}, false)
		router := contactShareRouterFor(db, cfg, recipient.ID)

		w := doJSON(t, router, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{SessionID: "someone-elses"})
		requireReason(t, w, http.StatusBadRequest, "contact_share_session_mismatch")

		w = doJSON(t, router, "POST", "/contact-shares/"+share.ID+"/decline", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = doJSON(t, router, "POST", "/contact-shares/"+share.ID+"/accept", nil)
		requireReason(t, w, http.StatusConflict, "contact_share_already_responded")
	})
}
