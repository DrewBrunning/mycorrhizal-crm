package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apperrors "mycorrhizal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateContact_LockTimeoutIs503RetryAfter is issue #1720 end to end: a
// write that cannot get SQLite's write lock within the real busy_timeout
// (5s, database.openDSN — this test genuinely waits it out) must answer a
// retryable 503 DATABASE_BUSY with Retry-After, not the generic 500
// DATABASE_ERROR it used to. The lock is held by another connection in the
// handler's own pool, the way a concurrent long writer holds it in
// production. Once the lock is released, the same request succeeds — the
// 503's "retry" advice is true.
func TestUpdateContact_LockTimeoutIs503RetryAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the real 5s busy_timeout")
	}
	env := newCWEnv(t)
	path := fmt.Sprintf("/contacts/%d", env.alice.ID)

	sqlDB, err := env.db.DB()
	require.NoError(t, err)
	holder, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	_, err = holder.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)

	w := env.do(http.MethodPut, path, "", contactCardBody("Locked", ""))
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Equal(t, "1", w.Header().Get("Retry-After"))
	var body struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, apperrors.ErrCodeDatabaseBusy, body.Error.Code)
	assert.Equal(t, "Failed to update contact", body.Error.Details["operation"])

	_, err = holder.ExecContext(context.Background(), "ROLLBACK")
	require.NoError(t, err)

	w = env.do(http.MethodPut, path, "", contactCardBody("Unlocked", ""))
	require.Equal(t, http.StatusOK, w.Code, "retrying after the lock clears succeeds: %s", w.Body.String())
}
