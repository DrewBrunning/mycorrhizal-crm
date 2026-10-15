package errors

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/go-sqlite" // registers the "sqlite" driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realDriverError returns a genuine error from the SQLite driver this app
// ships, produced by running stmt on a fresh connection to path. A
// hand-built error would only prove IsDatabaseBusy matches what the test
// author thinks the driver returns.
func realDriverError(t *testing.T, dsn, stmt string) error {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(stmt)
	require.Error(t, err)
	return err
}

// realBusyError holds the write lock on one connection and asks for it on a
// second with busy_timeout(0), so the driver returns SQLITE_BUSY at once
// instead of after production's 5s wait.
func realBusyError(t *testing.T) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "busy.db")
	holder, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	conn, err := holder.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") })

	return realDriverError(t, path+"?_pragma=busy_timeout(0)", "BEGIN IMMEDIATE")
}

func TestIsDatabaseBusy(t *testing.T) {
	busy := realBusyError(t)
	require.Contains(t, busy.Error(), "SQLITE_BUSY", "sanity: the fixture must be a real lock timeout")

	assert.True(t, IsDatabaseBusy(busy))
	assert.True(t, IsDatabaseBusy(fmt.Errorf("update contact: %w", busy)), "a %%w-wrapped busy error still counts")

	assert.False(t, IsDatabaseBusy(nil))
	assert.False(t, IsDatabaseBusy(fmt.Errorf("database is locked")), "a look-alike string is not the driver's error")
	syntax := realDriverError(t, filepath.Join(t.TempDir(), "s.db"), "NOT SQL")
	assert.False(t, IsDatabaseBusy(syntax), "another SQLite error code must stay a 500")
}

func respond(t *testing.T, err *AppError) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	RespondWithError(c, err)
	return rec
}

func TestRespondWithError_DatabaseBusyIs503WithRetryAfter(t *testing.T) {
	busy := realBusyError(t)
	rec := respond(t, ErrDatabase("Failed to update contact").WithError(busy))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeDatabaseBusy, detail["code"])
	details, ok := detail["details"].(map[string]any)
	require.True(t, ok, "the original operation detail carries over")
	assert.Equal(t, "Failed to update contact", details["operation"])
	assert.False(t, strings.Contains(rec.Body.String(), "SQLITE_BUSY"), "the driver error must not reach the body")
}

func TestRespondWithError_BusyOnlyOverridesA500(t *testing.T) {
	busy := realBusyError(t)
	rec := respond(t, ErrNotFound("Contact").WithError(busy))
	assert.Equal(t, http.StatusNotFound, rec.Code, "a handler's specific status is kept")
	assert.Empty(t, rec.Header().Get("Retry-After"))

	rec = respond(t, ErrDatabase("Failed to update contact").WithError(fmt.Errorf("disk I/O error")))
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "a non-busy database failure stays a 500")
	assert.Empty(t, rec.Header().Get("Retry-After"))
}

// A plain error pushed onto c.Errors reaches RespondWithError through
// GetAppError's ErrInternal wrapping; the override must apply there too.
func TestErrorHandlerMiddleware_PlainBusyErrorIs503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	busy := realBusyError(t)
	r := gin.New()
	r.Use(ErrorHandlerMiddleware())
	r.GET("/x", func(c *gin.Context) { _ = c.Error(busy) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
}
