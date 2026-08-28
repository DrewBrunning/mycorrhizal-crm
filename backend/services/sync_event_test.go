package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordSyncEvent_Success(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	ctx := logger.WithCorrelationID(context.Background(), "corr-sync-1")
	recordSyncEvent(ctx, db, logger.ComponentContactSync, 7, time.Now().Add(-2*time.Second), nil, "created=1 updated=2")

	var ev models.SystemEvent
	require.NoError(t, db.Order("id desc").First(&ev).Error)
	assert.Equal(t, models.SysEventSyncCompleted, ev.EventType)
	assert.Equal(t, logger.ComponentContactSync, ev.Component)
	assert.Equal(t, "corr-sync-1", ev.CorrelationID)
	assert.Equal(t, logger.SeverityInfo, ev.Severity)
	require.NotNil(t, ev.Result)
	assert.Equal(t, logger.ResultSuccess, *ev.Result)
	require.NotNil(t, ev.DurationMS)
	assert.GreaterOrEqual(t, *ev.DurationMS, int64(1000))
	require.NotNil(t, ev.UserID)
	assert.Equal(t, uint(7), *ev.UserID)
	assert.Equal(t, "created=1 updated=2", ev.Detail)
}

func TestRecordSyncEvent_Failure(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	recordSyncEvent(context.Background(), db, logger.ComponentCalendarSync, 0, time.Now(), errors.New("dav 401"), "")

	var ev models.SystemEvent
	require.NoError(t, db.Order("id desc").First(&ev).Error)
	assert.Equal(t, models.SysEventSyncFailed, ev.EventType)
	assert.Equal(t, logger.SeverityError, ev.Severity)
	require.NotNil(t, ev.Result)
	assert.Equal(t, logger.ResultFailure, *ev.Result)
	assert.Equal(t, "dav 401", ev.Error)
	assert.Nil(t, ev.UserID, "userID 0 must not persist a bogus user attribution")
}
