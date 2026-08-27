package controllers

import (
	"net/http"
	"strconv"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListJobRuns returns background-job run history (issue #391), newest-first,
// filterable by job_name / result / time range. Backs the per-job drill-down
// in the background-job monitor.
//
// Instance-wide operational diagnostics, admin-only: like /admin/system-events
// and /admin/subsystem-health it is not user-scoped and is registered under
// the /admin group.
func ListJobRuns(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	f := services.JobRunFilter{
		JobName: c.Query("job_name"),
		Result:  c.Query("result"),
	}
	if v := c.Query("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("since", "must be an RFC3339 timestamp"))
			return
		}
		f.Since = &t
	}
	if v := c.Query("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("until", "must be an RFC3339 timestamp"))
			return
		}
		f.Until = &t
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}

	runs, err := services.ListJobRuns(c.Request.Context(), db, f)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to list job runs").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"job_runs": runs, "total": len(runs)})
}

// GetJobRunHealth returns the folded per-job run health (issue #391) — current
// status, last run / success / failure, the consecutive-failure run and its
// first-failure time, and an avg/max duration trend — for every known
// background job. Derived on read from job_runs; instance-wide, admin-only.
func GetJobRunHealth(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	health, err := services.ComputeJobRunHealth(c.Request.Context(), db)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute job run health").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": health})
}
