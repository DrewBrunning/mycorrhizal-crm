package controllers

import (
	"net/http"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSubsystemHealth returns the per-subsystem last-known-good state (issue
// #427) — current status, last attempt / success / failure, the first failure
// of the current incident, and the consecutive-failure count — for every
// tracked subsystem.
//
// Instance-wide operational diagnostics, admin-only: like /admin/system-events
// it is not user-scoped and is registered under the /admin group. The state is
// derived on read from system_events, so there is no write path and nothing to
// keep fresh.
func GetSubsystemHealth(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	health, err := services.ComputeSubsystemHealth(c.Request.Context(), db)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute subsystem health").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"subsystems": health})
}
