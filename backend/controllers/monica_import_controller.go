package controllers

import (
	"net/http"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// monicaImportSessions owns all in-progress Monica import wizard state,
// including the API tokens (in memory only). Controllers validate HTTP input
// and delegate to the manager (issue #549).
var monicaImportSessions = services.NewMonicaImportManager()

// rejectMonicaSessionOverLimit aborts with 429 when the caller already holds
// the maximum number of live Monica import sessions (issue #415), mirroring
// rejectImportSessionOverLimit for the file imports.
func rejectMonicaSessionOverLimit(c *gin.Context) bool {
	userID, ok := currentUserID(c)
	if !ok {
		return true
	}
	if monicaImportSessions.CountActive(userID) >= services.MaxMonicaImportSessionsPerUser {
		apperrors.AbortWithError(c, apperrors.NewError(
			apperrors.ErrCodeRateLimitExceeded,
			"Too many in-progress Monica imports. Finish or cancel an existing one before starting another.",
			http.StatusTooManyRequests,
		))
		return true
	}
	return false
}

// ConnectMonicaImport validates the Monica URL and API token and returns the
// account's entity counts together with a new session ID.
func ConnectMonicaImport(c *gin.Context, cfg *config.Config) {
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	go monicaImportSessions.CleanupExpired()
	if rejectMonicaSessionOverLimit(c) {
		return
	}

	req, validationErr := middleware.GetValidated[models.MonicaConnectRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	resp, appErr := monicaImportSessions.Connect(c.Request.Context(), userID, *req, cfg.MonicaBlockPrivateURLs)
	if appErr != nil {
		log.Warn().Str("code", appErr.Code).Msg("Monica connect failed")
		apperrors.AbortWithError(c, appErr)
		return
	}

	log.Info().
		Str("session_id", resp.SessionID).
		Int("contacts", resp.Totals.Contacts).
		Msg("Monica import session connected")

	c.JSON(http.StatusOK, resp)
}

// StartMonicaFetch launches the background snapshot fetch for a session.
func StartMonicaFetch(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	req, validationErr := middleware.GetValidated[models.MonicaFetchRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	if appErr := monicaImportSessions.StartFetch(db, userID, *req, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// GetMonicaImportStatus reports fetch/import progress for polling.
func GetMonicaImportStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}

	status, appErr := monicaImportSessions.Status(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetMonicaImportPreview returns the review rows of a fetched snapshot,
// including the loss report.
func GetMonicaImportPreview(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}

	preview, appErr := monicaImportSessions.Preview(userID, sessionID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusOK, preview)
}

// ConfirmMonicaImport starts the import in the background (202); the client
// polls GET /status for progress and the result.
func ConfirmMonicaImport(c *gin.Context, cfg *config.Config) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	req, validationErr := middleware.GetValidated[models.MonicaConfirmRequest](c)
	if validationErr != nil {
		apperrors.AbortWithError(c, validationErr)
		return
	}

	if appErr := monicaImportSessions.Confirm(db, userID, *req, cfg, log); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_id": req.SessionID})
}

// CancelMonicaImport cancels an in-flight import (rolls it back, phase
// "cancelled") or, in any other phase, drops the session and its API token.
func CancelMonicaImport(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("session_id"))
		return
	}

	if appErr := monicaImportSessions.Cancel(userID, sessionID); appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}
