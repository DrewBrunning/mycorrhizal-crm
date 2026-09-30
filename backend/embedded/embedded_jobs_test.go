package embedded

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/require"
)

// TestRegisterScheduledJobs_EmbeddedOmitsDisabledJobs pins the embedded-mode
// job gating (ADR 0028 Decision 2, issue #1258) at the registration layer: a
// server-mode scheduler registers every job, an embedded one skips exactly
// embeddedDisabledJobs.
func TestRegisterScheduledJobs_EmbeddedOmitsDisabledJobs(t *testing.T) {
	serverCfg := testSchedulerConfig()
	server := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(server, nil, &serverCfg))
	serverTags := jobsByTag(server)

	embeddedCfg := testSchedulerConfig()
	embeddedCfg.Deployment = config.DeploymentEmbedded
	embedded := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(embedded, nil, &embeddedCfg))
	embeddedTags := jobsByTag(embedded)

	for job := range embeddedDisabledJobs {
		require.Contains(t, serverTags, job, "server mode must register %s", job)
		require.NotContains(t, embeddedTags, job, "embedded mode must not register %s", job)
	}

	// Core jobs survive the gating.
	for _, job := range []string{
		models.JobNamePurgeDeleted,
		models.JobNameDBIntegrityCheck,
		models.JobNameCadenceOverdue,
		models.JobNameReachOutDetection,
	} {
		require.Contains(t, embeddedTags, job, "embedded mode must keep %s", job)
	}
}

// TestEmbeddedDisabledJobs_AreRealJobNames guards against a typo'd entry in
// embeddedDisabledJobs silently gating nothing: every listed token must be a
// job that server mode actually registers.
func TestEmbeddedDisabledJobs_AreRealJobNames(t *testing.T) {
	cfg := testSchedulerConfig()
	s := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(s, nil, &cfg))
	registered := jobsByTag(s)

	for job, disabled := range embeddedDisabledJobs {
		require.True(t, disabled, "embeddedDisabledJobs[%q] must be true", job)
		require.Contains(t, registered, job,
			"embeddedDisabledJobs lists %q, which server mode does not register — typo?", job)
	}
}

// TestRegisterScheduledJobs_EmbeddedExactSet pins the whole embedded job set
// (ADR 0028 amendment, issue #1367: storage only). A new job must be added
// here deliberately; a job that reaches out to the network must not be.
func TestRegisterScheduledJobs_EmbeddedExactSet(t *testing.T) {
	cfg := testSchedulerConfig()
	cfg.Deployment = config.DeploymentEmbedded
	s := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(s, nil, &cfg))

	got := map[string]bool{}
	for tag := range jobsByTag(s) {
		got[tag] = true
	}
	want := map[string]bool{
		models.JobNamePurgeDeleted:        true,
		models.JobNameAuditPurge:          true,
		models.JobNameSystemEventPurge:    true,
		models.JobNameJobRunPurge:         true,
		models.JobNameIdempotencyKeyPurge: true,
		models.JobNameSessionPurge:        true,
		models.JobNameCadenceOverdue:      true,
		models.JobNameReachOutDetection:   true,
		models.JobNameDBIntegrityCheck:    true,
	}
	require.Equal(t, want, got)
	for _, outbound := range []string{
		models.JobNameCalendarSync, models.JobNameImmichSync, models.JobNameDailyReminders,
		models.JobNameWebhookRetries, models.JobNameAlertEval,
	} {
		require.NotContains(t, got, outbound)
	}
}
