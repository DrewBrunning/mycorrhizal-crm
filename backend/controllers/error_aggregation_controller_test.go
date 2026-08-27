package controllers

import (
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
)

type errorBucketRow struct {
	Component         string    `json:"component"`
	Cause             string    `json:"cause"`
	SampleError       string    `json:"sample_error"`
	EventTypes        []string  `json:"event_types"`
	Count             int       `json:"count"`
	Recurring         bool      `json:"recurring"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
	EventIDs          []uint    `json:"event_ids"`
	EventIDsTruncated bool      `json:"event_ids_truncated"`
}

type errorAggregationResponse struct {
	WindowHours int              `json:"window_hours"`
	Since       time.Time        `json:"since"`
	Until       time.Time        `json:"until"`
	TotalEvents int              `json:"total_events"`
	Buckets     []errorBucketRow `json:"buckets"`
}

// TestGetErrorAggregation exercises GET /admin/error-aggregation (issue #426).
// One real migrated DB is shared across the sub-cases (each clears
// system_events first) so migrations run once, not per case.
func TestGetErrorAggregation(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "error-aggregation-ctrl.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/admin/error-aggregation", GetErrorAggregation)

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	}

	get := func(t *testing.T, query string) errorAggregationResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/error-aggregation"+query, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp errorAggregationResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	t.Run("fresh DB is an empty window with the default 24h", func(t *testing.T) {
		reset(t)
		resp := get(t, "")
		assert.Equal(t, 24, resp.WindowHours)
		assert.Equal(t, 0, resp.TotalEvents)
		assert.Empty(t, resp.Buckets)
		assert.WithinDuration(t, time.Now(), resp.Until, 5*time.Second)
		assert.WithinDuration(t, time.Now().Add(-24*time.Hour), resp.Since, 5*time.Second)
	})

	t.Run("failures collapse by cause, most frequent first, with the event ids", func(t *testing.T) {
		reset(t)
		now := time.Now()
		var ids []uint
		for i := 0; i < 9; i++ {
			ev := models.SystemEvent{
				Component:  "contact_sync",
				EventType:  models.SysEventSyncFailed,
				OccurredAt: now.Add(-time.Duration(i) * time.Minute),
				Error:      "carddav auth rejected (HTTP 401) for subscription " + strconv.Itoa(i),
			}
			require.NoError(t, db.Create(&ev).Error)
			ids = append(ids, ev.ID)
		}
		require.NoError(t, db.Create(&models.SystemEvent{
			Component: "notification", EventType: models.SysEventNotificationFailed,
			OccurredAt: now.Add(-time.Minute), Error: "smtp timeout",
		}).Error)

		resp := get(t, "")
		assert.Equal(t, 10, resp.TotalEvents)
		require.Len(t, resp.Buckets, 2)

		top := resp.Buckets[0]
		assert.Equal(t, "contact_sync", top.Component)
		assert.Equal(t, 9, top.Count)
		assert.True(t, top.Recurring)
		assert.Len(t, top.EventIDs, 9)
		assert.ElementsMatch(t, ids, top.EventIDs)

		assert.Equal(t, 1, resp.Buckets[1].Count)
		assert.False(t, resp.Buckets[1].Recurring)
	})

	t.Run("window_hours narrows the window", func(t *testing.T) {
		reset(t)
		now := time.Now().UTC()
		require.NoError(t, db.Create(&models.SystemEvent{
			Component: "contact_sync", EventType: models.SysEventSyncFailed,
			OccurredAt: now.Add(-2 * time.Hour), Error: "recent",
		}).Error)
		require.NoError(t, db.Create(&models.SystemEvent{
			Component: "contact_sync", EventType: models.SysEventSyncFailed,
			OccurredAt: now.Add(-10 * time.Hour), Error: "old",
		}).Error)

		resp := get(t, "?window_hours=6")
		assert.Equal(t, 6, resp.WindowHours)
		assert.Equal(t, 1, resp.TotalEvents)
		require.Len(t, resp.Buckets, 1)
		assert.Equal(t, "recent", resp.Buckets[0].Cause)
	})

	t.Run("a bad window_hours is a 400", func(t *testing.T) {
		reset(t)
		for _, q := range []string{"?window_hours=0", "?window_hours=999", "?window_hours=abc"} {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/admin/error-aggregation"+q, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, q)
		}
	})
}
