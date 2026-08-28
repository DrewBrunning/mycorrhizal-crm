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

const (
	errAggDefaultWindowHours = 24
	errAggMaxWindowHours     = 168 // 7 days
)

// GetErrorAggregation returns operational failures over a rolling window
// bucketed by cause (issue #426) — one bucket per (component, normalized error)
// with its count, whether it recurs, when it was first/last seen, and the exact
// system_events row ids behind it (for the timeline's ?ids= drill-down).
//
// Instance-wide operational diagnostics, admin-only: like /admin/system-events
// and /admin/subsystem-health it is not user-scoped and is registered under the
// /admin group. Derived on read from system_events — no write path, nothing to
// keep fresh.
func GetErrorAggregation(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	windowHours := errAggDefaultWindowHours
	if raw := c.Query("window_hours"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > errAggMaxWindowHours {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("window_hours", "must be an integer between 1 and 168"))
			return
		}
		windowHours = n
	}

	until := time.Now().UTC()
	since := until.Add(-time.Duration(windowHours) * time.Hour)

	buckets, total, err := services.AggregateOperationalErrors(c.Request.Context(), db, since)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to aggregate operational errors").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"window_hours": windowHours,
		"since":        since,
		"until":        until,
		"total_events": total,
		"buckets":      buckets,
	})
}
