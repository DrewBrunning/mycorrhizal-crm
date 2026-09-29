package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// timelinePreviewLimit bounds each of the six timeline-eligible collection
// blocks of the M4 composite (buildContactDetail) at the default 5-item
// preview. 5 per type is provably sufficient to build a correct 5-item
// preview regardless of type distribution (T66 design decision 4): any item
// in the global top 5 must be within its own type's top 5, provided each
// type's block is ranked by the same key the preview merges on (event date).
const timelinePreviewLimit = 5

// ---------------------------------------------------------------------------
// Contact timeline (T66).
//
// GET /contacts/:id/timeline is the paginated, filterable view of a
// contact's merged timeline across all six event types. The composer itself
// (and its cursor type, predicates and merge) lives in services/timeline.go —
// moved there by ADR 0030 decision 3 so the private Atom feed shares the exact
// same query path. This file owns only the HTTP surface: parsing and
// validating the query controls, resolving the owned contact, and serializing
// the page.
//
// The merge strategy is described at length in services/timeline.go.
// ---------------------------------------------------------------------------

// timelineCursor is the composer's cursor type, re-exported under the
// controller's historical name so the query parser reads naturally.
type timelineCursor = services.TimelineCursor

func encodeTimelineCursor(date time.Time, typ, id string) string {
	return services.EncodeTimelineCursor(date, typ, id)
}

func decodeTimelineCursor(raw string) (*timelineCursor, error) {
	return services.DecodeTimelineCursor(raw)
}

// parseTimelineParams extracts and validates the timeline endpoint's query
// controls: limit (shared default/bounds), order (asc|desc), the opaque
// cursor, the comma-separated type filter, and the recency bucket. A
// malformed cursor, unknown type token, or unknown bucket is an error (400)
// — never a silent fallback.
func parseTimelineParams(c *gin.Context) (limit int, order string, cur *timelineCursor, types []string, bucket string, appErr *apperrors.AppError) {
	limit = parsePositiveOrDefault(c.DefaultQuery("limit", "25"), defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}
	order = c.DefaultQuery("order", "desc")
	if order != "asc" {
		order = "desc"
	}
	if raw := c.Query("cursor"); raw != "" {
		parsed, err := decodeTimelineCursor(raw)
		if err != nil {
			return 0, "", nil, nil, "", apperrors.ErrInvalidInput("cursor", err.Error())
		}
		cur = parsed
	}
	types, appErr = parseTimelineTypeFilter(c.Query("type"))
	if appErr != nil {
		return
	}
	bucket, appErr = parseTimelineBucket(c.Query("bucket"))
	return
}

// parseTimelineTypeFilter splits the ?type= query param (comma-separated
// tokens) into the set of timeline types to include. Empty means all six.
// Any unknown token is a 400 — an explicitly-set filter that is silently
// ignored is the worse failure.
func parseTimelineTypeFilter(raw string) ([]string, *apperrors.AppError) {
	if strings.TrimSpace(raw) == "" {
		return models.TimelineTypes, nil
	}
	var types []string
	seen := make(map[string]bool, len(models.TimelineTypes))
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if _, ok := models.TimelineTypeRank(tok); !ok {
			return nil, apperrors.ErrInvalidInput("type",
				fmt.Sprintf("unknown timeline type %q; expected one of: %s", tok, strings.Join(models.TimelineTypes, ", ")))
		}
		if !seen[tok] {
			seen[tok] = true
			types = append(types, tok)
		}
	}
	if len(types) == 0 {
		return models.TimelineTypes, nil
	}
	return types, nil
}

// parseTimelineBucket normalizes the ?bucket= recency filter, defaulting the
// empty value to "all". Anything outside the fixed set is a 400.
func parseTimelineBucket(raw string) (string, *apperrors.AppError) {
	switch raw {
	case "", models.TimelineBucketAll:
		return models.TimelineBucketAll, nil
	case models.TimelineBucketLast7Days, models.TimelineBucketLast30Days,
		models.TimelineBucketLast90Days, models.TimelineBucketThisYear:
		return raw, nil
	default:
		return "", apperrors.ErrInvalidInput("bucket",
			"must be one of: last_7_days, last_30_days, last_90_days, this_year, all")
	}
}

// timelineBucketCutoff returns the inclusive cutoff instant for a recency
// bucket computed against now, and ok=false when the bucket is "all". now
// carries the location the "this_year" boundary is computed in.
func timelineBucketCutoff(bucket string, now time.Time) (time.Time, bool) {
	switch bucket {
	case models.TimelineBucketLast7Days:
		return now.AddDate(0, 0, -7), true
	case models.TimelineBucketLast30Days:
		return now.AddDate(0, 0, -30), true
	case models.TimelineBucketLast90Days:
		return now.AddDate(0, 0, -90), true
	case models.TimelineBucketThisYear:
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), true
	default:
		return time.Time{}, false
	}
}

// GetContactTimeline returns a page of a contact's merged timeline across
// the six event types, filtered by ?type= (comma-separated) and ?bucket=
// (recency), paginated by an opaque (event_date, type, id) cursor. Read-only;
// every sub-query is scoped by user_id (CLAUDE.md trap 5).
func GetContactTimeline(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}

	limit, order, cur, types, bucket, appErr := parseTimelineParams(c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	now := reminderNow(c)
	cutoff, hasCutoff := timelineBucketCutoff(bucket, now)
	var cutoffPtr *time.Time
	if hasCutoff {
		cutoffPtr = &cutoff
	}

	contactID := contact.ID
	items, nextCursor, err := services.ComposeTimeline(db, userID, services.TimelineQuery{
		ContactID: &contactID,
		Limit:     limit,
		Desc:      order == "desc",
		Types:     types,
		Cutoff:    cutoffPtr,
		Cursor:    cur,
		Now:       now,
	})
	if err != nil {
		if errors.Is(err, services.ErrTimelineBadCursor) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("cursor", "cursor id is malformed"))
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact timeline").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": nextCursor,
		"limit":       limit,
	})
}
