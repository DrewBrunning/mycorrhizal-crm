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

// meerkatImportSessions owns all in-progress Meerkat import wizard state,
// including each session's uploaded-file temp directory (issue #550).
var meerkatImportSessions = services.NewMeerkatImportManager()

func rejectMeerkatSessionOverLimit(c *gin.Context) bool {
	userID, ok := currentUserID(c)
	if !ok {
		return true
	}
	if meerkatImportSessions.CountActive(userID) >= services.MaxMeerkatImportSessionsPerUser {
		apperrors.AbortWithError(c, apperrors.NewError(
			apperrors.ErrCodeRateLimitExceeded,
			"Too many in-progress Meerkat imports. Finish or cancel an existing one first.",
			http.StatusTooManyRequests,
		))
		return true
	}
	return false
}

// UploadMeerkatDatabase accepts the uploaded Meerkat SQLite file, validates
// and stages it, and returns the source-user picker + a new session ID.
func UploadMeerkatDatabase(c *gin.Context) {
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	go meerkatImportSessions.CleanupExpired()
	if rejectMeerkatSessionOverLimit(c) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "No file uploaded"))
		return
	}

	resp, appErr := meerkatImportSessions.Upload(userID, file)
	if appErr != nil {
		log.Warn().Str("code", appErr.Code).Msg("Meerkat upload rejected")
		apperrors.AbortWithError(c, appErr)
		return
	}

	log.Info().
		Str("session_id", resp.SessionID).
		Int("contacts", resp.Totals.Contacts).
		Int("source_users", len(resp.SourceUsers)).
		Msg("Meerkat database uploaded")

	c.JSON(http.StatusOK, resp)
}

// StartMeerkatFetch launches the background map + preview build.
func StartMeerkatFetch(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	req, validationErr := middleware.GetValidated[models.MeerkatFetchRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	if appErr := meerkatImportSessions.StartFetch(db, userID, *req, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// GetMeerkatImportStatus reports map/import progress for polling.
func GetMeerkatImportStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	status, appErr := meerkatImportSessions.Status(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetMeerkatImportPreview returns the review rows + loss report.
func GetMeerkatImportPreview(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	preview, appErr := meerkatImportSessions.Preview(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, preview)
}

// ConfirmMeerkatImport starts the import in the background (202); the client
// polls GET /status for progress and the result.
func ConfirmMeerkatImport(c *gin.Context) {
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

	if appErr := meerkatImportSessions.Confirm(db, userID, *req, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// CancelMeerkatImport cancels an in-flight import (rolls it back) or, in any
// other phase, drops the session and deletes its temp file.
func CancelMeerkatImport(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}
	if appErr := meerkatImportSessions.Cancel(userID, sessionID); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}
