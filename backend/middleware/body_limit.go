package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Default body size limits
const (
	DefaultMaxBodySize = 10 << 20 // 10 MB
	MaxJSONBodySize    = 1 << 20  // 1 MB
)

// largeBodyRoutePaths is an explicit allowlist of routes that need a body
// larger than DefaultMaxBodySize and apply their own, larger
// BodySizeLimitMiddleware directly on the route in routes.go (see the
// /contacts/import/* upload registrations). DefaultBodySizeLimitMiddleware
// runs engine-wide (main.go, before any route matches), so without this
// exemption its 10MB check/wrap would always win over a bigger per-route
// limit registered later in the chain — a route-specific middleware cannot
// loosen a stricter one that already ran. Kept as a hardcoded allowlist,
// not a general opt-out mechanism, so a new route defaults to the strict
// global cap unless someone deliberately widens it here *and* gives it its
// own explicit limit in routes.go.
//
// Issue #416: this pairs with services.MaxCSVSize/MaxVCFSize, which used to
// be dead code — any request over the global 10MB was already rejected here
// before those larger, documented limits could ever be reached.
var largeBodyRoutePaths = map[string]bool{
	"/api/v1/contacts/import/upload":           true,
	"/api/v1/contacts/import/vcf/upload":       true,
	"/api/v1/contacts/import/jscontact/upload": true,
	"/api/v1/contacts/import/meerkat/upload":   true, // Meerkat SQLite file (issue #550, MaxMeerkatDBSize)
}

// BodySizeLimitMiddleware limits the size of request bodies. For requests
// with a Content-Length header exceeding maxBytes, the middleware aborts
// with 413 immediately. For chunked/streaming bodies without Content-Length,
// the MaxBytesReader wrapper will terminate the read and Gin's JSON binding
// will produce a parse error (not a 413, but the request is still rejected).
func BodySizeLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if lengthStr := c.Request.Header.Get("Content-Length"); lengthStr != "" {
			if length, err := strconv.ParseInt(lengthStr, 10, 64); err == nil && length > maxBytes {
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func JSONBodySizeLimitMiddleware() gin.HandlerFunc {
	return BodySizeLimitMiddleware(MaxJSONBodySize)
}

func DefaultBodySizeLimitMiddleware() gin.HandlerFunc {
	limited := BodySizeLimitMiddleware(DefaultMaxBodySize)
	return func(c *gin.Context) {
		if largeBodyRoutePaths[c.Request.URL.Path] {
			c.Next()
			return
		}
		limited(c)
	}
}
