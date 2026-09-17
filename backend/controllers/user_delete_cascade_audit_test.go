package controllers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// TestDeleteUserCascade_DoesNotFireSpuriousAuditHooks pins the fix for the
// pre-existing audit-trail hole deleteUserCascade shares with DeleteUser and
// DeleteOwnAccount: several per-entity CRUD audit hooks (Circle, Tag,
// Household, Reminder) have no soft-delete guard, so a bulk
// tx.Where("user_id = ?", id).Delete(&Model{}) call fires them once
// regardless of how many rows actually matched, with a zero-value struct
// (empty entity ID, user_id 0). That queued write can never satisfy
// audit_events.user_id's NOT NULL FK to users(id). Before deleteUserCascade's
// SkipHooks session, deleting an account with even zero owned Circles/Tags/
// Households/Reminders reliably logged four "constraint failed: FOREIGN KEY
// constraint failed" warnings from models/audit.go on every single call.
//
// models.RegisterAuditDB is deliberately called *after* seeding each target
// user's owned entities, not before: registering it earlier would also queue
// async audit_events for those setup-time Creates, whose own completion race
// against the test's own timing (not deleteUserCascade's) and would make
// this test flaky for a reason unrelated to what it pins. This test asserts
// only that deleteUserCascade itself, mid-transaction, fires nothing.
//
// Hand-verified: reverting the `tx = tx.Session(&gorm.Session{SkipHooks:
// true})` line in user_delete_cascade.go makes both subtests below fail on
// the captured-log assertion, with the exact warning text this test greps
// for.
func TestDeleteUserCascade_DoesNotFireSpuriousAuditHooks(t *testing.T) {
	t.Run("DeleteUser", func(t *testing.T) {
		gin.SetMode(gin.ReleaseMode)
		db := dbtest.New(t)

		admin := seedCascadeUser(t, db, "audit-hook-admin")
		target := seedCascadeUser(t, db, "audit-hook-target")
		seedAuditHookOwnedEntities(t, db, target.ID)

		models.RegisterAuditDB(db)
		t.Cleanup(func() {
			models.AuditFlush()
			models.RegisterAuditDB(nil)
		})
		buf := captureTestLogger(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Set("userID", admin.ID)
			c.Next()
		})
		router.DELETE("/users/:id", DeleteUser)
		req, _ := http.NewRequest("DELETE", "/users/"+strconv.FormatUint(uint64(target.ID), 10), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "DeleteUser: %s", w.Body.String())

		models.AuditFlush()
		assertNoAuditFKWarning(t, buf)
	})

	t.Run("DeleteOwnAccount", func(t *testing.T) {
		gin.SetMode(gin.ReleaseMode)
		db := dbtest.New(t)
		cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}

		user := seedDeleteAccountUser(t, db, "audit-hook-self", false)
		seedAuditHookOwnedEntities(t, db, user.ID)
		token, err := services.IssueSession(db, user, cfg, "", "")
		require.NoError(t, err)

		models.RegisterAuditDB(db)
		t.Cleanup(func() {
			models.AuditFlush()
			models.RegisterAuditDB(nil)
		})
		buf := captureTestLogger(t)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Set("cfg", *cfg)
			c.Next()
		})
		protected := router.Group("/")
		protected.Use(middleware.AuthMiddleware(cfg))
		protected.DELETE("/account", func(c *gin.Context) {
			DeleteOwnAccount(c, cfg)
		})

		req := deleteAccountRequest(token, map[string]string{"current_password": strongPassword})
		w, _ := doRequest(router, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		models.AuditFlush()
		assertNoAuditFKWarning(t, buf)
	})
}

// seedAuditHookOwnedEntities seeds one real row in each table whose model
// has an AfterDelete hook with no soft-delete guard (Circle, Tag, Household,
// Reminder -- see models/audit_hooks.go), the entities whose bulk cascade
// delete previously fired a spurious audit write on every account deletion.
func seedAuditHookOwnedEntities(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Audit", Lastname: "Hook"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Circle{UserID: userID, Name: "c"}).Error)
	require.NoError(t, db.Create(&models.Tag{UserID: userID, Name: "t"}).Error)
	require.NoError(t, db.Create(&models.Household{UserID: userID, Name: "h", Type: models.HouseholdTypeOther}).Error)
	require.NoError(t, db.Create(&models.Reminder{UserID: userID, ContactID: &contact.ID, Message: "m", RemindAt: time.Now().Add(24 * time.Hour), Recurrence: "once"}).Error)
}

// assertNoAuditFKWarning fails the test if the captured server log contains
// the FK-constraint warning models/audit.go's fire-and-forget recorder logs
// when a queued audit_events insert fails.
func assertNoAuditFKWarning(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	logged := buf.String()
	assert.NotContains(t, logged, "FOREIGN KEY constraint failed",
		"deleteUserCascade must not fire per-entity audit hooks that can never satisfy audit_events' FK to the user row being deleted")
	assert.NotContains(t, logged, "audit: failed to persist audit event",
		"deleteUserCascade must not attempt a doomed audit write during the user-delete cascade")
}
