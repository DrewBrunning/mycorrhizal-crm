package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetDuplicatePairs is T93's read endpoint
// (docs/fork-plan/tickets/137-T93-duplicate-scan-endpoint-and-review.md):
// returns the caller's duplicate-candidate pairs strongest-first, offset-
// paginated. The pairs are computed fresh on every request by
// services.FindDuplicatePairs — a constant number of SQL queries regardless
// of contact count — with already-dismissed pairs filtered out. Read-only:
// nothing is persisted here.
func GetDuplicatePairs(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	pairs, err := services.FindDuplicatePairs(db, userID)
	if err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Failed to scan for duplicate pairs")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("scan for duplicate pairs").WithError(err))
		return
	}

	params := GetPaginationParams(c)
	total := len(pairs)
	start := params.Offset
	if start > total {
		start = total
	}
	end := start + params.Limit
	if end > total {
		end = total
	}

	// pairs[start:end] on a non-nil empty slice stays a non-nil slice, so an
	// empty scan still serializes as `"pairs":[]` — never null (CLAUDE.md
	// frontend trap #8; pinned by a raw-JSON test).
	c.JSON(http.StatusOK, models.DuplicatePairsResponse{
		Pairs: pairs[start:end],
		Total: total,
		Page:  params.Page,
		Limit: params.Limit,
	})
}

// DismissDuplicatePair records that a pair is not a duplicate, so the scanner
// never offers it again (T93 — dismissal must persist or the same twins /
// father-and-son pair becomes noise on every visit). Idempotent: dismissing an
// already-dismissed pair is a no-op success, so a double-click cannot 409.
//
// Both uids must be the caller's own contacts (ownership scoping — a 404 also
// covers the case where one contact was deleted since the scan, in which case
// there is nothing to dismiss anyway).
func DismissDuplicatePair(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.DuplicateDismissalInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	var count int64
	if err := db.Model(&models.Contact{}).
		Where("user_id = ? AND vcard_uid IN ?", userID, []string{input.UIDA, input.UIDB}).
		Count(&count).Error; err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Failed to verify contacts for duplicate dismissal")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("verify contacts").WithError(err))
		return
	}
	if count != 2 {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact"))
		return
	}

	uidLow, uidHigh := input.UIDA, input.UIDB
	if uidLow > uidHigh {
		uidLow, uidHigh = uidHigh, uidLow
	}

	// Insert, tolerating a duplicate: the (user_id, uid_low, uid_high) unique
	// index is the idempotency guard. A count-then-create would race — two
	// concurrent dismissals of the same pair could both see "absent" and the
	// second INSERT would then 500 on the index, violating the idempotent
	// contract. Instead the unique-constraint error IS the "already dismissed"
	// answer (double-click safe, concurrent-call safe). The string match is
	// the house idiom for a unique-index violation (admin_user_controller.go)
	// — GORM only translates to gorm.ErrDuplicatedKey with TranslateError,
	// which this app does not enable.
	dismissal := models.DismissedDuplicatePair{UserID: userID, UIDLow: uidLow, UIDHigh: uidHigh}
	if err := db.Create(&dismissal).Error; err != nil && !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		logger.FromContext(c).Error().Err(err).Msg("Failed to record duplicate dismissal")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("record duplicate dismissal").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pair dismissed"})
}
