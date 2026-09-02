package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// INT-02 (issue #465), actions 2/4/5/6: for the scheduled/interactive syncs
// (CardDAV contacts, CalDAV calendar), a request failure must produce a
// *defined* outcome — the run records failure, the per-subscription mutex is
// released, local data is untouched, and the failure is observable through the
// sync-health fields and a sync_failed operational event. The quiet-failure
// case (#464's silent staleness) is impossible by construction.

func syncFailureCfg() config.Config {
	return config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
}

func syncFailureUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Username: "syncfail", Password: "password123!A", Email: "syncfail@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func countSyncFailedEvents(t *testing.T, db *gorm.DB, component string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.SystemEvent{}).
		Where("component = ? AND event_type = ?", component, models.SysEventSyncFailed).
		Count(&n).Error)
	return n
}

// TestContactSync_RequestFailureIsDefinedAndObservable arms the CardDAV request
// seam and asserts the whole post-run contract holds.
func TestContactSync_RequestFailureIsDefinedAndObservable(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := dbtest.New(t)
	cfg := syncFailureCfg()
	user := syncFailureUser(t, db)

	// Two existing contacts that a broken pull must never touch.
	for _, name := range []string{"Ada", "Grace"} {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: name}).Error)
	}
	var before int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&before).Error)

	enc, err := EncryptCredential(cfg.JWTSecretKey, "secret")
	require.NoError(t, err)
	sub := &models.ContactSubscription{
		UserID: user.ID, Name: "book", URL: "https://dav.example.com/addressbooks/x/",
		Username: "u", PasswordEncrypted: enc, SyncEnabled: true,
	}
	require.NoError(t, db.Create(sub).Error)

	faults.ArmError(faultContactSyncRequest, errors.New("carddav upstream 503"))
	t.Cleanup(func() { faults.Disarm(faultContactSyncRequest) })

	_, syncErr := NewContactSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)

	// 1. The run fails — not a silent success.
	require.Error(t, syncErr)

	// 2. Bookkeeping records the failure.
	var reloaded models.ContactSubscription
	require.NoError(t, db.First(&reloaded, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusError, reloaded.LastSyncStatus)
	assert.NotEmpty(t, reloaded.LastSyncError)

	// 3. Sync-health advances (issue #390): this is what makes a stopped sync
	//    visible instead of looking like "nothing changed".
	assert.Equal(t, 1, reloaded.ConsecutiveFailures)
	assert.NotNil(t, reloaded.LastFailureAt)
	assert.NotNil(t, reloaded.IncidentFirstFailureAt)

	// 4. A sync_failed operational event was emitted (issue #424).
	assert.Equal(t, int64(1), countSyncFailedEvents(t, db, logger.ComponentContactSync))

	// 5. Local data is untouched — no contact created, deleted, or archived.
	var after int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&after).Error)
	assert.Equal(t, before, after, "a failed pull must not change the local contact set")

	// 6. The subscription mutex is released — a second run proceeds (and fails
	//    the same way) rather than deadlocking.
	_, secondErr := NewContactSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, secondErr)
	require.NoError(t, db.First(&reloaded, sub.ID).Error)
	assert.Equal(t, 2, reloaded.ConsecutiveFailures, "the second failed run must also be counted")
}

// TestCalendarSync_RequestFailureIsDefinedAndObservable is the CalDAV mirror.
func TestCalendarSync_RequestFailureIsDefinedAndObservable(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := dbtest.New(t)
	cfg := syncFailureCfg()
	user := syncFailureUser(t, db)

	enc, err := EncryptCredential(cfg.JWTSecretKey, "secret")
	require.NoError(t, err)
	sub := &models.CalendarSubscription{
		UserID: user.ID, Name: "cal", URL: "https://dav.example.com/calendars/x/",
		Username: "u", PasswordEncrypted: enc, SyncEnabled: true,
	}
	require.NoError(t, db.Create(sub).Error)

	// An existing imported activity + its link that a broken pull must keep.
	act := models.Activity{UserID: user.ID, Title: "Standup", Date: time.Now()}
	require.NoError(t, db.Create(&act).Error)
	require.NoError(t, db.Create(&models.CalendarEventLink{
		SubscriptionID: sub.ID, UserID: user.ID, ActivityID: act.ID, UID: "evt-1", ContentHash: "h1",
	}).Error)
	var linksBefore int64
	require.NoError(t, db.Model(&models.CalendarEventLink{}).Where("user_id = ?", user.ID).Count(&linksBefore).Error)

	faults.ArmError(faultCalendarSyncRequest, errors.New("caldav upstream 503"))
	t.Cleanup(func() { faults.Disarm(faultCalendarSyncRequest) })

	_, syncErr := NewCalendarSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, syncErr)

	var reloaded models.CalendarSubscription
	require.NoError(t, db.First(&reloaded, sub.ID).Error)
	assert.Equal(t, models.CalendarSyncStatusError, reloaded.LastSyncStatus)
	assert.Equal(t, 1, reloaded.ConsecutiveFailures)
	assert.NotNil(t, reloaded.IncidentFirstFailureAt)
	assert.Equal(t, int64(1), countSyncFailedEvents(t, db, logger.ComponentCalendarSync))

	var actAfter, linksAfter int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&actAfter).Error)
	require.NoError(t, db.Model(&models.CalendarEventLink{}).Where("user_id = ?", user.ID).Count(&linksAfter).Error)
	assert.Equal(t, int64(1), actAfter, "a failed pull must not delete imported activities")
	assert.Equal(t, linksBefore, linksAfter, "a failed pull must not drop event links")
}

// TestSync_HungRemoteIsBoundedByContext pins issue #465 action 3: a caller that
// imposes a deadline (as the scheduled job does) is not held past it by a hung
// remote, and the per-subscription mutex is released so the next run can start.
func TestSync_HungRemoteIsBoundedByContext(t *testing.T) {
	db := dbtest.New(t)
	cfg := syncFailureCfg()
	user := syncFailureUser(t, db)

	// A host that accepts the socket and never answers — a genuinely hung
	// remote, not an injected error.
	blackHole := newBlackHoleServer(t)

	enc, err := EncryptCredential(cfg.JWTSecretKey, "secret")
	require.NoError(t, err)
	sub := &models.ContactSubscription{
		UserID: user.ID, Name: "book", URL: blackHole + "/addressbooks/x/",
		Username: "u", PasswordEncrypted: enc, SyncEnabled: true,
	}
	require.NoError(t, db.Create(sub).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, e := NewContactSyncService(false).SyncSubscription(ctx, db, cfg, sub)
		done <- e
	}()

	select {
	case err := <-done:
		require.Error(t, err, "a hung remote must surface as an error once the deadline passes")
		assert.Less(t, time.Since(start), 10*time.Second, "the sync must not outlive its context by much")
	case <-time.After(15 * time.Second):
		t.Fatal("SyncSubscription ignored its context deadline against a hung remote")
	}
}
