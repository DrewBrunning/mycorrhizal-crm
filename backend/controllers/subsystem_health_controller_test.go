package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubsystemHealthRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "subsystem-health-ctrl.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/admin/subsystem-health", GetSubsystemHealth)
	return db, router
}

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

func TestGetSubsystemHealth_FreshDB(t *testing.T) {
	_, router := setupSubsystemHealthRouter(t)

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
}

func TestGetSubsystemHealth_FailingThenRecovered(t *testing.T) {
	db, router := setupSubsystemHealthRouter(t)
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
	var cs subsystemHealthRow
	for _, r := range rows {
		if r.Subsystem == "contact_sync" {
			cs = r
		}
	}
	assert.Equal(t, "failing", cs.Status)
	assert.Equal(t, 9, cs.ConsecutiveFailures)
	assert.Equal(t, "carddav auth rejected", cs.LastError)
	require.NotNil(t, cs.IncidentFirstFailureAt)
	assert.WithinDuration(t, now.Add(-9*time.Hour), *cs.IncidentFirstFailureAt, time.Second)

	// One success closes the incident.
	require.NoError(t, db.Create(&models.SystemEvent{
		Component:  "contact_sync",
		EventType:  models.SysEventSyncCompleted,
		OccurredAt: now,
	}).Error)

	rows = getSubsystemHealth(t, router)
	for _, r := range rows {
		if r.Subsystem == "contact_sync" {
			cs = r
		}
	}
	assert.Equal(t, "healthy", cs.Status)
	assert.Zero(t, cs.ConsecutiveFailures)
	assert.Nil(t, cs.IncidentFirstFailureAt)
	assert.Empty(t, cs.LastError)
}
