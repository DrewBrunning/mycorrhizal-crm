package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	systemEventDefaultLimit = 100
	systemEventMaxLimit     = 500
)

// ListSystemEvents returns the operational-event timeline (issue #424),
// reverse-chronological by occurrence, filterable by component / severity /
// event_type / correlation_id / time range.
//
// Admin-only and instance-wide: system_events is not user-scoped — it records
// what happened to the system, not to one account — so it is registered under
// the /admin group (middleware.AdminMiddleware) rather than carrying a
// user_id filter here.
func ListSystemEvents(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	query := db.Model(&models.SystemEvent{})

	if v := c.Query("component"); v != "" {
		query = query.Where("component = ?", v)
	}
	if v := c.Query("severity"); v != "" {
		query = query.Where("severity = ?", v)
	}
	if v := c.Query("event_type"); v != "" {
		query = query.Where("event_type = ?", v)
	}
	if v := c.Query("correlation_id"); v != "" {
		query = query.Where("correlation_id = ?", v)
	}
	if v := c.Query("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("since", "must be an RFC3339 timestamp"))
			return
		}
		query = query.Where("occurred_at >= ?", t)
	}
	if v := c.Query("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("until", "must be an RFC3339 timestamp"))
			return
		}
		query = query.Where("occurred_at <= ?", t)
	}

	limit := systemEventDefaultLimit
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= systemEventMaxLimit {
			limit = n
		}
	}

	var events []models.SystemEvent
	if err := query.Order("occurred_at desc").Limit(limit).Find(&events).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to list system events").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"system_events": events, "total": len(events)})
}
