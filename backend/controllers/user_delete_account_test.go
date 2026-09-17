package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// deleteAccountTestEnv mirrors twoFactorTestEnv (real migrated schema via
// dbtest.New(t) — CLAUDE.md backend trap #1) with the routes DeleteOwnAccount
// tests need: the 2FA enrollment routes (so enableTwoFactor can be reused for
// the TOTP-required cases) and the self-service delete-account route itself.
// Sessions are minted directly with services.IssueSession rather than via
// POST /login, since these tests don't need the login flow itself.
func deleteAccountTestEnv(t *testing.T) (*gorm.DB, *gin.Engine, *config.Config) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() {
		models.AuditFlush()
		models.RegisterAuditDB(nil)
	})
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware(cfg))
	{
		protected.POST("/users/2fa/setup", SetupTwoFactor)
		protected.POST("/users/2fa/confirm", ConfirmTwoFactor)
		protected.DELETE("/account", func(c *gin.Context) {
			DeleteOwnAccount(c, cfg)
		})
	}

	return db, router, cfg
}

// seedDeleteAccountUser creates a user with a real bcrypt password hash
// (DeleteOwnAccount re-proves identity via bcrypt.CompareHashAndPassword, so
// a plaintext-password fixture would never authenticate).
func seedDeleteAccountUser(t *testing.T, db *gorm.DB, username string, isAdmin bool) models.User {
	t.Helper()
	hashed, err := services.HashPassword(strongPassword)
	require.NoError(t, err)
	user := models.User{
		Username: username,
		Email:    username + "@example.com",
		Password: hashed,
		IsAdmin:  isAdmin,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func deleteAccountRequest(token string, body any) *http.Request {
	return sessionRequest("DELETE", "/account", body, token)
}

// selfDeleteRouter is deleteAccountTestEnv's bare-router counterpart — no
// AuthMiddleware, matching admin_user_error_paths_test.go's adminRouter
// pattern. It exists so a specific table can be dropped to fault-inject one
// of DeleteOwnAccount's own DB calls without AuthMiddleware's unrelated
// session lookup (which also touches the users table) getting in the way
// first.
func selfDeleteRouter(db *gorm.DB, cfg *config.Config, userID uint, withUserID bool) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		if withUserID {
			c.Set("userID", userID)
		}
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.DELETE("/account", func(c *gin.Context) {
		DeleteOwnAccount(c, cfg)
	})
	return router
}

// TestDeleteOwnAccount_Succeeds is the happy path: a non-admin user (so no
// promotion guard applies) deletes their own account, the cascade runs, and
// the response clears the session cookie.
func TestDeleteOwnAccount_Succeeds(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "deleteme", false)
	contact := models.Contact{UserID: user.ID, Firstname: "Solo", Lastname: "Delete"}
	require.NoError(t, db.Create(&contact).Error)

	token, err := services.IssueSession(db, user, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, cookies := doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var remaining models.User
	err = db.Unscoped().First(&remaining, user.ID).Error
	assert.Error(t, err, "self-deleted user must be hard-deleted")

	var contactCount int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contactCount).Error)
	assert.Zero(t, contactCount, "the deleted user's contacts must be cleaned up by the cascade")

	authCookie, ok := cookies["auth_token"]
	require.True(t, ok, "response must clear the auth_token cookie")
	assert.Equal(t, "", authCookie.Value)
}

// TestDeleteOwnAccount_WrongPasswordRejected proves nothing is deleted when
// the re-proof fails.
func TestDeleteOwnAccount_WrongPasswordRejected(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "wrongpw", false)

	token, err := services.IssueSession(db, user, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{"current_password": "not-the-password"})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var stillThere models.User
	assert.NoError(t, db.First(&stillThere, user.ID).Error, "account must survive a failed re-proof")
}

// TestDeleteOwnAccount_MissingPasswordRejected proves the handler requires
// current_password even to be present.
func TestDeleteOwnAccount_MissingPasswordRejected(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "nopw", false)

	token, err := services.IssueSession(db, user, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var stillThere models.User
	assert.NoError(t, db.First(&stillThere, user.ID).Error)
}

