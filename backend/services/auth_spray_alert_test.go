package services

import (
	"context"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// injectAuthVelocity swaps the auth_spray condition's signal source for the
// duration of a test, so the alert logic can be driven without touching the
// process-wide rate limiter.
func injectAuthVelocity(t *testing.T, snap *middleware.AuthVelocitySnapshot) {
	t.Helper()
	orig := authVelocitySnapshot
	authVelocitySnapshot = func() middleware.AuthVelocitySnapshot { return *snap }
	t.Cleanup(func() { authVelocitySnapshot = orig })
}

func authSprayTestConfig() config.Config {
	return config.Config{AuthSprayEnabled: true, AlertAuthSprayEnabled: true}
}

// TestAuthSprayCondition covers the condition's verdict for a clean signal, a
// tripped incident, a throttled incident and a disabled feature.
func TestAuthSprayCondition(t *testing.T) {
	t.Run("clean signal does not fire", func(t *testing.T) {
		snap := middleware.AuthVelocitySnapshot{Enabled: true}
		injectAuthVelocity(t, &snap)

		r := authSprayCondition(authSprayTestConfig())
		assert.Equal(t, alertConditionKeyAuthSpray, r.key)
		assert.False(t, r.firing)
	})

	t.Run("incident fires with counts and no usernames", func(t *testing.T) {
		snap := middleware.AuthVelocitySnapshot{
			Enabled: true, Incident: true,
			Failures: 120, Identifiers: 40, SourceIPs: 37, WindowSeconds: 60,
		}
		injectAuthVelocity(t, &snap)

		r := authSprayCondition(authSprayTestConfig())
		require.True(t, r.firing)
		assert.Equal(t, 120, r.failureCount)
		assert.Contains(t, r.detail, "120 failed logins")
		assert.Contains(t, r.detail, "40 identifiers")
		assert.Contains(t, r.detail, "37 source IPs")
		assert.Contains(t, r.detail, "last 60s")
	})

	t.Run("throttled incident reports the active throttle", func(t *testing.T) {
		snap := middleware.AuthVelocitySnapshot{
			Enabled: true, Incident: true, Throttled: true,
			Failures: 80, Identifiers: 20, SourceIPs: 20, WindowSeconds: 60, RetryAfterSecs: 240,
		}
		injectAuthVelocity(t, &snap)

		r := authSprayCondition(authSprayTestConfig())
		require.True(t, r.firing)
		assert.Contains(t, r.detail, "throttle active for another 240s")
	})

	t.Run("targeted identifiers are counted but never named", func(t *testing.T) {
		snap := middleware.AuthVelocitySnapshot{
			Enabled: true, Incident: true,
			Failures: 50, Identifiers: 12, SourceIPs: 30, WindowSeconds: 60,
			TargetedIdentifiers: []string{"victim@example.com"},
		}
		injectAuthVelocity(t, &snap)

		r := authSprayCondition(authSprayTestConfig())
		require.True(t, r.firing)
		assert.Contains(t, r.detail, "1 identifier(s) failed from many source IPs")
		assert.NotContains(t, r.detail, "victim@example.com", "alert payloads must not leak identifiers")
	})

	t.Run("disabled feature does not fire even while the signal is hot", func(t *testing.T) {
		snap := middleware.AuthVelocitySnapshot{Enabled: true, Incident: true, Failures: 500}
		injectAuthVelocity(t, &snap)

		cfg := authSprayTestConfig()
		cfg.AuthSprayEnabled = false
		assert.False(t, authSprayCondition(cfg).firing)

		cfg = authSprayTestConfig()
		cfg.AlertAuthSprayEnabled = false
		assert.False(t, authSprayCondition(cfg).firing)
	})
}

// TestEvaluateAlertConditionsWiresAuthSpray guards the call site: the condition
// must be produced by the evaluator when enabled.
func TestEvaluateAlertConditionsWiresAuthSpray(t *testing.T) {
	db := dbtest.New(t)
	cfg := alertTestConfig(filepath.Join(t.TempDir(), "x.db"))
	cfg.AlertDiskUsagePercent = 0
	cfg.AuthSprayEnabled = true
	cfg.AlertAuthSprayEnabled = true

	snap := middleware.AuthVelocitySnapshot{Enabled: true, Incident: true, Failures: 99, Identifiers: 33, SourceIPs: 33, WindowSeconds: 60}
	injectAuthVelocity(t, &snap)

	results := evaluateAlertConditions(context.Background(), db, cfg, nil, nil)
	var found bool
	for _, r := range results {
		if r.key == alertConditionKeyAuthSpray {
			found = true
			assert.True(t, r.firing)
		}
	}
	assert.True(t, found, "the evaluator must emit the auth_spray condition")

	// Disabling the condition removes it from the evaluation entirely.
	cfg.AlertAuthSprayEnabled = false
	for _, r := range evaluateAlertConditions(context.Background(), db, cfg, nil, nil) {
		assert.NotEqual(t, alertConditionKeyAuthSpray, r.key)
	}
}

// TestEvaluateAlerts_AuthSprayRaiseAndRecover drives the full transition over a
// real migrated DB: a tripped signal raises exactly once, repeats are silent,
// and clearing dispatches the recovery — the "alert/webhook on spike" the issue
// asks for.
func TestEvaluateAlerts_AuthSprayRaiseAndRecover(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "authspray.db")
	db := dbtest.NewAt(t, dbPath)

	rec := &recordingDeliverer{deliver: true}
	origDeliverer := alertDeliverer
	alertDeliverer = rec.fn
	t.Cleanup(func() { alertDeliverer = origDeliverer })

	cfg := alertTestConfig(dbPath)
	cfg.AlertDiskUsagePercent = 0
	cfg.AuthSprayEnabled = true
	cfg.AlertAuthSprayEnabled = true

	snap := middleware.AuthVelocitySnapshot{Enabled: true}
	origSnapshot := authVelocitySnapshot
	authVelocitySnapshot = func() middleware.AuthVelocitySnapshot { return snap }
	t.Cleanup(func() { authVelocitySnapshot = origSnapshot })

	ctx := context.Background()

	// Clean baseline: no dispatch.
	RunAlertEvaluation(ctx, db, cfg)
	require.Empty(t, rec.alerts, "a clean baseline must not dispatch anything")

	// Spike: one raise, carrying the counts.
	snap = middleware.AuthVelocitySnapshot{
		Enabled: true, Incident: true, Throttled: true,
		Failures: 90, Identifiers: 25, SourceIPs: 25, WindowSeconds: 60, RetryAfterSecs: 120,
	}
	RunAlertEvaluation(ctx, db, cfg)
	require.Len(t, rec.alerts, 1)
	assert.Equal(t, alertConditionKeyAuthSpray, rec.alerts[0].conditionKey)
	assert.True(t, rec.alerts[0].firing)
	assert.Contains(t, rec.alerts[0].subject(), "Authentication abuse failed")

	// Still spiking: no storm.
	RunAlertEvaluation(ctx, db, cfg)
	require.Len(t, rec.alerts, 1, "an ongoing incident must not re-alert")

	// Quiet: recovery.
	snap = middleware.AuthVelocitySnapshot{Enabled: true}
	RunAlertEvaluation(ctx, db, cfg)
	require.Len(t, rec.alerts, 2)
	assert.False(t, rec.alerts[1].firing)
	assert.Contains(t, rec.alerts[1].subject(), "recovered")
}
