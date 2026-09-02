package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func derivedRebuildRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.POST("/admin/contacts/rebuild-derived", RebuildDerivedColumnsHandler)
	return db, router
}

func TestRebuildDerivedColumnsHandler_RepairsDriftAndRecordsJobRun(t *testing.T) {
	db, router := derivedRebuildRouter(t)
	u := models.User{Username: "drv-ctrl", Password: "password123!A", Email: "drv-ctrl@example.com"}
	require.NoError(t, db.Create(&u).Error)
	c := models.Contact{UserID: u.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&c).Error)

	// A hook-bypassing write leaves the sort key stale.
	require.NoError(t, db.Exec("UPDATE contacts SET sort_name = 'zzz' WHERE id = ?", c.ID).Error)

	req, _ := http.NewRequest("POST", "/admin/contacts/rebuild-derived", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Message         string           `json:"message"`
		ContactsScanned int64            `json:"contacts_scanned"`
		ContactsUpdated int64            `json:"contacts_updated"`
		ColumnUpdates   map[string]int64 `json:"column_updates"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, int64(1), body.ContactsScanned)
	assert.Equal(t, int64(1), body.ContactsUpdated)
	assert.Equal(t, int64(1), body.ColumnUpdates["sort_name"])

	var got models.Contact
	require.NoError(t, db.First(&got, c.ID).Error)
	assert.Equal(t, "lovelace", got.SortName)

	var run models.JobRun
	require.Eventually(t, func() bool {
		return db.Where("job_name = ?", models.JobNameDerivedColumnsRebuild).Order("id desc").First(&run).Error == nil
	}, 2*time.Second, 20*time.Millisecond, "a job_runs row must be recorded for the rebuild")
	assert.Equal(t, models.JobTriggerManual, run.Trigger)
	assert.Equal(t, models.JobRunResultSuccess, run.Result)
	require.NotNil(t, run.ItemsProcessed)
	assert.Equal(t, 1, *run.ItemsProcessed)
}

func TestRebuildDerivedColumnsHandler_NoOpOnCleanDB(t *testing.T) {
	db, router := derivedRebuildRouter(t)
	u := models.User{Username: "drv-ctrl2", Password: "password123!A", Email: "drv-ctrl2@example.com"}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Ada"}).Error)

	req, _ := http.NewRequest("POST", "/admin/contacts/rebuild-derived", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		ContactsUpdated int64 `json:"contacts_updated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, int64(0), body.ContactsUpdated, "a faithful DB is a fixpoint")
}
