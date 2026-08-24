package errors

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/logger"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func decodeErrorDetail(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	detail, ok := body["error"].(map[string]interface{})
	require.True(t, ok, "response must carry an 'error' object, got: %v", body)
	return detail
}

// --- RespondWithError / AbortWithError: the JSON envelope contract ---

func TestRespondWithError_JSONEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/contacts", nil)
	c.Set("request_id", "req-123")

	RespondWithError(c, ErrNotFound("Contact").WithDetails("id", "42"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.True(t, c.IsAborted(), "RespondWithError must abort the request")

	body := decodeErrorEnvelope(t, rec)
	detail := decodeErrorDetail(t, body)
	assert.Equal(t, ErrCodeNotFound, detail["code"])
	assert.Equal(t, "Contact not found", detail["message"])
	details, ok := detail["details"].(map[string]interface{})
	require.True(t, ok, "details should be an object")
	assert.Equal(t, "42", details["id"])
	assert.Equal(t, "req-123", body["request_id"])
	assert.NotEmpty(t, body["timestamp"])
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"))
}

func TestRespondWithError_StatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		err  *AppError
	}{
		{"unauthorized", ErrUnauthorized("denied")},
		{"forbidden", ErrForbidden("no")},
		{"conflict", ErrAlreadyExists("Circle")},
		{"validation", ErrValidation("bad")},
		{"rate limited", ErrRateLimitExceeded()},
		{"internal", ErrInternal("oops")},
		{"external", ErrExternal("seafile", "down")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			RespondWithError(c, tt.err)

			assert.Equal(t, tt.err.HTTPStatus, rec.Code)
			detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
			assert.Equal(t, tt.err.Code, detail["code"])
			assert.Equal(t, tt.err.Message, detail["message"])
		})
	}
}

func TestRespondWithError_OmitsEmptyRequestIDAndDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondWithError(c, ErrInternal("oops"))

	body := decodeErrorEnvelope(t, rec)
	assert.NotContains(t, body, "request_id", "an absent request_id must be omitted, not empty")
	detail := decodeErrorDetail(t, body)
	assert.NotContains(t, detail, "details", "errors without details must omit the key, not emit null")
}

func TestAbortWithError_DelegatesToRespondWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	AbortWithError(c, ErrValidation("nope"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.True(t, c.IsAborted())
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeValidation, detail["code"])
}

// --- ErrorHandlerMiddleware ---

func TestErrorHandlerMiddleware_RecoversPanicAsInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeInternal, detail["code"])
	assert.Equal(t, "An unexpected error occurred", detail["message"])

	// Fail-secure: the panic value, the stack trace, and any Go type/struct
	// internals must never leak into the response body.
	assert.NotContains(t, rec.Body.String(), "boom")
	assert.NotContains(t, detail, "details", "panic path must not expose a details object")
	assert.NotContains(t, rec.Body.String(), "goroutine")
	assert.NotContains(t, rec.Body.String(), "runtime/")
	assert.NotContains(t, rec.Body.String(), "Panic recovered")
}

// TestErrorHandlerMiddleware_PanicDoesNotLeakStructOrTypeNames pins the
// "don't leak" bar for a panic whose value is a rich Go object: a pointer to a
// struct with identifying fields, a `%v` type name, and a wrapped error string.
// None of that may appear in the generic 500 body.
func TestErrorHandlerMiddleware_PanicDoesNotLeakStructOrTypeNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/panic", func(c *gin.Context) {
		type internalSecret struct {
			APIKey string
			DSN    string
		}
		panic(&internalSecret{APIKey: "sk_live_12345", DSN: "postgres://user:pass@db/secret"})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "internalSecret", "Go type name must not leak")
	assert.NotContains(t, body, "sk_live_12345", "struct field values must not leak")
	assert.NotContains(t, body, "postgres://user:pass@db/secret", "struct field values must not leak")
	assert.NotContains(t, body, "goroutine")
	assert.NotContains(t, body, "runtime/")
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeInternal, detail["code"])
	assert.NotContains(t, detail, "details")
}

// TestErrorHandlerMiddleware_ReleaseMode_NoInternalDetail pins that in release
// mode the error envelope carries no internal detail — no panic value, no
// stack trace, and no underlying error string — for both a recovered panic and
// a raw (non-AppError) error wrapped by GetAppError.
func TestErrorHandlerMiddleware_ReleaseMode_NoInternalDetail(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	defer gin.SetMode(gin.TestMode)

	t.Run("panic", func(t *testing.T) {
		router := gin.New()
		router.Use(ErrorHandlerMiddleware())
		router.GET("/panic", func(c *gin.Context) {
			panic("sensitive internal value")
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		body := rec.Body.String()
		assert.NotContains(t, body, "sensitive internal value")
		assert.NotContains(t, body, "goroutine")
		assert.NotContains(t, body, "runtime/")
		detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
		assert.Equal(t, ErrCodeInternal, detail["code"])
		assert.NotContains(t, detail, "details")
	})

	t.Run("raw error wrapped as internal", func(t *testing.T) {
		router := gin.New()
		router.Use(ErrorHandlerMiddleware())
		router.GET("/err", func(c *gin.Context) {
			c.Error(errors.New("sql: database is locked (SQLITE_BUSY)"))
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		body := rec.Body.String()
		assert.NotContains(t, body, "SQLITE_BUSY", "raw driver error must not leak")
		assert.NotContains(t, body, "sql: database is locked")
		detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
		assert.Equal(t, ErrCodeInternal, detail["code"])
		assert.NotContains(t, detail, "details")
	})
}

func TestErrorHandlerMiddleware_RespondsToAppErrorInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/err", func(c *gin.Context) {
		c.Error(ErrValidation("bad input"))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeValidation, detail["code"])
	assert.Equal(t, "bad input", detail["message"])
}

func TestErrorHandlerMiddleware_WrapsPlainErrorAsInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/err", func(c *gin.Context) {
		c.Error(errors.New("some raw failure"))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeInternal, detail["code"])
}

func TestErrorHandlerMiddleware_DoesNotDoubleWriteAfterResponseSent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/written", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.WriteString("ok")
		c.Error(ErrInternal("too late to report"))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/written", nil))

	assert.Equal(t, http.StatusOK, rec.Code, "a response that was already written must not be overwritten by error middleware")
	assert.Equal(t, "ok", rec.Body.String())
}

func TestErrorHandlerMiddleware_NoErrorsPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorHandlerMiddleware())
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"fine": true})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"fine":true}`, rec.Body.String())
}

// --- LogError ---

func TestLogError_EmitsExpectedFields(t *testing.T) {
	old := logger.Logger
	buf := &bytes.Buffer{}
	logger.Logger = zerolog.New(buf).With().Timestamp().Logger()
	defer func() { logger.Logger = old }()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/contacts/1", nil)

	LogError(c, ErrDatabase("save").WithError(errors.New("inner disk failure")).WithDetails("table", "contacts"))

	out := buf.String()
	assert.Contains(t, out, `"code":"DATABASE_ERROR"`)
	assert.Contains(t, out, `"status":500`)
	assert.Contains(t, out, "Request error")
	// The underlying error and the AppError message must both surface (the
	// Err() field overwrites the earlier Str("error") in the JSON output, so
	// assert both strings independently rather than a single key value).
	assert.Contains(t, out, "inner disk failure")
	assert.Contains(t, out, "Database save operation failed")
	assert.Contains(t, out, `"table":"contacts"`)
}
