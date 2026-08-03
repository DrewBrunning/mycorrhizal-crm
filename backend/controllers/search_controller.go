package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SearchAll implements GET /search (T11 / WP-86): full-text search across the
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
// rebuilds the FTS index from source, idempotently. Useful after a bulk
// import or a raw-SQL migration that bypassed the FTS triggers.
func RebuildSearchIndexHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	if err := services.RebuildSearchIndex(db); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to rebuild search index").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Search index rebuilt"})
}
