package models

import "time"

// Alert state values, matching the CHECK constraint in
// 000040_alert_states.up.sql.
const (
	// AlertStateOK — the condition is not currently raised.
	AlertStateOK = "ok"
	// AlertStateAlerting — the condition is currently raised; a notification
	// was dispatched when it entered this state.
	AlertStateAlerting = "alerting"
)

// AlertState holds whether one alert condition (issue #428) is currently
// raised, and since when. One row per ConditionKey, upserted by
// services.transitionAlertState. The scheduled alert evaluator compares each
// condition's freshly-computed state against the row here and dispatches a
// notification ONLY on a transition — so a condition that keeps failing
// produces one alert, not one per evaluation ("no alert storms").
//
// Not user-authored content: hard state, no DeletedAt (CLAUDE.md backend trap
// #7). Explicit gorm column tags because "condition_key"/"last_notified_at" are
// exactly the kind of names GORM's derivation gets subtly wrong (trap #1).
type AlertState struct {
	// ConditionKey is the stable identifier of the alert condition — see the
	// alertConditionKey* constants in services/alerting_conditions.go.
	ConditionKey string `gorm:"column:condition_key;primaryKey" json:"condition_key"`
	// State is AlertStateOK or AlertStateAlerting.
	State string `gorm:"column:state;not null" json:"state"`
	// Since is when the condition entered State.
	Since time.Time `gorm:"column:since;not null" json:"since"`
	// Detail is a short, sanitized human-readable elaboration of the most
	// recent transition (a mismatch list, an error string, "disk usage 92%").
	Detail string `gorm:"column:detail;not null;default:''" json:"detail,omitempty"`
	// FailureCount is the consecutive-failure count captured when the alert
	// was raised, so the recovery notification can say "recovered after N
	// failures". 0 while State is ok.
	FailureCount int `gorm:"column:failure_count;not null;default:0" json:"failure_count"`
	// PendingNotify is true while an alerting condition's raise has not been
	// accepted by any delivery path (issue #973). The evaluator sets it when
	// it persists a raise and clears it once a dispatch is durably enqueued
	// or delivered, re-attempting on every evaluation in between so a raise
	// made while every channel was briefly down is not lost. Always false
	// while State is ok.
	PendingNotify bool `gorm:"column:pending_notify;not null;default:false" json:"pending_notify"`
	// LastNotifiedAt is when the last alert/recovery notification for this
	// condition was dispatched. Nil until the first transition.
	LastNotifiedAt *time.Time `gorm:"column:last_notified_at" json:"last_notified_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"-"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"-"`
}

// TableName pins the table name so it never drifts from the migration.
func (AlertState) TableName() string {
	return "alert_states"
}
