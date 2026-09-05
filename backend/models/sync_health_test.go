package models

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvanceForRunAccruesConsecutiveFailures(t *testing.T) {
	t.Parallel()
	var h SyncHealthFields
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	boom := errors.New("boom")

	// First failure opens an incident.
	h.AdvanceForRun(base, 200*time.Millisecond, `{"created":0}`, boom, "")
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.Equal(t, base, *h.IncidentFirstFailureAt)
	assert.Equal(t, 1, h.ConsecutiveFailures)
	assert.Equal(t, base, *h.LastFailureAt)
	assert.Equal(t, base, *h.LastAttemptAt)
	assert.Nil(t, h.LastSuccessAt)
	require.NotNil(t, h.LastRunDurationMS)
	assert.Equal(t, int64(200), *h.LastRunDurationMS)

	// Two more failures: count climbs, incident start is pinned.
	h.AdvanceForRun(base.Add(1*time.Hour), time.Second, `{}`, boom, "")
	h.AdvanceForRun(base.Add(2*time.Hour), time.Second, `{}`, boom, "")
	assert.Equal(t, 3, h.ConsecutiveFailures)
	assert.Equal(t, base, *h.IncidentFirstFailureAt, "incident start must not move mid-incident")
	assert.Equal(t, base.Add(2*time.Hour), *h.LastFailureAt)

	// A success closes the incident and resets the counter.
	ok := base.Add(3 * time.Hour)
	h.AdvanceForRun(ok, 50*time.Millisecond, `{"created":2}`, nil, "")
	assert.Zero(t, h.ConsecutiveFailures)
	assert.Nil(t, h.IncidentFirstFailureAt)
	require.NotNil(t, h.LastSuccessAt)
	assert.Equal(t, ok, *h.LastSuccessAt)
	assert.Equal(t, ok, *h.LastAttemptAt)
	assert.Equal(t, base.Add(2*time.Hour), *h.LastFailureAt, "last_failure_at is history, not cleared by a later success")
	assert.Equal(t, `{"created":2}`, h.LastRunStats)

	// A fresh failure after recovery opens a new incident at its own time.
	next := base.Add(4 * time.Hour)
	h.AdvanceForRun(next, time.Second, `{}`, boom, "")
	assert.Equal(t, 1, h.ConsecutiveFailures)
	require.NotNil(t, h.IncidentFirstFailureAt)
	assert.Equal(t, next, *h.IncidentFirstFailureAt)
}

func TestAdvanceForRunReturnsPersistableUpdates(t *testing.T) {
	t.Parallel()
	var h SyncHealthFields
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	fail := h.AdvanceForRun(now, time.Second, `{}`, errors.New("x"), "")
	assert.Equal(t, 1, fail["consecutive_failures"])
	assert.Equal(t, h.IncidentFirstFailureAt, fail["incident_first_failure_at"])
	assert.NotContains(t, fail, "last_success_at", "a failed run does not touch last_success_at")
	assert.NotContains(t, fail, "terminal_failure_at", "a transient failure does not touch terminal state")

	ok := h.AdvanceForRun(now.Add(time.Minute), time.Second, `{}`, nil, "")
	assert.Equal(t, 0, ok["consecutive_failures"])
	assert.Nil(t, ok["incident_first_failure_at"])
	assert.Equal(t, h.LastSuccessAt, ok["last_success_at"])
	assert.Contains(t, ok, "terminal_failure_at", "a success always clears terminal state")
	assert.Nil(t, ok["terminal_failure_at"])
}

func TestAdvanceForRunTerminalFailureLifecycle(t *testing.T) {
	t.Parallel()
	var h SyncHealthFields
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	boom := errors.New("401 unauthorized")

	// A transient failure never enters the terminal state.
	h.AdvanceForRun(base, time.Second, `{}`, boom, "")
	assert.Nil(t, h.TerminalFailureAt)
	assert.Empty(t, h.TerminalReason)

	// A permanent failure enters it, recording the reason and the entry time.
	up := h.AdvanceForRun(base.Add(time.Hour), time.Second, `{}`, boom, "auth-expiry")
	require.NotNil(t, h.TerminalFailureAt)
	assert.Equal(t, base.Add(time.Hour), *h.TerminalFailureAt)
	assert.Equal(t, "auth-expiry", h.TerminalReason)
	assert.Equal(t, h.TerminalFailureAt, up["terminal_failure_at"])
	assert.Equal(t, "auth-expiry", up["terminal_reason"])

	// A second permanent run does not move the entry time ("when did it stop
	// working" must be stable) but may refine the reason.
	h.AdvanceForRun(base.Add(2*time.Hour), time.Second, `{}`, boom, "authz-revoked")
	assert.Equal(t, base.Add(time.Hour), *h.TerminalFailureAt, "terminal entry time must not move")
	assert.Equal(t, "authz-revoked", h.TerminalReason)

	// Any success clears it — that is the recovery signal.
	h.AdvanceForRun(base.Add(3*time.Hour), time.Second, `{}`, nil, "")
	assert.Nil(t, h.TerminalFailureAt)
	assert.Empty(t, h.TerminalReason)
}

func TestClearTerminalStateLiftsWithoutRecordingARun(t *testing.T) {
	t.Parallel()
	h := SyncHealthFields{
		ConsecutiveFailures: 4,
		TerminalReason:      "auth-expiry",
	}
	now := time.Now()
	h.TerminalFailureAt = &now

	updates := h.ClearTerminalState()
	assert.Nil(t, h.TerminalFailureAt)
	assert.Empty(t, h.TerminalReason)
	assert.Nil(t, updates["terminal_failure_at"])
	assert.Equal(t, "", updates["terminal_reason"])
	// It does not touch the failure counters — only a real run does that.
	assert.Equal(t, 4, h.ConsecutiveFailures)
	assert.NotContains(t, updates, "consecutive_failures")
}
