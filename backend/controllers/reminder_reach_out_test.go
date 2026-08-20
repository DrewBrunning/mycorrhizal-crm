package controllers

import (
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompleteReminder_DismissesLinkedReachOutSuggestion covers issue #177's
// accepted lifecycle coupling: completing (or skipping) the one-off Reminder
// auto-created for a ReachOutSuggestion also dismisses that suggestion, so it
// doesn't linger on the dashboard after the user already acted via the
// reminder. Real migrated schema (CLAUDE.md backend trap 1).
func TestCompleteReminder_DismissesLinkedReachOutSuggestion(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "reminder-reach-out.db"))
	require.NoError(t, err)

	user := models.User{Username: "rrouser", Password: "password123!A", Email: "rrouser@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Dana"}
	require.NoError(t, db.Create(&contact).Error)

	reminder := models.Reminder{
		UserID: user.ID, Message: "Reach out — organization changed: Dana (OldCo → NewCo)",
		RemindAt: time.Now(), Recurrence: "once", ContactID: &contact.ID,
	}
	require.NoError(t, db.Create(&reminder).Error)

	suggestion := models.ReachOutSuggestion{
		UserID: user.ID, ContactVCardUID: contact.VCardUID, Kind: models.ReachOutKindOrganization,
		OldValue: "OldCo", NewValue: "NewCo", AuditEventID: 1, ReminderID: &reminder.ID,
		Status: models.ReachOutStatusPending,
	}
	require.NoError(t, db.Create(&suggestion).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/reminders/:id/complete", CompleteReminder)

	req, _ := http.NewRequest("POST", "/reminders/"+strconv.Itoa(int(reminder.ID))+"/complete", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.ReachOutSuggestion
	require.NoError(t, db.First(&reloaded, "id = ?", suggestion.ID).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, reloaded.Status, "completing the companion reminder must dismiss its ReachOutSuggestion")
}
