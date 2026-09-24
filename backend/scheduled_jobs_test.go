package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/require"
)

// jobNameConstantsFromSource AST-scans backend/models/job_execution.go for
// every `JobName...` string constant and returns {constant identifier ->
// its string value}. Scanning the source (rather than hand-listing the
// tokens here) means a newly added JobName* constant is automatically
// picked up by TestRegisterScheduledJobs_EveryJobNameAccountedFor without
// anyone remembering to update this test.
func jobNameConstantsFromSource(t *testing.T) map[string]string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must resolve this test file's path")
	path := filepath.Join(filepath.Dir(thisFile), "models", "job_execution.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, "parsing %s", path)

	consts := map[string]string{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "JobName") {
					continue
				}
				require.Less(t, i, len(valueSpec.Values),
					"%s must have an explicit string literal value for the AST scan to read", name.Name)
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				require.True(t, ok && lit.Kind == token.STRING,
					"%s must be assigned a plain string literal", name.Name)
				value, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				consts[name.Name] = value
			}
		}
	}
	require.NotEmpty(t, consts, "expected to find JobName* constants in %s", path)
	return consts
}

// testSchedulerConfig returns a config.Config with every field
// registerScheduledJobs reads set to a distinct, valid, nonzero value —
// nonzero so registration itself never fails, distinct so a test asserting
// "this job's interval came from config" can't pass by accident against a
// hardcoded literal that happens to match a shared default.
func testSchedulerConfig() config.Config {
	return config.Config{
		ReminderTime:                  "06:00",
		CalDAVSyncIntervalHours:       6,
		ImmichSyncIntervalHours:       7,
		DBIntegrityCheckIntervalHours: 24,
		DBRestoreDrillIntervalHours:   168,
		AlertEvalIntervalMinutes:      15,
	}
}

// jobsByTag registers no jobs itself; it just indexes an already-registered
// scheduler's jobs by their models.JobName* tag for easy lookup in
// assertions below.
func jobsByTag(s *gocron.Scheduler) map[string]*gocron.Job {
	byTag := map[string]*gocron.Job{}
	for _, job := range s.Jobs() {
		for _, tag := range job.Tags() {
			byTag[tag] = job
		}
	}
	return byTag
}

// TestRegisterScheduledJobs_EveryJobNameAccountedFor pins the actual bug
// class this refactor exists to catch: main.go used to inline every
// s.Every(...).Do(...) call with its error discarded, so deleting one (e.g.
// a data-retention purge) failed no test. Every models.JobName* constant
// found by the AST scan must now either be registered with the scheduler
// (tagged with its own name) or be listed in manualJobReasons with a
// documented reason it is operator-triggered only.
func TestRegisterScheduledJobs_EveryJobNameAccountedFor(t *testing.T) {
	constants := jobNameConstantsFromSource(t)

	cfg := testSchedulerConfig()
	s := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(s, nil, &cfg))

	registered := jobsByTag(s)

	for constName, jobName := range constants {
		if reason, manual := manualJobReasons[jobName]; manual {
			require.NotEmpty(t, reason,
				"manualJobReasons[%s] must document why it is not scheduled", jobName)
			_, isRegistered := registered[jobName]
			require.False(t, isRegistered,
				"%s (%q) is listed in manualJobReasons but was also registered with the scheduler — pick one", constName, jobName)
			continue
		}
		_, isRegistered := registered[jobName]
		require.True(t, isRegistered,
			"models.%s (%q) is not registered by registerScheduledJobs and not in manualJobReasons — "+
				"a new scheduled job (or a deleted registration line) must be caught here", constName, jobName)
	}

	// The other direction: every tag actually applied must correspond to a
	// real JobName* constant, so a typo'd literal tag can't silently "count".
	knownValues := map[string]bool{}
	for _, v := range constants {
		knownValues[v] = true
	}
	for tag := range registered {
		require.True(t, knownValues[tag],
			"scheduler tag %q does not match any models.JobName* constant found in models/job_execution.go", tag)
	}
}