// TestDeleteOwnAccount_TOTPRequiredWhenEnabled walks a real TOTP enrollment
// (enableTwoFactor, shared with two_factor_controller_test.go) and proves:
// missing code is rejected, wrong code is rejected, and a correct code
// succeeds — nothing deleted until the correct proof is given.
func TestDeleteOwnAccount_TOTPRequiredWhenEnabled(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "totpdelete", false)
	secret, _, sessionToken := enableTwoFactor(t, db, router, cfg, user)

	// Missing code.
	req := deleteAccountRequest(sessionToken, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Wrong code.
	req = deleteAccountRequest(sessionToken, map[string]string{
		"current_password": strongPassword,
		"totp_code":        "000000",
	})
	w, _ = doRequest(router, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var stillThere models.User
	require.NoError(t, db.First(&stillThere, user.ID).Error, "account must survive both failed 2FA proofs")

	// Correct code succeeds.
	req = deleteAccountRequest(sessionToken, map[string]string{
		"current_password": strongPassword,
		"totp_code":        totpCode(t, secret),
	})
	w, _ = doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	err := db.Unscoped().First(&models.User{}, user.ID).Error
	assert.Error(t, err, "account must be gone after a correct 2FA proof")
}

// TestDeleteOwnAccount_SoleAdminOtherUsersRequirePromotion covers issue #972
// decision 1: the sole admin cannot self-delete while other user rows exist
// without naming a promotion target, since RegisterUser only re-grants admin
// when the user table is completely empty — leaving those rows stranded with
// no admin forever otherwise.
func TestDeleteOwnAccount_SoleAdminOtherUsersRequirePromotion(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	admin := seedDeleteAccountUser(t, db, "soleadmin", true)
	other := seedDeleteAccountUser(t, db, "regularuser", false)

	token, err := services.IssueSession(db, admin, cfg, "", "")
	require.NoError(t, err)

	// No promote_user_id: blocked, candidate list returned.
	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var errResp struct {
		Error struct {
			Details struct {
				Candidates []struct {
					ID       uint   `json:"id"`
					Username string `json:"username"`
				} `json:"candidates"`
			} `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	require.Len(t, errResp.Error.Details.Candidates, 1)
	assert.Equal(t, other.ID, errResp.Error.Details.Candidates[0].ID)

	var adminStillThere models.User
	require.NoError(t, db.First(&adminStillThere, admin.ID).Error, "blocked self-delete must not touch the account")

	// An invalid promote_user_id (not an eligible candidate) is rejected the
	// same way.
	req = deleteAccountRequest(token, map[string]any{
		"current_password": strongPassword,
		"promote_user_id":  999999,
	})
	w, _ = doRequest(router, req)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	// A valid promote_user_id promotes that user, then deletes the caller.
	req = deleteAccountRequest(token, map[string]any{
		"current_password": strongPassword,
		"promote_user_id":  other.ID,
	})
	w, _ = doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var promoted models.User
	require.NoError(t, db.First(&promoted, other.ID).Error)
	assert.True(t, promoted.IsAdmin, "the named candidate must be promoted to admin")

	var deletedAdmin models.User
	err = db.Unscoped().First(&deletedAdmin, admin.ID).Error
	assert.Error(t, err, "the sole admin's own account must be deleted once promotion is satisfied")
}

// TestDeleteOwnAccount_SoleAdminNoOtherUsersDeletesDirectly is the genuinely
// single-user instance: no promotion is possible or needed, since there is
// nobody left to strand.
func TestDeleteOwnAccount_SoleAdminNoOtherUsersDeletesDirectly(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	admin := seedDeleteAccountUser(t, db, "onlyuser", true)

	token, err := services.IssueSession(db, admin, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	err = db.Unscoped().First(&models.User{}, admin.ID).Error
	assert.Error(t, err)
}

// TestDeleteOwnAccount_NotSoleAdminDeletesDirectly: when a peer admin exists,
// no promotion step applies either.
func TestDeleteOwnAccount_NotSoleAdminDeletesDirectly(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	admin1 := seedDeleteAccountUser(t, db, "admin1", true)
	_ = seedDeleteAccountUser(t, db, "admin2", true)

	token, err := services.IssueSession(db, admin1, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	err = db.Unscoped().First(&models.User{}, admin1.ID).Error
	assert.Error(t, err)
}

// TestDeleteOwnAccount_NoAuditEventPersisted pins issue #972 decision 3:
// audit_events.user_id is NOT NULL with an ON DELETE CASCADE FK to users.id,
// and actor == target for self-deletion, so no row can durably record this
// operation. The handler deliberately skips RecordAuditEvent rather than
// hitting that FK — this asserts the total row count is unchanged, not just
// that the deleted user's own rows are gone (which the FK cascade would
// explain on its own).
func TestDeleteOwnAccount_NoAuditEventPersisted(t *testing.T) {
	db, router, cfg := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "noaudit", false)

	var before int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Count(&before).Error)

	token, err := services.IssueSession(db, user, cfg, "", "")
	require.NoError(t, err)

	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	models.AuditFlush()

	var after int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Count(&after).Error)
	assert.Equal(t, before, after, "self-service account deletion must not add an audit_events row")
}

// TestDeleteOwnAccount_DemoModeDisabled mirrors ChangePassword's demo-mode
// gate — deleting the demo account would break the shared demo instance for
// everyone else.
func TestDeleteOwnAccount_DemoModeDisabled(t *testing.T) {
	db, _, _ := deleteAccountTestEnv(t)
	user := seedDeleteAccountUser(t, db, "demouser", false)

	demoCfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24, DemoMode: true}
	token, err := services.IssueSession(db, user, demoCfg, "", "")
	require.NoError(t, err)

	// Rebuild the route with the demo-mode config for this one request.
	demoRouter := gin.New()
	demoRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *demoCfg)
		c.Next()
	})
	demoProtected := demoRouter.Group("/")
	demoProtected.Use(middleware.AuthMiddleware(demoCfg))
	demoProtected.DELETE("/account", func(c *gin.Context) {
		DeleteOwnAccount(c, demoCfg)
	})

	req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
	w, _ := doRequest(demoRouter, req)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var stillThere models.User
	assert.NoError(t, db.First(&stillThere, user.ID).Error)
}

// TestSoleAdminPromotionCandidates covers soleAdminPromotionCandidates
// directly for the two "no promotion needed" branches, complementing the
// end-to-end HTTP cases above.
func TestSoleAdminPromotionCandidates(t *testing.T) {
	db := dbtest.New(t)

	t.Run("not an admin", func(t *testing.T) {
		u := seedDeleteAccountUser(t, db, "candnonadmin", false)
		candidates, err := soleAdminPromotionCandidates(db, u.ID)
		require.NoError(t, err)
		assert.Empty(t, candidates)
	})

	t.Run("sole admin, no other users", func(t *testing.T) {
		db2 := dbtest.New(t)
		u := seedDeleteAccountUser(t, db2, "candsolo", true)
		candidates, err := soleAdminPromotionCandidates(db2, u.ID)
		require.NoError(t, err)
		assert.Empty(t, candidates)
	})

	t.Run("sole admin, other users exist", func(t *testing.T) {
		db3 := dbtest.New(t)
		u := seedDeleteAccountUser(t, db3, "candneeded", true)
		other := seedDeleteAccountUser(t, db3, "candother", false)
		candidates, err := soleAdminPromotionCandidates(db3, u.ID)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, other.ID, candidates[0].ID)
	})

	t.Run("user lookup DB error", func(t *testing.T) {
		db4 := dbtest.New(t)
		u := seedDeleteAccountUser(t, db4, "canderr", false)
		require.NoError(t, db4.Exec("DROP TABLE users").Error)
		_, err := soleAdminPromotionCandidates(db4, u.ID)
		assert.Error(t, err)
	})
}

// TestDeleteOwnAccount_Unauthenticated covers currentUserID's own failure
// path (no "userID" in context) — unreachable through the real
// AuthMiddleware-protected router (it would already 401 before the handler
// ran), so this uses the bare selfDeleteRouter directly, mirroring
// TestDeleteUser_Unauthenticated's adminRouter(db, false) pattern.
func TestDeleteOwnAccount_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	router := selfDeleteRouter(db, cfg, 0, false)

	req := deleteAccountRequest("", map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// TestDeleteOwnAccount_UserLookupError mirrors TestDeleteUser_UserLookupError
// for the self-service path: a DB failure loading the caller's own user row
// must 500 before any re-proof or deletion.
func TestDeleteOwnAccount_UserLookupError(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	user := seedDeleteAccountUser(t, db, "lookuperr", false)

	require.NoError(t, db.Exec("DROP TABLE users").Error)

	router := selfDeleteRouter(db, cfg, user.ID, true)
	req := deleteAccountRequest("", map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// TestDeleteOwnAccount_AttachmentPluckError mirrors
// TestDeleteUser_ReportsPluckFailure: a DB failure capturing the caller's
// attachment names must abort before deleting anything, and the account
// must survive.
func TestDeleteOwnAccount_AttachmentPluckError(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	user := seedDeleteAccountUser(t, db, "pluckerr", false)

	require.NoError(t, db.Exec("DROP TABLE attachments").Error)

	router := selfDeleteRouter(db, cfg, user.ID, true)
	req := deleteAccountRequest("", map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var stillThere models.User
	assert.NoError(t, db.First(&stillThere, user.ID).Error, "a Pluck failure must abort before deleting the user")
}

// TestDeleteOwnAccount_CascadeTransactionFails is the self-service
// counterpart to TestDeleteUser_ReportsTransactionFailure: a mid-cascade
// delete failure must roll back the whole transaction (the outer
// db.Transaction wrapping both the optional promotion and deleteUserCascade)
// and 500 — the account and its data survive.
func TestDeleteOwnAccount_CascadeTransactionFails(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	user := seedDeleteAccountUser(t, db, "txfail", false)
	contact := models.Contact{UserID: user.ID, Firstname: "C"}
	require.NoError(t, db.Create(&contact).Error)
	activity := models.Activity{UserID: user.ID, Title: "call", Type: "call", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Model(&activity).Association("Contacts").Append(&contact))

	// Dropping activity_contacts makes the cascade fail partway through.
	require.NoError(t, db.Exec("DROP TABLE activity_contacts").Error)

	router := selfDeleteRouter(db, cfg, user.ID, true)
	req := deleteAccountRequest("", map[string]string{"current_password": strongPassword})
	w, _ := doRequest(router, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var userCount, activityCount int64
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).Count(&userCount).Error)
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&activityCount).Error)
	assert.EqualValues(t, 1, userCount, "the user must survive a rolled-back self-delete cascade")
	assert.EqualValues(t, 1, activityCount, "activities must survive the rollback")
}
