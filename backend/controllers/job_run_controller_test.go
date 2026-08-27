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
	"gorm.io/gorm"
)

func newJobRunControllerRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/admin/job-runs", ListJobRuns)
	router.GET("/admin/job-runs/health", GetJobRunHealth)
	return router, db
}

func seedControllerRun(t *testing.T, db *gorm.DB, job string, at time.Time, result string) {
	t.Helper()
	require.NoError(t, db.Create(&models.JobRun{
		JobName:    job,
		Trigger:    models.JobTriggerScheduled,
		StartedAt:  at,
		FinishedAt: at.Add(time.Second),
		DurationMS: 1000,
		Result:     result,
		CreatedAt:  at,
	}).Error)
}

func TestListJobRuns_FiltersAndShape(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	base := time.Now().Add(-time.Hour).UTC()
	seedControllerRun(t, db, models.JobNameDailyReminders, base, models.JobRunResultSuccess)
	seedControllerRun(t, db, models.JobNameDailyReminders, base.Add(time.Minute), models.JobRunResultFailure)
	seedControllerRun(t, db, models.JobNameCalendarSync, base.Add(2*time.Minute), models.JobRunResultSuccess)

	var all struct {
		JobRuns []models.JobRun `json:"job_runs"`
		Total   int             `json:"total"`
	}
	doGET(t, router, "/admin/job-runs", &all)
	assert.Equal(t, 3, all.Total)
	assert.Len(t, all.JobRuns, 3)
	assert.Equal(t, models.JobNameCalendarSync, all.JobRuns[0].JobName, "newest first")

	var filtered struct {
		JobRuns []models.JobRun `json:"job_runs"`
		Total   int             `json:"total"`
	}
	doGET(t, router, "/admin/job-runs?job_name="+models.JobNameDailyReminders+"&result=failure", &filtered)
	require.Equal(t, 1, filtered.Total)
	assert.Equal(t, models.JobRunResultFailure, filtered.JobRuns[0].Result)
}

func TestListJobRuns_RejectsBadTimestamp(t *testing.T) {
	router, _ := newJobRunControllerRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/job-runs?since=not-a-time", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetJobRunHealth_ShapeAndFold(t *testing.T) {
	router, db := newJobRunControllerRouter(t)
	base := time.Now().Add(-time.Hour).UTC()
	seedControllerRun(t, db, models.JobNameDailyReminders, base, models.JobRunResultFailure)
	seedControllerRun(t, db, models.JobNameDailyReminders, base.Add(time.Minute), models.JobRunResultFailure)

	var resp struct {
		Jobs []struct {
			JobName             string `json:"job_name"`
			Status              string `json:"status"`
			ConsecutiveFailures int    `json:"consecutive_failures"`
		} `json:"jobs"`
	}
	doGET(t, router, "/admin/job-runs/health", &resp)
	require.Len(t, resp.Jobs, len(models.KnownJobNames))
	assert.Equal(t, models.KnownJobNames[0], resp.Jobs[0].JobName, "deterministic order preserved")

	var reminders struct {
		Status              string
		ConsecutiveFailures int
	}
	for _, j := range resp.Jobs {
		if j.JobName == models.JobNameDailyReminders {
			reminders.Status = j.Status
			reminders.ConsecutiveFailures = j.ConsecutiveFailures
		}
	}
	assert.Equal(t, "failing", reminders.Status)
	assert.Equal(t, 2, reminders.ConsecutiveFailures)
}

func doGET(t *testing.T, router *gin.Engine, path string, out interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), out))
}
