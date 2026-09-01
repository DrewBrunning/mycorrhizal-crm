package services

import (
	"context"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Alerting on state transitions (issue #428).
//
// Two scheduled jobs already fire one-off failure webhooks
// (db.integrity_check_failed, db.restore_drill_failed) on every failed run,
// with no recovery counterpart and no de-duplication. This replaces that with
// a single evaluator that watches the per-subsystem last-known-good state
// (issue #427) plus a few threshold checks, and dispatches exactly one
// notification per *transition* — failure and recovery — through the existing
// webhook + notification channels.
//
// Nothing here builds a new alerting pipeline: subsystem state comes from
// ComputeSubsystemHealth, delivery from triggerWebhooksForAllUsers and the
// notification_service senders. The only new persistence is alert_states, one
// row per condition, holding the current state so a transition can be detected.

// alertEvalMinInterval is the de-dup window for this configurable-cadence job:
// the shared JobCatchupWindow of the configured period (issue #526, ADR 0011).
func alertEvalMinInterval(cfg config.Config) time.Duration {
	return JobCatchupWindow(time.Duration(cfg.AlertEvalIntervalMinutes) * time.Minute)
}

// EvaluateAlerts is the scheduled entry point. Job-lock guarded like every
// other scheduled job in this package; config-gated by ALERTING_ENABLED.
func EvaluateAlerts(db *gorm.DB, cfg config.Config) {
	if !cfg.AlertingEnabled {
		return
	}

	ctx := logger.JobContext(models.JobNameAlertEval)

	acquired, err := acquireJobLock(db, models.JobNameAlertEval, alertEvalMinInterval(cfg))
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("alerting: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameAlertEval, true); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("alerting: failed to release job lock")
		}
	}()

	RunAlertEvaluation(ctx, db, cfg)
}

// RunAlertEvaluation is the lock-free core: compute subsystem health, evaluate
// every enabled condition, and transition each one. Split out so tests can
// drive it directly without the job lock.
func RunAlertEvaluation(ctx context.Context, db *gorm.DB, cfg config.Config) {
	health, err := ComputeSubsystemHealth(ctx, db)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("alerting: failed to compute subsystem health")
		return
	}

	prev, err := loadAlertStates(ctx, db)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("alerting: failed to load alert states")
		return
	}

	for _, res := range evaluateAlertConditions(ctx, db, cfg, health, prev) {
		transitionAlertState(ctx, db, cfg, res, prev[res.key])
	}
}

func loadAlertStates(ctx context.Context, db *gorm.DB) (map[string]models.AlertState, error) {
	var rows []models.AlertState
	if err := db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]models.AlertState, len(rows))
	for _, r := range rows {
		m[r.ConditionKey] = r
	}
	return m, nil
}

// transitionAlertState compares a condition's fresh verdict against its
// persisted alert_states row. On a state change it updates the row and
// dispatches exactly one raise/recover notification; with no change it never
// notifies (the "no alert storms" guarantee).
func transitionAlertState(ctx context.Context, db *gorm.DB, cfg config.Config, res alertConditionResult, prev models.AlertState) {
	now := time.Now().UTC()
	hadRow := prev.ConditionKey != ""

	want := models.AlertStateOK
	if res.firing {
		want = models.AlertStateAlerting
	}

	prevState := models.AlertStateOK
	if hadRow {
		prevState = prev.State
	}

	if prevState == want {
		transitionNoop(ctx, db, res, prev, hadRow, want, now)
		return
	}

	row := models.AlertState{
		ConditionKey:   res.key,
		State:          want,
		Since:          now,
		Detail:         res.detail,
		LastNotifiedAt: &now,
	}
	alert := operationalAlert{
		conditionKey: res.key,
		title:        res.title,
		firing:       res.firing,
		detail:       res.detail,
		since:        now,
	}
	if res.firing {
		row.FailureCount = res.failureCount
		alert.failureCount = res.failureCount
	} else {
		// Recovery reports how long the just-closed incident lasted — the
		// failure count captured when it was raised.
		alert.failureCount = prev.FailureCount
	}

	if err := persistAlertState(ctx, db, row, hadRow); err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("condition", res.key).
			Msg("alerting: failed to persist state transition — not notifying")
		return // don't notify without a recorded transition, or the next run re-alerts
	}

	logger.Ctx(ctx).Warn().
		Str("condition", res.key).
		Str("state", want).
		Str(logger.FieldComponent, logger.ComponentApp).
		Int("failure_count", alert.failureCount).
		Msg("alerting: " + alert.subject())

	dispatchOperationalAlert(ctx, db, cfg, alert)
}

// transitionNoop handles the no-transition path: refresh a stored detail for an
// ongoing incident, or write the clean baseline row for a first-ever
// observation, but never notify.
func transitionNoop(ctx context.Context, db *gorm.DB, res alertConditionResult, prev models.AlertState, hadRow bool, want string, now time.Time) {
	if !hadRow {
		row := models.AlertState{ConditionKey: res.key, State: want, Since: now, Detail: res.detail}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("condition", res.key).
				Msg("alerting: failed to create baseline alert state")
		}
		return
	}
	if res.firing && prev.Detail != res.detail {
		if err := db.WithContext(ctx).Model(&models.AlertState{}).
			Where("condition_key = ?", res.key).
			Updates(map[string]interface{}{"detail": res.detail, "updated_at": now}).Error; err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("condition", res.key).
				Msg("alerting: failed to refresh alert detail")
		}
	}
}

func persistAlertState(ctx context.Context, db *gorm.DB, row models.AlertState, hadRow bool) error {
	if !hadRow {
		return db.WithContext(ctx).Create(&row).Error
	}
	return db.WithContext(ctx).Model(&models.AlertState{}).
		Where("condition_key = ?", row.ConditionKey).
		Updates(map[string]interface{}{
			"state":            row.State,
			"since":            row.Since,
			"detail":           row.Detail,
			"failure_count":    row.FailureCount,
			"last_notified_at": row.LastNotifiedAt,
			"updated_at":       row.Since,
		}).Error
}
