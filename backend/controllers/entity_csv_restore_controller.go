package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// entityCSVRestoreInFlight holds the user ids with a restore running. One
// restore per user at a time: the restore is a single transaction over the
// whole file, and a second concurrent one could only repeat the first's rows.
var entityCSVRestoreInFlight sync.Map

// acquireEntityCSVRestore claims the user's restore slot. When ok, the caller
// must call release exactly once.
func acquireEntityCSVRestore(userID uint) (release func(), ok bool) {
	if _, loaded := entityCSVRestoreInFlight.LoadOrStore(userID, struct{}{}); loaded {
		return nil, false
	}
	return func() { entityCSVRestoreInFlight.Delete(userID) }, true
}

// RestoreEntitiesCSV restores the entity sections (organisations, places,
// pets, their links and field values) of a CSV backup produced by GET /export
// into the caller's account (issue #2300, ADR 0043 decision 7 as amended
// 2026-11-10). It is a dedicated endpoint: the contacts column-mapping import
// is unchanged. The upload is multipart ("file"), capped at services.MaxCSVSize,
// and applied in one transaction by services.RestoreEntityCSV, which re-mints
// ids behind the import ledger, validates every row like a REST write and
// reports (never creates) a contact or field definition the CSV cannot carry.
func RestoreEntitiesCSV(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "No file uploaded").WithReason(apperrors.ReasonImportFileMissing, nil))
		return
	}
	if file.Size > services.MaxCSVSize {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", fmt.Sprintf("File too large. Maximum size is %d MB", services.MaxCSVSize/(1024*1024))).WithReason(apperrors.ReasonCSVFileTooLarge, map[string]any{"max_mb": services.MaxCSVSize / (1024 * 1024)}))
		return
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".csv") {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "File must be a CSV file").WithReason(apperrors.ReasonCSVFileNotCSV, nil))
		return
	}

	release, acquired := acquireEntityCSVRestore(userID)
	if !acquired {
		apperrors.AbortWithError(c, apperrors.NewError(
			apperrors.ErrCodeRateLimitExceeded,
			"An entity restore is already running for this account. Wait for it to finish, then retry.",
			http.StatusTooManyRequests,
		))
		return
	}
	defer release()

	f, err := file.Open()
	if err != nil { // # pragma: no cover — defensive: a parsed multipart part always reopens
		log.Error().Err(err).Msg("Failed to open uploaded entity CSV")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to process file"))
		return
	}
	defer f.Close()

	report, err := services.RestoreEntityCSV(c.Request.Context(), db, userID, f)
	if err != nil {
		if services.IsEntityCSVRejection(err) {
			log.Info().Err(err).Msg("Entity CSV restore rejected")
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", err.Error()).WithReason(apperrors.ReasonImportFileUnreadable, map[string]any{"reason": err.Error()}))
			return
		}
		log.Error().Err(err).Msg("Entity CSV restore failed")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to restore entities"))
		return
	}

	resp := models.EntityCSVRestoreResponse{
		EntitiesCreated:    report.EntitiesCreated,
		LinksCreated:       report.LinksCreated,
		FieldValuesCreated: report.FieldValuesCreated,
		Issues:             make([]models.EntityCSVRestoreIssue, 0, len(report.Issues)),
	}
	for _, i := range report.Issues {
		resp.Issues = append(resp.Issues, models.EntityCSVRestoreIssue{Record: i.Record, Field: i.Field, Category: i.Category, Message: i.Message})
	}
	log.Info().
		Int("entities", resp.EntitiesCreated).
		Int("links", resp.LinksCreated).
		Int("field_values", resp.FieldValuesCreated).
		Int("issues", len(resp.Issues)).
		Msg("Entity CSV restored")
	c.JSON(http.StatusOK, resp)
}
