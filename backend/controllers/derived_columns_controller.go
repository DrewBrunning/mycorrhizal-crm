package controllers

import (
	"errors"
	"net/http"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RebuildDerivedColumnsHandler implements POST /admin/contacts/rebuild-derived
// (admin): re-derives every live contact's denormalized columns — the flat
// contacts.* projection, contacts.sort_name, contacts.addresses_flat,
// contacts.phones_normalized — from the authoritative nested Card,
// idempotently (issue #497). It is the flat-column analogue of POST
// /admin/search/rebuild and the operator-facing path for a deployment with no
// Go toolchain: a stock Docker install reaches it over HTTP, no `go run`
// needed. Run both after a restore.
//
// Like the other /admin/trigger-* jobs it runs synchronously and records a
// job_runs row (trigger "manual", job_name "derived_columns_rebuild") so its
// duration and outcome land on the admin job-run timeline. On a faithful
// database it rewrites nothing — the projection is a fixpoint of a re-save. A
// rebuild already in progress in this process is reported as 409 rather than
// queued.
func RebuildDerivedColumnsHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	ctx := c.Request.Context()
	start := time.Now()

	stats, err := services.RebuildDerivedContactColumnsExclusive(ctx, db)
	if errors.Is(err, services.ErrJobSkipped) {
		apperrors.AbortWithError(c, apperrors.ErrConflict("A derived-column rebuild is already in progress"))
		return
	}
	if err != nil {
		recordManualJobRun(ctx, db, models.JobNameDerivedColumnsRebuild, start, nil, err)
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to rebuild derived contact columns").WithError(err))
		return
	}

	updated := int(stats.ContactsUpdated)
	recordManualJobRun(ctx, db, models.JobNameDerivedColumnsRebuild, start, &updated, nil)
	c.JSON(http.StatusOK, gin.H{
		"message":          "Derived contact columns rebuilt",
		"contacts_scanned": stats.ContactsScanned,
		"contacts_updated": stats.ContactsUpdated,
		"column_updates":   stats.ColumnUpdates,
	})
}
