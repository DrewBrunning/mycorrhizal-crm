package controllers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// heldAuditRecorder models an async audit event that is queued but whose
// persist has not run yet (issue #1693): Record only parks the event, and
// nothing reaches the real recorder until Flush -- exactly the window the
// user-deletion path must close. Deterministic: no sleeps, no goroutine race.
type heldAuditRecorder struct {
	models.AuditRecorder
	mu      sync.Mutex
	held    []func()
	flushed int
}

func (h *heldAuditRecorder) Record(tx *gorm.DB, entityType, entityID, operation string, userID uint, snapshot string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held = append(h.held, func() {
		h.AuditRecorder.Record(tx, entityType, entityID, operation, userID, snapshot)
	})
}

func (h *heldAuditRecorder) Flush() {
	h.mu.Lock()
	held := h.held
	h.held = nil
	h.flushed++
	h.mu.Unlock()
	for _, f := range held {
		f()
	}
	h.AuditRecorder.Flush()
}

func (h *heldAuditRecorder) pending() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.held)
}

func newHeldRecorder(t *testing.T, db *gorm.DB) *heldAuditRecorder {
	t.Helper()
	inner := models.NewAuditRecorder(db) // async, wrapped pool
	h := &heldAuditRecorder{AuditRecorder: inner}
	db.Plugins[inner.Name()] = h
	return h
}

func auditRowsFor(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

// TestUserDeletion_FlushesQueuedAuditEvents pins issue #1693: an async audit
// event queued for a user just before DeleteUser / DeleteOwnAccount must be
// persisted (then removed by the audit_events ON DELETE CASCADE) rather than
// racing the hard delete and failing the user_id FK with a warn log.
func TestUserDeletion_FlushesQueuedAuditEvents(t *testing.T) {
	adminDelete := func(db *gorm.DB, admin, target models.User) *httptest.ResponseRecorder {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Set("userID", admin.ID)
			c.Set("cfg", config.Config{})
		})
		router.DELETE("/users/:id", DeleteUser)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/users/"+strconv.Itoa(int(target.ID)), nil)
		router.ServeHTTP(w, req)
		return w
	}
	selfDelete := func(db *gorm.DB, _, target models.User) *httptest.ResponseRecorder {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Set("userID", target.ID)
			c.Set("cfg", config.Config{})
		})
		router.DELETE("/account", func(c *gin.Context) { DeleteOwnAccount(c, &config.Config{}) })
		w := httptest.NewRecorder()
		router.ServeHTTP(w, sessionRequest("DELETE", "/account", map[string]string{"current_password": strongPassword}, ""))
		return w
	}

	for name, run := range map[string]func(*gorm.DB, models.User, models.User) *httptest.ResponseRecorder{
		"admin_DeleteUser": adminDelete,
		"DeleteOwnAccount": selfDelete,
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.ReleaseMode)
			isolateAccountLimiter(t)
			guard := logtest.Guard(t)
			db := dbtest.New(t)
			admin := seedCascadeUser(t, db, "race-admin")
			require.NoError(t, db.Model(&admin).Update("is_admin", true).Error)
			target := seedCascadeUser(t, db, "race-target")

			held := newHeldRecorder(t, db)
			// The login event the E2E run queued just before the delete.
			held.Record(db, "auth", "e2e", "login", target.ID, "")
			require.Equal(t, 1, held.pending())

			w := run(db, admin, target)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			// The target's queued event was released by the pre-delete flush;
			// the only thing still parked is the admin's own user/delete
			// event, recorded after the cascade (DeleteOwnAccount records none).
			assert.Equal(t, 1, held.flushed, "deletion must flush before the cascade")
			wantPending := 0
			if name == "admin_DeleteUser" {
				wantPending = 1
			}
			assert.Equal(t, wantPending, held.pending())
			// Persisted, then cascade-removed with the user: no orphan, no
			// dropped event (Guard fails the test on the FK warn log).
			assert.Zero(t, auditRowsFor(t, db, target.ID))
			guard.AssertNoWarnings(t)
			f, ok := held.AuditRecorder.(interface{ FailedWrites() int64 })
			require.True(t, ok)
			assert.Zero(t, f.FailedWrites(), "no audit insert may fail")
		})
	}
}
