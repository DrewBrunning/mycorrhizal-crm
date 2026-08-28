package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type subsystemHealthRow struct {
	Subsystem              string     `json:"subsystem"`
	Status                 string     `json:"status"`
	LastAttemptAt          *time.Time `json:"last_attempt_at"`
	LastSuccessAt          *time.Time `json:"last_success_at"`
	LastFailureAt          *time.Time `json:"last_failure_at"`
	IncidentFirstFailureAt *time.Time `json:"incident_first_failure_at"`
	ConsecutiveFailures    int        `json:"consecutive_failures"`
	LastError              string     `json:"last_error"`
}

func getSubsystemHealth(t *testing.T, router *gin.Engine) []subsystemHealthRow {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/subsystem-health", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Subsystems []subsystemHealthRow `json:"subsystems"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Subsystems
}

// TestGetSubsystemHealth exercises GET /admin/subsystem-health (issue #427).
// One migrated DB is shared across the sub-cases (each clears system_events
// first).
func TestGetSubsystemHealth(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/admin/subsystem-health", GetSubsystemHealth)

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	}

	t.Run("fresh DB lists every subsystem as unknown, in order", func(t *testing.T) {
		reset(t)

		rows := getSubsystemHealth(t, router)
		require.Len(t, rows, 6)

		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Subsystem
			assert.Equal(t, "unknown", r.Status)
			assert.Zero(t, r.ConsecutiveFailures)
			assert.Nil(t, r.LastAttemptAt)
		}
		assert.Equal(t, []string{
			"contact_sync", "calendar_sync", "notification", "backup", "scheduler", "webhook",
		}, names)
	})

	t.Run("a failing subsystem reports the count and first-failure time, then recovers", func(t *testing.T) {
		reset(t)
		now := time.Now().UTC()

		for i := 0; i < 9; i++ {
			require.NoError(t, db.Create(&models.SystemEvent{
				Component:  "contact_sync",
				EventType:  models.SysEventSyncFailed,
				OccurredAt: now.Add(time.Duration(-9+i) * time.Hour),
				Error:      "carddav auth rejected",
			}).Error)
		}

		rows := getSubsystemHealth(t, router)
		cs := findRow(rows, "contact_sync")
		assert.Equal(t, "failing", cs.Status)
		assert.Equal(t, 9, cs.ConsecutiveFailures)
		assert.Equal(t, "carddav auth rejected", cs.LastError)
		require.NotNil(t, cs.IncidentFirstFailureAt)
		assert.WithinDuration(t, now.Add(-9*time.Hour), *cs.IncidentFirstFailureAt, time.Second)

		require.NoError(t, db.Create(&models.SystemEvent{
			Component:  "contact_sync",
			EventType:  models.SysEventSyncCompleted,
			OccurredAt: now,
		}).Error)

		cs = findRow(getSubsystemHealth(t, router), "contact_sync")
		assert.Equal(t, "healthy", cs.Status)
		assert.Zero(t, cs.ConsecutiveFailures)
		assert.Nil(t, cs.IncidentFirstFailureAt)
		assert.Empty(t, cs.LastError)
	})
}

func findRow(rows []subsystemHealthRow, subsystem string) subsystemHealthRow {
	for _, r := range rows {
		if r.Subsystem == subsystem {
			return r
		}
	}
	return subsystemHealthRow{}
}
