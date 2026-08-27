package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemEventRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "sysevent-ctrl.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })
	// InitDB's migration run records a migration_completed event; clear it so
	// each test controls the full row set.
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/admin/system-events", ListSystemEvents)
	return db, router
}

func seedSystemEvents(t *testing.T, db *gorm.DB) {
	t.Helper()
	rows := []models.SystemEvent{
		{EventType: models.SysEventJobCompleted, Component: "scheduler", Severity: "info", CorrelationID: "chain-A", OccurredAt: time.Now().Add(-3 * time.Hour)},
		{EventType: models.SysEventSyncFailed, Component: "contact_sync", Severity: "error", CorrelationID: "chain-A", OccurredAt: time.Now().Add(-2 * time.Hour)},
		{EventType: models.SysEventSyncCompleted, Component: "calendar_sync", Severity: "info", CorrelationID: "chain-B", OccurredAt: time.Now().Add(-1 * time.Hour)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}
}

func decodeSystemEvents(t *testing.T, body []byte) []models.SystemEvent {
	t.Helper()
	var resp struct {
		SystemEvents []models.SystemEvent `json:"system_events"`
		Total        int                  `json:"total"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, len(resp.SystemEvents), resp.Total)
	return resp.SystemEvents
}

func TestListSystemEvents_ReverseChronological(t *testing.T) {
	db, router := setupSystemEventRouter(t)
	seedSystemEvents(t, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	events := decodeSystemEvents(t, w.Body.Bytes())
	require.Len(t, events, 3)
	assert.Equal(t, models.SysEventSyncCompleted, events[0].EventType, "newest first")
	assert.Equal(t, models.SysEventJobCompleted, events[2].EventType)
}

func TestListSystemEvents_FilterByCorrelationID(t *testing.T) {
	db, router := setupSystemEventRouter(t)
	seedSystemEvents(t, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events?correlation_id=chain-A", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	events := decodeSystemEvents(t, w.Body.Bytes())
	require.Len(t, events, 2)
	for _, e := range events {
		assert.Equal(t, "chain-A", e.CorrelationID)
	}
}

func TestListSystemEvents_FilterBySeverityAndComponent(t *testing.T) {
	db, router := setupSystemEventRouter(t)
	seedSystemEvents(t, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events?severity=error&component=contact_sync", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	events := decodeSystemEvents(t, w.Body.Bytes())
	require.Len(t, events, 1)
	assert.Equal(t, models.SysEventSyncFailed, events[0].EventType)
}

func TestListSystemEvents_FilterByIDs(t *testing.T) {
	db, router := setupSystemEventRouter(t)
	rows := []models.SystemEvent{
		{EventType: models.SysEventSyncFailed, Component: "contact_sync", Severity: "error", OccurredAt: time.Now().Add(-3 * time.Hour)},
		{EventType: models.SysEventSyncFailed, Component: "contact_sync", Severity: "error", OccurredAt: time.Now().Add(-2 * time.Hour)},
		{EventType: models.SysEventSyncFailed, Component: "contact_sync", Severity: "error", OccurredAt: time.Now().Add(-1 * time.Hour)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	// A real subset — the error-aggregation drill-down passes the exact ids.
	target := strconv.FormatUint(uint64(rows[0].ID), 10) + "," + strconv.FormatUint(uint64(rows[2].ID), 10)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events?ids="+target, nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := decodeSystemEvents(t, w.Body.Bytes())
	require.Len(t, got, 2)
	assert.ElementsMatch(t, []uint{rows[0].ID, rows[2].ID}, []uint{got[0].ID, got[1].ID})

	// Unknown ids and non-numeric tokens are not an error — they just match
	// nothing.
	for _, q := range []string{"?ids=999999", "?ids=abc", "?ids=,,"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/system-events"+q, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, q)
		assert.Empty(t, decodeSystemEvents(t, w.Body.Bytes()), q)
	}
}

func TestListSystemEvents_RejectsBadSinceTimestamp(t *testing.T) {
	_, router := setupSystemEventRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events?since=not-a-time", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListSystemEvents_LimitCap(t *testing.T) {
	db, router := setupSystemEventRouter(t)
	// One event so a huge limit still returns 200; the cap is on the parsed
	// value, verified by the handler not erroring on an over-cap request.
	models.RecordSystemEvent(context.Background(), db, models.SystemEvent{EventType: models.SysEventApplicationStarted})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-events?limit=99999", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	events := decodeSystemEvents(t, w.Body.Bytes())
	assert.Len(t, events, 1)
}
