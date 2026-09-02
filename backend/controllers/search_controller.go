package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SearchAll implements GET /search (T11): full-text search across the
// caller's contacts, notes, and interactions, grouped into three sections.
// Query params: q=<term> (required), limit=<n> (per section, default 10,
// max 50). The response echoes a resolved_relation when the whole query is a
// relation synonym ("brother" → sibling_of) so a client can surface
// relationship-aware results.
func SearchAll(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	term := strings.TrimSpace(c.Query("q"))
	if term == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("q", "q (a search term) is required"))
		return
	}
	if len([]rune(term)) > services.MaxSearchTermLen {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("q", fmt.Sprintf("q must be at most %d characters", services.MaxSearchTermLen)))
		return
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("limit", "limit must be a positive integer"))
			return
		}
		limit = parsed
	}

	// Optional household scope ("everyone in the Smith household"): the
	// household (UUID id) must belong to the caller.
	var householdID *string
	if raw := strings.TrimSpace(c.Query("household_id")); raw != "" {
		var household models.Household
		if err := db.Where("id = ? AND user_id = ?", raw, userID).First(&household).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrNotFound("Household").WithDetails("id", raw))
			} else {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve household").WithError(err))
			}
			return
		}
		householdID = &raw
	}

	result, err := services.Search(db, userID, term, limit, householdID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Search failed").WithError(err))
		return
	}
	c.JSON(http.StatusOK, result)
}

// RebuildSearchIndexHandler implements POST /admin/search/rebuild (admin):
// rebuilds all three FTS indexes from canonical data, idempotently
// (SEARCH-01, issue #461). It is the operator-facing rebuild path for a
// deployment with no Go toolchain — a stock Docker install reaches it over
// HTTP, no `go run` needed.
//
// The rebuild runs synchronously (like the other /admin/trigger-* jobs) but
// records a job_runs row (trigger "manual", job_name "search_index_rebuild")
// so its duration and outcome land on the admin job-run timeline (issue
// #391), not only in this response. Search is not blocked while it runs;
// ordinary writes queue behind it (see RebuildSearchIndexReport). A rebuild
// already in progress in this process is reported as 409 rather than queued.
func RebuildSearchIndexHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	ctx := c.Request.Context()
	start := time.Now()

	stats, err := services.RebuildSearchIndexExclusive(db)
	if errors.Is(err, services.ErrJobSkipped) {
		// Nothing ran, so there is no job_runs row to record — and the
		// in-progress rebuild holds SQLite's write lock, so a diagnostic
		// insert here would only block on it. The 409 is the whole signal.
		apperrors.AbortWithError(c, apperrors.ErrConflict("A search index rebuild is already in progress"))
		return
	}
	if err != nil {
		recordManualJobRun(ctx, db, models.JobNameSearchIndexRebuild, start, nil, err)
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to rebuild search index").WithError(err))
		return
	}

	total := int(stats.Total())
	recordManualJobRun(ctx, db, models.JobNameSearchIndexRebuild, start, &total, nil)
	c.JSON(http.StatusOK, gin.H{
		"message": "Search index rebuilt",
		"indexed": gin.H{
			"contacts":   stats.Contacts,
			"notes":      stats.Notes,
			"activities": stats.Activities,
		},
	})
}