// TestRegisterScheduledJobs_ConfigDriven pins that each job's recurrence
// actually comes from config.Config, not a hardcoded literal that happens
// to match the shipped default — using distinct values per field so a
// wrong-field mixup (e.g. Immich's interval wired to Alert's config field)
// would fail too.
func TestRegisterScheduledJobs_ConfigDriven(t *testing.T) {
	cfg := config.Config{
		ReminderTime:                  "08:15",
		CalDAVSyncIntervalHours:       9,
		ImmichSyncIntervalHours:       11,
		DBIntegrityCheckIntervalHours: 13,
		DBRestoreDrillIntervalHours:   170,
		AlertEvalIntervalMinutes:      42,
	}

	s := gocron.NewScheduler(time.UTC)
	require.NoError(t, registerScheduledJobs(s, nil, &cfg))

	byTag := jobsByTag(s)

	require.Equal(t, "hours", byTag[models.JobNameCalendarSync].ScheduledUnit())
	require.Equal(t, cfg.CalDAVSyncIntervalHours, byTag[models.JobNameCalendarSync].ScheduledInterval())

	require.Equal(t, "hours", byTag[models.JobNameImmichSync].ScheduledUnit())
	require.Equal(t, cfg.ImmichSyncIntervalHours, byTag[models.JobNameImmichSync].ScheduledInterval())

	require.Equal(t, "hours", byTag[models.JobNameDBIntegrityCheck].ScheduledUnit())
	require.Equal(t, cfg.DBIntegrityCheckIntervalHours, byTag[models.JobNameDBIntegrityCheck].ScheduledInterval())

	require.Equal(t, "hours", byTag[models.JobNameRestoreDrill].ScheduledUnit())
	require.Equal(t, cfg.DBRestoreDrillIntervalHours, byTag[models.JobNameRestoreDrill].ScheduledInterval())

	require.Equal(t, "minutes", byTag[models.JobNameAlertEval].ScheduledUnit())
	require.Equal(t, cfg.AlertEvalIntervalMinutes, byTag[models.JobNameAlertEval].ScheduledInterval())

	require.Equal(t, "days", byTag[models.JobNameDailyReminders].ScheduledUnit())
	require.Equal(t, cfg.ReminderTime, byTag[models.JobNameDailyReminders].ScheduledAtTime())
}

// TestRegisterScheduledJobs_InvalidAtTimeReturnsError pins that a malformed
// .At() time fails registration loudly instead of the pre-refactor behavior
// (the error from s.Every(...).At(...).Do(...) was simply discarded, so a
// bad REMINDER_TIME would silently produce a reminder job that never runs).
func TestRegisterScheduledJobs_InvalidAtTimeReturnsError(t *testing.T) {
	cfg := testSchedulerConfig()
	cfg.ReminderTime = "not-a-time"

	s := gocron.NewScheduler(time.UTC)
	err := registerScheduledJobs(s, nil, &cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), models.JobNameDailyReminders)
}

// TestRegisterScheduledJobs_InvalidIntervalReturnsError pins the same
// failure mode for a config-driven interval: a zero/negative hours value
// (e.g. an operator setting CALDAV_SYNC_INTERVAL_HOURS=0, which
// config.ValidateOrPanic would normally reject before this runs, but this
// function must fail closed on its own too rather than rely solely on that
// upstream guard) must fail registration rather than silently registering
// a job with an invalid interval.
func TestRegisterScheduledJobs_InvalidIntervalReturnsError(t *testing.T) {
	cfg := testSchedulerConfig()
	cfg.CalDAVSyncIntervalHours = 0

	s := gocron.NewScheduler(time.UTC)
	err := registerScheduledJobs(s, nil, &cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), models.JobNameCalendarSync)
}
