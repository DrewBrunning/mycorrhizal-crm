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
		start := time.Now() // rawtime:allow elapsed-duration measurement for a latency/duration log or metric; the value is never compared to a stored instant, so a pinned clock adds nothing
		// Path and query are user-controlled; sanitize control characters so a
		// crafted request cannot inject forged lines into the log stream, and
		// redact sensitive query values (e.g. the OIDC authorization code) so
		// credentials never land in the logs.
		path := logger.SanitizeLogField(c.Request.URL.Path)
		query := logger.SanitizeLogField(logger.RedactQueryValues(c.Request.URL.RawQuery))

		// Process request
		c.Next()

		// Calculate request duration
		duration := time.Since(start) // rawtime:allow elapsed-duration measurement for a latency/duration log or metric; the value is never compared to a stored instant, so a pinned clock adds nothing

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

		// Create log event. A 5xx is server misbehaviour and logs at error; a
		// 4xx (a rejected request, a 404, a failed login) is the client's
		// outcome, not a server fault, so it logs at info. This is the same
		// "a correctly rejected request is not server misbehaviour" rule
		// issue #1474 applied to errors.LogError and the validation
		// middleware, extended to the access log so the warn/error log guard
		// is not permanently red on the negative cases every E2E run drives.
		event := logger.Logger.Info()
		if statusCode >= 500 {
			event = logger.Logger.Error()
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
