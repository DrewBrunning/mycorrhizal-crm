package controllers

// Coverage tests for milestone-30 issue #1623 §3: the low-coverage admin/job
// error paths in job_run_controller.go — the invalid `until`/`limit` filter
// branches, the database-failure branches, the empty-result ("no matching
// job") path, and the non-admin 403 gate.

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestJobRunCov_BadUntilRejected covers the invalid `until` filter branch
// (mirroring the existing `since` case).
func TestJobRunCov_BadUntilRejected(t *testing.T) {
	router, _ := newJobRunControllerRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/job-runs?until=not-a-time", nil)
	router.ServeHTTP(w, req)
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "until")
}

// TestJobRunCov_UntilAndLimitParsed covers the successful `until` parse and
// the `limit` parse (both the numeric and the silently-ignored non-numeric
// forms).
func TestJobRunCov_UntilAndLimitParsed(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	base := time.Now().Add(-time.Hour).UTC()
	seedControllerRun(t, db, models.JobNameDailyReminders, base, models.JobRunResultSuccess)
	seedControllerRun(t, db, models.JobNameDailyReminders, base.Add(time.Minute), models.JobRunResultFailure)

	until := base.Add(30 * time.Second).Format(time.RFC3339)
	var untilResp struct {
		JobRuns []models.JobRun `json:"job_runs"`
		Total   int             `json:"total"`
	}
	doGET(t, router, "/admin/job-runs?until="+until, &untilResp)
	assert.Equal(t, 1, untilResp.Total, "only the run before `until` must remain")

	var limitResp struct {
		JobRuns []models.JobRun `json:"job_runs"`
	}
	doGET(t, router, "/admin/job-runs?limit=1", &limitResp)
	assert.Len(t, limitResp.JobRuns, 1, "limit is honoured")

	// A non-numeric limit is ignored rather than rejected.
	doGET(t, router, "/admin/job-runs?limit=not-a-number", &limitResp)
}

// TestJobRunCov_UnknownJobNameEmpty covers the "no matching job" result: an
// unknown job_name filters to an empty list, still a 200.
func TestJobRunCov_UnknownJobNameEmpty(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	seedControllerRun(t, db, models.JobNameDailyReminders, time.Now().Add(-time.Hour).UTC(), models.JobRunResultSuccess)

	var resp struct {
		JobRuns []models.JobRun `json:"job_runs"`
		Total   int             `json:"total"`
	}
	doGET(t, router, "/admin/job-runs?job_name=does-not-exist", &resp)
	assert.Zero(t, resp.Total)
	assert.Empty(t, resp.JobRuns)
}

// TestJobRunCov_ListDBError exercises ListJobRuns' database-failure branch.
func TestJobRunCov_ListDBError(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	dbtest.HideTable(t, db, "job_runs")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/job-runs", nil)
	router.ServeHTTP(w, req)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestJobRunCov_HealthDBError exercises GetJobRunHealth's database-failure
// branch.
func TestJobRunCov_HealthDBError(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	dbtest.HideTable(t, db, "job_runs")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/job-runs/health", nil)
	router.ServeHTTP(w, req)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// newJobRunCovAdminRouter wires the real AdminMiddleware in front of the job
// handlers, so the non-admin 403 gate is exercised with the real route stack.
func newJobRunCovAdminRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Next()
	})
	router.Use(middleware.AdminMiddleware())
	router.GET("/admin/job-runs", ListJobRuns)
	router.GET("/admin/job-runs/health", GetJobRunHealth)
	return router
}

// TestJobRunCov_NonAdminForbidden proves a non-admin caller is refused with
// 403 and the handler never runs.
func TestJobRunCov_NonAdminForbidden(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)
	nonAdmin := models.User{Username: "jobrun-nonadmin", Password: "password123!A", Email: "jobrun-nonadmin@example.com", IsAdmin: false}
	require.NoError(t, db.Create(&nonAdmin).Error)

	router := newJobRunCovAdminRouter(t, db, nonAdmin.ID)
	for _, path := range []string{"/admin/job-runs", "/admin/job-runs/health"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", path, w.Body.String())
		assert.Contains(t, w.Body.String(), "Admin access required", path)
	}
}

// TestJobRunCov_AdminAllowed is the contrast case: an admin user passes the
// gate and reaches the handler.
func TestJobRunCov_AdminAllowed(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)
	admin := models.User{Username: "jobrun-admin", Password: "password123!A", Email: "jobrun-admin@example.com", IsAdmin: true}
	require.NoError(t, db.Create(&admin).Error)

	router := newJobRunCovAdminRouter(t, db, admin.ID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/job-runs", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
