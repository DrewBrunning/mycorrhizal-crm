package controllers

import (
	"net/http"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// mycorrhizalImportSessions owns all in-progress account-bundle import wizard
// state (issue #1260, ADR 0028 Decision 3).
var mycorrhizalImportSessions = services.NewMycorrhizalImportManager()

func rejectMycorrhizalSessionOverLimit(c *gin.Context) bool {
	userID, ok := currentUserID(c)
	if !ok {
		return true
	}
	if mycorrhizalImportSessions.CountActive(userID) >= services.MaxMycorrhizalImportSessionsPerUser {
		apperrors.AbortWithError(c, apperrors.NewError(
			apperrors.ErrCodeRateLimitExceeded,
			"Too many in-progress Mycorrhizal imports. Finish or cancel an existing one first.",
			http.StatusTooManyRequests,
		))
		return true
	}
	return false
}

// UploadMycorrhizalBundle accepts an uploaded account bundle, validates it and
// opens an import session.
func UploadMycorrhizalBundle(c *gin.Context) {
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	go mycorrhizalImportSessions.CleanupExpired()
	if rejectMycorrhizalSessionOverLimit(c) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "No file uploaded"))
		return
	}

	resp, appErr := mycorrhizalImportSessions.Upload(userID, file)
	if appErr != nil {
		log.Warn().Str("code", appErr.Code).Msg("Mycorrhizal bundle upload rejected")
		apperrors.AbortWithError(c, appErr)
		return
	}

	log.Info().
		Str("session_id", resp.SessionID).
		Int("contacts", resp.Totals.Contacts).
		Int("version", resp.Version).
		Msg("Account bundle uploaded")

	c.JSON(http.StatusOK, resp)
}

// StartMycorrhizalFetch launches the background map + preview build.
func StartMycorrhizalFetch(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	req, validationErr := middleware.GetValidated[models.MycorrhizalFetchRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	if appErr := mycorrhizalImportSessions.StartFetch(db, userID, *req, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// GetMycorrhizalImportStatus reports map/import progress for polling.
func GetMycorrhizalImportStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	status, appErr := mycorrhizalImportSessions.Status(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetMycorrhizalImportPreview returns the review rows + loss report.
func GetMycorrhizalImportPreview(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	preview, appErr := mycorrhizalImportSessions.Preview(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, preview)
}

// ConfirmMycorrhizalImport starts the import in the background (202).
func ConfirmMycorrhizalImport(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	req, validationErr := middleware.GetValidated[models.SourceImportConfirmRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	if appErr := mycorrhizalImportSessions.Confirm(db, userID, *req, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// CancelMycorrhizalImport cancels an in-flight import (rolls it back) or, in
// any other phase, drops the session.
func CancelMycorrhizalImport(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	if appErr := mycorrhizalImportSessions.Cancel(userID, sessionID); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}
