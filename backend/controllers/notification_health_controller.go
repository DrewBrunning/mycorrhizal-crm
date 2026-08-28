package controllers

import (
	"net/http"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetNotificationChannelHealth returns per-channel notification delivery
// health (issue #422) — configured-or-not, reachability, last success /
// failure, consecutive failures, failure reason, and attempted / delivered
// counts — for every delivery channel (email, ntfy, gotify, push).
//
// Instance-wide operational diagnostics, admin-only: like
// /admin/subsystem-health it is not user-scoped and is registered under the
// /admin group. The state is derived on read from notification_deliveries and
// the per-user channel config, so there is no write path and nothing to keep
// fresh.
func GetNotificationChannelHealth(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)

	health, err := services.ComputeNotificationChannelHealth(c.Request.Context(), db, cfg)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute notification channel health").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"channels": health})
}
