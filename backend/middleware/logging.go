package middleware

import (
	"mycorrhizal/logger"
	"time"

	"github.com/gin-gonic/gin"
)

// LoggingMiddleware logs HTTP requests with structured logging
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		// Path and query are user-controlled; sanitize control characters so a
		// crafted request cannot inject forged lines into the log stream, and
		// redact sensitive query values (e.g. the OIDC authorization code) so
		// credentials never land in the logs.
		path := logger.SanitizeLogField(c.Request.URL.Path)
		query := logger.SanitizeLogField(logger.RedactQueryValues(c.Request.URL.RawQuery))

		// Process request
		c.Next()

		// Calculate request duration
		duration := time.Since(start)

		// Get status code
		statusCode := c.Writer.Status()

		// Get request ID
		requestID, _ := c.Get("request_id")
		requestIDStr, _ := requestID.(string)

		// Get user ID if available
		var userID uint
		if uid, exists := c.Get("userID"); exists {
			if id, ok := uid.(uint); ok {
				userID = id
			}
		}

		// Create log event
		event := logger.Logger.Info()

		// Add level based on status code
		if statusCode >= 500 {
			event = logger.Logger.Error()
		} else if statusCode >= 400 {
			event = logger.Logger.Warn()
		}

		// Build log entry
		logEntry := event.
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", statusCode).
			Dur("duration", duration).
			Str("ip", c.ClientIP()).
			Str("user_agent", logger.SanitizeLogField(c.Request.UserAgent()))

		if requestIDStr != "" {
			logEntry = logEntry.Str("request_id", requestIDStr)
		}

		if userID > 0 {
			logEntry = logEntry.Uint("user_id", userID)
		}

		if query != "" {
			logEntry = logEntry.Str("query", query)
		}

		// Add error message if present. Errors can echo user-controlled values
		// (validation failures, parse errors), so sanitize before logging.
		if len(c.Errors) > 0 {
			logEntry = logEntry.Str("error", logger.SanitizeLogField(c.Errors.String()))
		}

		// Log the request
		logEntry.Msg("HTTP request")
	}
}
