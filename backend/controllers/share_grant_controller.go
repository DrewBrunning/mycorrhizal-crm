package controllers

import (
	"net/http"
	"strconv"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Standing shares API (ADR 0042, issue #2290). Handlers are thin: every
// authorization decision lives in services/share_grant.go, which scopes each
// lookup by the caller's role column. Responses are Cache-Control: no-store
// with no ETag (NoStoreMiddleware), so a revoked read is never served from a
// cache. There is no include_sensitive anywhere on this surface.

// NoStoreMiddleware marks every response no-store and strips any ETag, so a
// browser, proxy or service worker cannot keep serving a revoked grant. It
// sets the header before the handler runs so error paths carry it too.
func NoStoreMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

func shareGrantNow(c *gin.Context) (now time.Time) { return clock.FromContext(c).Now() }

// CreateShareGrant — POST /share-grants (owner).
func CreateShareGrant(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, err := middleware.GetValidated[models.ShareGrantInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	now := shareGrantNow(c)
	g, appErr := services.CreateShareGrant(db, userID, *input, now)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"share_grant": services.OwnerView(db, g, now)})
}

// ListOutgoingShareGrants — GET /share-grants/outgoing (owner).
func ListOutgoingShareGrants(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	now := shareGrantNow(c)
	rows, next, limit, appErr := listShareGrantRows(c, db, func(q *gorm.DB) *gorm.DB { return q.Where("from_user_id = ?", userID) })
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	views := make([]services.ShareGrantOwnerView, 0, len(rows))
	for i := range rows {
		views = append(views, services.OwnerView(db, &rows[i], now))
	}
	c.JSON(http.StatusOK, gin.H{"share_grants": views, "next_cursor": next, "limit": limit})
}

// ListIncomingShareGrants — GET /share-grants/incoming (recipient): pending
// and active only; blocked and expired rows are invisible.
func ListIncomingShareGrants(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	now := shareGrantNow(c)
	rows, next, limit, appErr := listShareGrantRows(c, db, func(q *gorm.DB) *gorm.DB { return services.IncomingVisible(q, userID, now) })
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	views := make([]services.ShareGrantRecipientView, 0, len(rows))
	for i := range rows {
		views = append(views, services.RecipientView(db, &rows[i], now))
	}
	c.JSON(http.StatusOK, gin.H{"share_grants": views, "next_cursor": next, "limit": limit})
}

// listShareGrantRows runs the cursor-paginated list. scope supplies the
// caller's role predicate; it is the only place the user enters the query.
// There is deliberately no total: a count must never include other rows.
func listShareGrantRows(c *gin.Context, db *gorm.DB, scope func(*gorm.DB) *gorm.DB) ([]models.ShareGrant, string, int, *apperrors.AppError) {
	params, err := GetCursorParams(c)
	if err != nil {
		return nil, "", 0, err
	}
	q := scope(db.Model(&models.ShareGrant{}))
	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("share_grants", params.Cursor, params.Cursor.ID, desc)
		q = q.Where(pred, t, idv)
	}
	var rows []models.ShareGrant
	if e := cursorOrderBy(q, "share_grants", desc).Limit(params.Limit + 1).Find(&rows).Error; e != nil {
		return nil, "", 0, apperrors.ErrDatabase("Failed to retrieve shares").WithError(e)
	}
	next := ""
	if len(rows) > params.Limit {
		rows = rows[:params.Limit]
		next = EncodeCursor(rows[len(rows)-1].UpdatedAt, rows[len(rows)-1].ID)
	}
	return rows, next, params.Limit, nil
}

// UpdateShareGrant — PATCH /share-grants/{id} (owner).
func UpdateShareGrant(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, err := middleware.GetValidated[models.ShareGrantPatchInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	now := shareGrantNow(c)
	g, appErr := services.UpdateShareGrant(db, userID, c.Param("id"), *input, func(rev int64) bool {
		return ifMatchSatisfied(c.GetHeader("If-Match"), rev)
	}, now)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share_grant": services.OwnerView(db, g, now)})
}

func recipientAction(c *gin.Context, fn func(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError)) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	now := shareGrantNow(c)
	g, appErr := fn(db, userID, c.Param("id"), now)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share_grant": services.RecipientView(db, g, now)})
}

func ownerAction(c *gin.Context, fn func(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError)) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	now := shareGrantNow(c)
	g, appErr := fn(db, userID, c.Param("id"), now)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share_grant": services.OwnerView(db, g, now)})
}

// AcceptShareGrant — POST /share-grants/{id}/accept (recipient).
func AcceptShareGrant(c *gin.Context) { recipientAction(c, services.AcceptShareGrant) }

// LeaveShareGrant — POST /share-grants/{id}/leave (recipient).
func LeaveShareGrant(c *gin.Context) { recipientAction(c, services.LeaveShareGrant) }

// DeclineShareGrant — POST /share-grants/{id}/decline (recipient), optional
// {"block": true}.
func DeclineShareGrant(c *gin.Context) {
	var body models.ShareGrantDeclineInput
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil && err.Error() != "EOF" {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("body", "invalid request body").WithReason(apperrors.ReasonValidationInvalidRequestBody, nil))
			return
		}
	}
	recipientAction(c, func(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
		return services.DeclineShareGrant(db, userID, id, body.Block, now)
	})
}

// RevokeShareGrant — POST /share-grants/{id}/revoke (owner).
func RevokeShareGrant(c *gin.Context) { ownerAction(c, services.RevokeShareGrant) }

// ReconfirmShareGrant — POST /share-grants/{id}/reconfirm (owner).
func ReconfirmShareGrant(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	now := shareGrantNow(c)
	g, appErr := services.ReconfirmShareGrant(db, userID, c.Param("id"), now)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	models.RecordShareGrantAudit(db, g, userID, models.AuditOpUpdate)
	c.JSON(http.StatusOK, gin.H{"share_grant": services.OwnerView(db, g, now)})
}

// GetShareGrantContent — GET /share-grants/{id}/content (recipient): the one
// recipient read route.
func GetShareGrantContent(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	content, appErr := services.ReadShareGrantContent(db, userID, c.Param("id"), currentConfig(c).ProfilePhotoDir, shareGrantNow(c))
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share_grant_content": content})
}

// ListShareBlocks — GET /share-grants/blocks (recipient's own block list).
func ListShareBlocks(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	blocks, appErr := services.ListShareBlocks(db, userID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"blocks": blocks})
}

// DeleteShareBlock — DELETE /share-grants/blocks/{user_id} (unblock).
func DeleteShareBlock(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	blocked, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("user_id", "must be a positive integer").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "user_id"}))
		return
	}
	if appErr := services.DeleteShareBlock(db, userID, uint(blocked)); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.Status(http.StatusNoContent)
}
