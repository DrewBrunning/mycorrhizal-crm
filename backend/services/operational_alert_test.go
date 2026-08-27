package services

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAlertUser(t *testing.T, db *gorm.DB, name string, isAdmin bool, ntfyURL string) models.User {
	t.Helper()
	user := models.User{
		Username:   name,
		Password:   "password123",
		Email:      name + "@example.com",
		Language:   "en",
		IsAdmin:    isAdmin,
		NotifyNtfy: ntfyURL != "",
	}
	require.NoError(t, db.Create(&user).Error)
	if ntfyURL != "" {
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID:    user.ID,
			NtfyURL:   ntfyURL,
			NtfyTopic: "ops",
		}).Error)
	}
	return user
}

// TestDeliverOperationalAlert_AdminOnly pins that personal-channel alerts reach
// admin users only — infra health is an operator concern, not something to page
// every user of a shared instance about (issue #428).
func TestDeliverOperationalAlert_AdminOnly(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "op-alert.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	srv := newFakeChannelServer(t, nil)

	newAlertUser(t, db, "ops-admin", true, srv.URL())
	newAlertUser(t, db, "ops-regular", false, srv.URL())

	cfg := config.Config{} // no mail, no webhook private-URL block

	deliverOperationalAlert(context.Background(), db, cfg, operationalAlert{
		conditionKey: alertConditionKeyBackup,
		title:        "Backup",
		firing:       true,
		detail:       "snapshot failed",
		failureCount: 2,
		since:        time.Now(),
	})

	require.Equal(t, 1, srv.count(), "only the admin's ntfy endpoint should have been hit")
	assert.Contains(t, srv.lastBody(), "Backup failed")
}

// TestOperationalAlertSubject covers the raise/recover subject phrasing the
// issue calls out ("🟢 Backup recovered after 3 failures" must follow
// "🔴 Backup failed").
func TestOperationalAlertSubject(t *testing.T) {
	raised := operationalAlert{title: "Backup", firing: true, failureCount: 3}
	assert.Equal(t, "🔴 Backup failed", raised.subject())

	recovered := operationalAlert{title: "Backup", firing: false, failureCount: 3}
	assert.Equal(t, "🟢 Backup recovered after 3 failures", recovered.subject())

	recoveredOne := operationalAlert{title: "Disk space", firing: false, failureCount: 1}
	assert.Equal(t, "🟢 Disk space recovered after 1 failure", recoveredOne.subject())

	recoveredZero := operationalAlert{title: "Disk space", firing: false, failureCount: 0}
	assert.Equal(t, "🟢 Disk space recovered", recoveredZero.subject())
}

func TestOperationalAlertHTML_Escapes(t *testing.T) {
	got := operationalAlertHTML("a <script> & \"quote\"")
	assert.NotContains(t, got, "<script>")
	assert.True(t, strings.HasPrefix(got, "<pre"))
}
