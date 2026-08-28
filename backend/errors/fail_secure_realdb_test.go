package errors

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestErrorHandler_ForcedDBFailure_SurfacesTypedCodeNotDriverError runs the
// error middleware against a real database.InitDB-migrated connection (the
// repo's real-DB idiom), closes the connection pool to force a genuine read
// failure, and asserts the client sees a typed apperrors code with a generic
// body — never the raw driver/GORM error string.
func TestErrorHandler_ForcedDBFailure_SurfacesTypedCodeNotDriverError(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	defer gin.SetMode(gin.TestMode)

	newRouter := func(t *testing.T, handler gin.HandlerFunc) *gin.Engine {
		t.Helper()
		db := dbtest.New(t)

		// Force any subsequent read to fail by closing the connection pool.
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())

		router := gin.New()
		router.Use(ErrorHandlerMiddleware())
		router.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Next()
		})
		router.GET("/read", handler)
		return router
	}

	doRead := func(router *gin.Engine) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/read", nil))
		return rec
	}

	type envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	assertGenericNoLeak := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
		body := rec.Body.String()
		assert.NotContains(t, body, "sql: database is closed", "raw driver error must not leak")
		assert.NotContains(t, body, "database/sql", "driver package names must not leak")
		assert.NotContains(t, body, "gorm.io", "GORM internals must not leak")
		assert.NotContains(t, body, "SQLITE_BUSY")
	}

	t.Run("typed ErrDatabase surfaces DATABASE_ERROR", func(t *testing.T) {
		router := newRouter(t, func(c *gin.Context) {
			// Read through the real GORM handle; the closed pool makes this fail.
			var n int
			err := c.MustGet("db").(*gorm.DB).Raw("SELECT 1").Scan(&n).Error
			AbortWithError(c, ErrDatabase("Failed to retrieve").WithError(err))
		})

		rec := doRead(router)
		assertGenericNoLeak(t, rec)

		var env envelope
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
		assert.Equal(t, ErrCodeDatabase, env.Error.Code)
		assert.Equal(t, "Database Failed to retrieve operation failed", env.Error.Message)
	})

	t.Run("raw driver error wraps as INTERNAL_ERROR", func(t *testing.T) {
		router := newRouter(t, func(c *gin.Context) {
			var n int
			err := c.MustGet("db").(*gorm.DB).Raw("SELECT 1").Scan(&n).Error
			_ = c.Error(err)
		})

		rec := doRead(router)
		assertGenericNoLeak(t, rec)

		var env envelope
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
		assert.Equal(t, ErrCodeInternal, env.Error.Code)
		assert.Equal(t, "An unexpected error occurred", env.Error.Message)
	})
}
