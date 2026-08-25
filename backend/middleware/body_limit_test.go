package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBodySizeLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		maxBytes       int64
		bodySize       int
		expectedStatus int
	}{
		{
			name:           "body within limit",
			maxBytes:       1024,
			bodySize:       512,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "body at exact limit",
			maxBytes:       1024,
			bodySize:       1024,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "body exceeds limit",
			maxBytes:       1024,
			bodySize:       2048,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "empty body",
			maxBytes:       1024,
			bodySize:       0,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(BodySizeLimitMiddleware(tt.maxBytes))
			router.POST("/test", func(c *gin.Context) {
				// Try to read the body - this triggers the size check
				body := make([]byte, tt.bodySize+1)
				_, err := c.Request.Body.Read(body)
				if err != nil && err.Error() == "http: request body too large" {
					c.AbortWithStatus(http.StatusRequestEntityTooLarge)
					return
				}
				c.Status(http.StatusOK)
			})

			body := bytes.NewReader(bytes.Repeat([]byte("x"), tt.bodySize))
			req := httptest.NewRequest(http.MethodPost, "/test", body)
			req.Header.Set("Content-Type", "application/octet-stream")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestJSONBodySizeLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(JSONBodySizeLimitMiddleware())
	router.POST("/test", func(c *gin.Context) {
		body := make([]byte, MaxJSONBodySize+1)
		_, err := c.Request.Body.Read(body)
		if err != nil && err.Error() == "http: request body too large" {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	// Test body within limit
	t.Run("within limit", func(t *testing.T) {
		body := strings.NewReader(`{"test": "data"}`)
		req := httptest.NewRequest(http.MethodPost, "/test", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Test body exceeding 1MB limit
	t.Run("exceeds limit", func(t *testing.T) {
		// Create a body larger than 1MB
		largeBody := bytes.Repeat([]byte("x"), MaxJSONBodySize+1)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}

func TestDefaultBodySizeLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Verify the default limit is 10MB
	assert.Equal(t, int64(10<<20), int64(DefaultMaxBodySize))

	router := gin.New()
	router.Use(DefaultBodySizeLimitMiddleware())
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Test small body passes
	body := strings.NewReader(`{"test": "data"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestDefaultBodySizeLimitMiddleware_ExemptPathBypassesDefaultLimit pins
// issue #416's fix: an exempt path (registered in largeBodyRoutePaths, e.g.
// the VCF import upload route) must pass an over-10MB body straight through
// DefaultBodySizeLimitMiddleware untouched, while every other path keeps
// enforcing the strict 10MB default. This is what lets routes.go's own,
// larger BodySizeLimitMiddleware(services.MaxVCFSize) actually take effect
// on that route — without the exemption, this engine-wide middleware runs
// first and always wins, making the route-specific override dead code.
func TestDefaultBodySizeLimitMiddleware_ExemptPathBypassesDefaultLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(DefaultBodySizeLimitMiddleware())
	// The exempt path applies its own, larger limit -- exactly like
	// routes.go does for the real import upload routes.
	router.POST("/api/v1/contacts/import/vcf/upload", BodySizeLimitMiddleware(50<<20), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.POST("/api/v1/other", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	oversized := bytes.Repeat([]byte("x"), 11<<20) // 11MB: over the 10MB default, under the 50MB override

	// httptest.NewRequest doesn't populate req.Header's Content-Length (only
	// the real HTTP transport does that on the wire) -- set it explicitly so
	// this exercises BodySizeLimitMiddleware's header-based fast-reject path,
	// which is what a real multipart upload's Content-Length triggers.
	newOversizedRequest := func(path string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(oversized))
		req.Header.Set("Content-Length", strconv.Itoa(len(oversized)))
		return req
	}

	t.Run("exempt path accepts a body over the default limit", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newOversizedRequest("/api/v1/contacts/import/vcf/upload"))
		assert.Equal(t, http.StatusOK, w.Code, "an allowlisted route must not be capped by the engine-wide default")
	})

	t.Run("a non-exempt path still enforces the default limit", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newOversizedRequest("/api/v1/other"))
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "the exemption must not leak to routes outside the allowlist")
	})
}
