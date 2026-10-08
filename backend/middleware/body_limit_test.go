package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	handlerRan := false
	sawTooLarge := false
	router.POST("/test", func(c *gin.Context) {
		handlerRan = true
		body := make([]byte, MaxJSONBodySize+1)
		_, err := c.Request.Body.Read(body)
		if err != nil && err.Error() == "http: request body too large" {
			sawTooLarge = true
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
		assert.True(t, handlerRan, "handler must run for an in-limit body")
	})

	// Test body exceeding 1MB limit
	t.Run("exceeds limit", func(t *testing.T) {
		// Create a body larger than 1MB
		handlerRan, sawTooLarge = false, false
		largeBody := bytes.Repeat([]byte("x"), MaxJSONBodySize+1)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		// No Content-Length header (httptest does not set one), so the
		// MaxBytesReader wrapper is what rejects: the handler ran and saw the
		// "too large" read error.
		assert.True(t, handlerRan)
		assert.True(t, sawTooLarge, "the read must fail with the MaxBytesReader error")
	})

	t.Run("exceeds limit by Content-Length header", func(t *testing.T) {
		handlerRan = false
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(bytes.Repeat([]byte("x"), MaxJSONBodySize+1)))
		req.Header.Set("Content-Length", strconv.Itoa(MaxJSONBodySize+1))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "request body too large", body["error"])
		assert.False(t, handlerRan, "handler must not run when the header already exceeds the limit")
	})

	// A body exactly at the limit is accepted.
	t.Run("at limit", func(t *testing.T) {
		handlerRan = false
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(bytes.Repeat([]byte("x"), MaxJSONBodySize)))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerRan)
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
	var exemptReadBytes int
	router.POST("/api/v1/contacts/import/vcf/upload", BodySizeLimitMiddleware(50<<20), func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		require.NoError(t, err)
		exemptReadBytes = int(n)
		c.Status(http.StatusOK)
	})
	otherRan := false
	router.POST("/api/v1/other", func(c *gin.Context) {
		otherRan = true
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
		assert.Equal(t, len(oversized), exemptReadBytes, "the handler must run and read the entire over-default body")
	})

	t.Run("a non-exempt path still enforces the default limit", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newOversizedRequest("/api/v1/other"))
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "the exemption must not leak to routes outside the allowlist")
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "request body too large", body["error"])
		assert.False(t, otherRan, "the handler must not run for a rejected body")
	})
}
