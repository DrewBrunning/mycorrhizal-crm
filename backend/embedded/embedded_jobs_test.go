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
		models.JobNameDailyReminders,
		models.JobNamePurgeDeleted,
		models.JobNameCalendarSync,
		models.JobNameImmichSync,
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
