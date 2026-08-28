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
)

type notificationChannelHealthRow struct {
	Channel             string  `json:"channel"`
	Status              string  `json:"status"`
	Configured          bool    `json:"configured"`
	Reachable           bool    `json:"reachable"`
	EnabledUserCount    int64   `json:"enabled_user_count"`
	DeviceCount         int64   `json:"device_count"`
	FCMConfigured       bool    `json:"fcm_configured"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	LastError           string  `json:"last_error"`
	AttemptedCount      int64   `json:"attempted_count"`
	DeliveredCount      int64   `json:"delivered_count"`
	LastSentAt          *string `json:"last_sent_at"`
	LastFailedAt        *string `json:"last_failed_at"`
}

func getNotificationHealth(t *testing.T, router *gin.Engine) []notificationChannelHealthRow {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/notification-health", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Channels []notificationChannelHealthRow `json:"channels"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Channels
}

// TestGetNotificationChannelHealth exercises GET /admin/notification-health
// (issue #422): a channel that has been failing reports so with its reason,
// a recovery returns it to healthy, and a no-device push channel is distinct
// from a failure.
func TestGetNotificationChannelHealth(t *testing.T) {
	db := dbtest.New(t)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", config.Config{UseSMTP: true})
		c.Next()
	})
	router.GET("/admin/notification-health", GetNotificationChannelHealth)

	t.Run("reports every channel in dispatch order", func(t *testing.T) {
		rows := getNotificationHealth(t, router)
		require.Len(t, rows, 4)
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Channel
		}
		assert.Equal(t, []string{"email", "ntfy", "gotify", "push"}, names)
	})

	t.Run("an email server makes email healthy; a failing gotify channel carries its reason", func(t *testing.T) {
		user := models.User{Username: "ctrl-health-user", Email: "ctrl@example.com", Password: "password123", NotifyGotify: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID: user.ID, GotifyURL: "https://gotify.example.com", GotifyTokenEncrypted: "encrypted",
		}).Error)

		contact := models.Contact{UserID: user.ID, Firstname: "Health", Lastname: "Ctrl"}
		require.NoError(t, db.Create(&contact).Error)
		byMail := false
		reminder := models.Reminder{UserID: user.ID, ContactID: &contact.ID, Message: "m", ByMail: &byMail, RemindAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Recurrence: "once"}
		require.NoError(t, db.Create(&reminder).Error)
		require.NoError(t, db.Create(&models.NotificationDelivery{
			ReminderID: reminder.ID, Channel: "gotify", Status: "failed", Error: strPtr("HTTP 401"),
		}).Error)

		rows := getNotificationHealth(t, router)
		var email, gotify *notificationChannelHealthRow
		for i := range rows {
			switch rows[i].Channel {
			case "email":
				email = &rows[i]
			case "gotify":
				gotify = &rows[i]
			}
		}
		require.NotNil(t, email)
		require.NotNil(t, gotify)

		assert.True(t, email.Configured)
		assert.Equal(t, "healthy", email.Status)
		assert.True(t, email.Reachable)

		assert.Equal(t, "failing", gotify.Status)
		assert.False(t, gotify.Reachable)
		assert.Equal(t, "HTTP 401", gotify.LastError)
		assert.Equal(t, 1, gotify.ConsecutiveFailures)
		assert.Equal(t, int64(1), gotify.AttemptedCount)
	})

	t.Run("a push channel with no devices is reported separately from a failure", func(t *testing.T) {
		// VAPID keys present (server provisioned for web push) but no
		// subscription: no_devices, not failing.
		require.NoError(t, db.Create(&models.ServerSetting{Key: "vapid_public_key", Value: "pub"}).Error)
		require.NoError(t, db.Create(&models.ServerSetting{Key: "vapid_private_key", Value: "priv"}).Error)

		rows := getNotificationHealth(t, router)
		var push *notificationChannelHealthRow
		for i := range rows {
			if rows[i].Channel == "push" {
				push = &rows[i]
			}
		}
		require.NotNil(t, push)
		assert.Equal(t, "no_devices", push.Status)
		assert.False(t, push.Reachable)
	})
}
