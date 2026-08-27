package middleware

import (
	"mycorrhizal/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID already exists in header
		requestID := c.GetHeader("X-Request-ID")

		// Generate new ID if not present
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in context for use throughout the request
		c.Set("request_id", requestID)

		// Add to response headers
		c.Header("X-Request-ID", requestID)

		// Bind the same ID as the correlation ID on the request's
		// context.Context, so any background work or outbound call a handler
		// starts from c.Request.Context() shares one ID with the HTTP log
		// lines (issue #425).
		c.Request = c.Request.WithContext(
			logger.WithCorrelationID(c.Request.Context(), requestID),
		)

		c.Next()
	}
}
