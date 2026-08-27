package services

import (
	"context"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// recordSyncEvent emits one sync_completed / sync_failed operational event
// (issue #424) for a finished subscription sync, plus the matching
// standardized log line (issue #425). `detail` carries only bounded counts —
// never the subscription URL or contact IDs (#424 non-goal); the error string
// is sanitized and length-capped by RecordSystemEvent.
func recordSyncEvent(ctx context.Context, db *gorm.DB, component string, userID uint, start time.Time, err error, detail string) {
	durMS := time.Since(start).Milliseconds()

	ev := models.SystemEvent{
		Component:  component,
		DurationMS: &durMS,
		Detail:     detail,
	}
	if userID != 0 {
		uid := userID
		ev.UserID = &uid
	}
	if err != nil {
		ev.EventType = models.SysEventSyncFailed
		ev.Result = models.SysResult(logger.ResultFailure)
		ev.Error = err.Error()
	} else {
		ev.EventType = models.SysEventSyncCompleted
		ev.Result = models.SysResult(logger.ResultSuccess)
	}
	models.RecordSystemEvent(ctx, db, ev)

	l := logger.Ctx(ctx).With().
		Str(logger.FieldComponent, component).
		Int64(logger.FieldDurationMS, durMS).
		Logger()
	if err != nil {
		l.Warn().
			Str(logger.FieldEvent, models.SysEventSyncFailed).
			Str(logger.FieldResult, logger.ResultFailure).
			Err(err).
			Msg("subscription sync failed")
	} else {
		l.Info().
			Str(logger.FieldEvent, models.SysEventSyncCompleted).
			Str(logger.FieldResult, logger.ResultSuccess).
			Str("counts", detail).
			Msg("subscription sync completed")
	}
}
