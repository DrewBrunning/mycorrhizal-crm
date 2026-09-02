package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/httputil"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Instance-wide system diagnostics, issue #423 — the "is this install
// healthy?" sweep an operator runs after an install, an upgrade, a migration,
// or a config change. It is the manual, admin-gated, one-pass counterpart to
// the continuous surfaces:
//
//   - GET /health (/ready, /live) is unauthenticated and answers "is the
//     process up / can it serve / is the CRM operational" continuously, but it
//     deliberately omits operator-only detail (config redaction, disk usage,
//     notification-channel config, per-user integration reachability) and is
//     cached to stay cheap for a polling load balancer.
//   - GET /admin/* (subsystem-health, job-runs, notification-health, ...) each
//     answer one operational question in depth.
//
// RunDiagnostics folds the same single-check paths those surfaces already use
// (checkDatabaseHealth's read probe, migration lag, the persisted restore-drill
// / integrity-check results, notification-channel health, job-run health) into
// one ok / warning / error checklist with a summary — it does not re-implement
// any of them. It is read-only: the only side effect is the same log lines the
// individual checks already emit.
//
// No secrets appear in the response: config values are never echoed, only the
// names of failing variables; integration base URLs are logged, never
// returned. Every outbound probe is time-bounded (integrationProbeTimeout) so
// an unreachable remote cannot hang the sweep, and the whole sweep runs under a
// caller-provided context so the handler can impose a total budget too.

// Diagnostic status values — a checklist of ok / warning / error.
const (
	DiagStatusOK      = "ok"
	DiagStatusWarning = "warning"
	DiagStatusError   = "error"
)

// DiagnosticCheck is one row of the sweep: a stable identifier, a status, and
// a short, secret-free human message.
type DiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DiagnosticSummary is the roll-up the issue asks for ("2 warnings, 0 errors").
type DiagnosticSummary struct {
	Status   string `json:"status"` // ok | warning | error
	OK       int    `json:"ok"`
	Warnings int    `json:"warnings"`
	Errors   int    `json:"errors"`
}

// Diagnostics is the full sweep output.
type Diagnostics struct {
	Timestamp string            `json:"timestamp"`
	Summary   DiagnosticSummary `json:"summary"`
	Checks    []DiagnosticCheck `json:"checks"`
}

// integrationProbeTimeout bounds every outbound reachability probe in the
// sweep — an unreachable remote becomes a "warning" after this, never a hang.
const integrationProbeTimeout = 2 * time.Second

// maxIntegrationProbes caps the number of distinct per-user integration
// endpoints probed in one sweep. The endpoint is admin-gated, but an instance
// with many users can still accumulate many distinct Immich/Paperless/Seafile/
// Nextcloud URLs; beyond this cap the remainder is skipped and the check says
// so, keeping the sweep bounded on large installs.
const maxIntegrationProbes = 12

// Sentinel errors surfaced by the guarded probe dialer (see probeIntegration).
var (
	errDiagnosticUnreachable    = errors.New("diagnostics: integration unreachable")
	errDiagnosticPrivateAddress = errors.New("diagnostics: integration resolves to a private or loopback address")
)

// RunDiagnostics runs the full sweep and folds it into one Diagnostics
// checklist. db may be nil (the endpoint then reports the database check as an
// error and continues); cfg must be non-nil.
func RunDiagnostics(ctx context.Context, db *gorm.DB, cfg config.Config) Diagnostics {
	out := Diagnostics{Timestamp: time.Now().UTC().Format(time.RFC3339)}

	out.Checks = append(out.Checks,
		diagnosticsConfig(cfg),
		diagnosticsDatabase(db),
		diagnosticsMigrations(db),
		diagnosticsFilesystem(cfg),
		diagnosticsBackup(db, cfg),
		diagnosticsDataIntegrity(db, cfg),
	)
	out.Checks = append(out.Checks, diagnosticsNotifications(ctx, db, cfg)...)
	out.Checks = append(out.Checks, diagnosticsIntegrations(ctx, db, cfg)...)
	out.Checks = append(out.Checks,
		diagnosticsDiskSpace(cfg),
		diagnosticsBackgroundJobs(ctx, db),
		diagnosticsVersion(),
	)

	// The summary counts every check and derives the overall status: an error
	// dominates, then a warning, then all-ok. Check count includes the
	// ok-level rows.
	out.Summary = rollUpDiagnostics(out.Checks)
	return out
}

func rollUpDiagnostics(checks []DiagnosticCheck) DiagnosticSummary {
	s := DiagnosticSummary{Status: DiagStatusOK}
	for _, c := range checks {
		switch c.Status {
		case DiagStatusError:
			s.Errors++
			s.Status = DiagStatusError
		case DiagStatusWarning:
			s.Warnings++
			if s.Status != DiagStatusError {
				s.Status = DiagStatusWarning
			}
		default:
			s.OK++
		}
	}
	return s
}

// diagnosticsConfig reuses config.Validate() — the same gate boot enforces —
// and reports the *names* of the failing variables, never their values.
func diagnosticsConfig(cfg config.Config) DiagnosticCheck {
	verrs := cfg.Validate()
	if len(verrs) > 0 {
		fields := make([]string, 0, len(verrs))
		for _, v := range verrs {
			fields = append(fields, v.Field)
		}
		sort.Strings(fields)
		return DiagnosticCheck{
			Name:    "config",
			Status:  DiagStatusError,
			Message: fmt.Sprintf("%d configuration problem(s): %s", len(fields), strings.Join(fields, ", ")),
		}
	}
	return DiagnosticCheck{Name: "config", Status: DiagStatusOK, Message: "configuration is valid"}
}

// diagnosticsDatabase reuses liveDatabaseReadCheck (the same read probe
// GET /health runs). A database read failure is the only check that is an
// outright error — the app cannot function without its database.
func diagnosticsDatabase(db *gorm.DB) DiagnosticCheck {
	if db == nil {
		return DiagnosticCheck{Name: "database", Status: DiagStatusError, Message: "no database handle"}
	}
	d := liveDatabaseReadCheck(db)
	if d.Status == DeepStatusUnhealthy {
		return DiagnosticCheck{Name: "database", Status: DiagStatusError, Message: "database unreachable"}
	}
	return DiagnosticCheck{Name: "database", Status: DiagStatusOK, Message: "database reachable"}
}

// diagnosticsMigrations reuses migrationLagCheck (the same fold GET /health
// runs). A schema behind the binary is a warning — the instance still serves,
// but an upgrade is pending.
func diagnosticsMigrations(db *gorm.DB) DiagnosticCheck {
	if db == nil {
		return DiagnosticCheck{Name: "migrations", Status: DiagStatusWarning, Message: "cannot read migration state"}
	}
	d := migrationLagCheck(db)
	switch d.Status {
	case DeepStatusOK:
		return DiagnosticCheck{Name: "migrations", Status: DiagStatusOK, Message: "schema is up to date"}
	default:
		return DiagnosticCheck{Name: "migrations", Status: DiagStatusWarning, Message: d.Reason}
	}
}

// diagnosticsFilesystem probes every operator-owned storage directory the
// instance must be able to write to: the profile-photo dir, the attachments
// dir, and the directory holding the database (where backups land by default,
// see database.DefaultBackupPath). A non-writable storage dir is an error —
// writes will fail, which is exactly the "attachment directory became
// read-only" case the issue calls out.
func diagnosticsFilesystem(cfg config.Config) DiagnosticCheck {
	dirs := []struct {
		label string
		dir   string
	}{
		{"profile photo directory", cfg.ProfilePhotoDir},
		{"attachments directory", cfg.AttachmentsDir},
	}
	if cfg.DBPath != "" {
		dirs = append(dirs, struct {
			label string
			dir   string
		}{"database directory", filepath.Dir(cfg.DBPath)})
	}

	for _, d := range dirs {
		if d.dir == "" {
			continue
		}
		if reason := ProbeWritableDir(d.dir); reason != "" {
			// The absolute path goes to the log only (ASVS 7.4.1).
			logger.Error().Str("dir", d.dir).Str("label", d.label).
				Msg("diagnostics: " + d.label + " " + reason)
			return DiagnosticCheck{
				Name:    "filesystem",
				Status:  DiagStatusError,
				Message: d.label + " " + reason,
			}
		}
	}

	if cfg.FCMServiceAccountFile != "" {
		if f, err := os.Open(cfg.FCMServiceAccountFile); err != nil {
			logger.Error().Err(err).Str("path", cfg.FCMServiceAccountFile).
				Msg("diagnostics: FCM service-account file not readable")
			return DiagnosticCheck{Name: "filesystem", Status: DiagStatusWarning,
				Message: "FCM service-account file is not readable"}
		} else {
			_ = f.Close()
		}
	}

	return DiagnosticCheck{Name: "filesystem", Status: DiagStatusOK, Message: "storage directories are writable"}
}

// ProbeWritableDir returns "" when dir exists, is a directory, and a file can
// be created in it; otherwise a generic, path-free reason (the detail is
// logged by the caller). Shared by the readiness endpoint (readinessFilesystem)
// and the diagnostics sweep so the two surfaces never disagree on what
// "writable" means.
func ProbeWritableDir(dir string) string {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "is missing"
		}
		return "is not accessible"
	}
	if !info.IsDir() {
		return "is not a directory"
	}
	probe := filepath.Join(dir, ".health-write-probe")
	// #nosec G304 -- dir is an operator-supplied config path (PROFILE_PHOTO_DIR /
	// ATTACHMENTS_DIR, validated absolute at startup), never request input; the
	// filename is a hardcoded constant.
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "is not writable"
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return ""
}

// diagnosticsBackup reuses persistedCheckDetail for the restore-drill job —
// the drill snapshots the live DB, restores it into a scratch DB, verifies the
// wrapped DEK decrypts, and compares per-table row counts, so its last result
// IS the latest backup-validity proof. A stale or failed drill is a warning,
// not an error: the instance still serves, it just has no fresh restore
// proof. A disabled drill is reported as ok (it is a deliberate configuration,
// not a problem).
func diagnosticsBackup(db *gorm.DB, cfg config.Config) DiagnosticCheck {
	if db == nil {
		return DiagnosticCheck{Name: "backup", Status: DiagStatusWarning, Message: "cannot read backup state"}
	}
	d := persistedCheckDetail(db, models.JobNameRestoreDrill,
		cfg.DBRestoreDrillEnabled, cfg.DBRestoreDrillIntervalHours)
	switch d.Status {
	case DeepStatusOK:
		return DiagnosticCheck{Name: "backup", Status: DiagStatusOK, Message: "last restore drill passed"}
	case DeepStatusNotConfigured:
		return DiagnosticCheck{Name: "backup", Status: DiagStatusOK, Message: "restore drill is disabled"}
	default:
		return DiagnosticCheck{Name: "backup", Status: DiagStatusWarning, Message: d.Reason}
	}
}

// diagnosticsDataIntegrity reuses persistedCheckDetail for the data-invariant
// pass (DB-01, issue #460) — the last recorded result of the checker that
// looks for relationships pointing at deleted contacts, orphaned join rows,
// dangling external references and malformed canonical records. A stale or
// failed pass is a warning (the instance still serves; the data has a logical
// hole an operator should look at with `doctor` or GET /admin/integrity-check).
// A disabled check is reported ok (deliberate configuration). This is the
// #423 coordination point: the operational doctor surfaces the data doctor's
// verdict without re-running it.
func diagnosticsDataIntegrity(db *gorm.DB, cfg config.Config) DiagnosticCheck {
	if db == nil {
		return DiagnosticCheck{Name: "data_integrity", Status: DiagStatusWarning, Message: "cannot read data-integrity state"}
	}
	d := persistedCheckDetail(db, models.CheckNameDataIntegrity,
		cfg.DBIntegrityCheckEnabled, cfg.DBIntegrityCheckIntervalHours)
	switch d.Status {
	case DeepStatusOK:
		return DiagnosticCheck{Name: "data_integrity", Status: DiagStatusOK, Message: "last data-invariant check passed"}
	case DeepStatusNotConfigured:
		return DiagnosticCheck{Name: "data_integrity", Status: DiagStatusOK, Message: "data-invariant check is disabled"}
	default:
		return DiagnosticCheck{Name: "data_integrity", Status: DiagStatusWarning, Message: d.Reason}
	}
}

// diagnosticsNotifications reuses ComputeNotificationChannelHealth (#422) and
// produces one check per channel. failing is an error (notifications are
// actively broken), no_devices a warning (push is provisioned but nobody is
// home), unconfigured and healthy are ok. No per-user delivery content and no
// channel secrets ever appear — only the status-derived message.
func diagnosticsNotifications(ctx context.Context, db *gorm.DB, cfg config.Config) []DiagnosticCheck {
	if db == nil {
		return []DiagnosticCheck{{
			Name: "notification_email", Status: DiagStatusWarning, Message: "cannot read notification state",
		}}
	}
	health, err := ComputeNotificationChannelHealth(ctx, db, cfg)
	if err != nil {
		logger.Error().Err(err).Msg("diagnostics: cannot compute notification channel health")
		return []DiagnosticCheck{{
			Name: "notification_channels", Status: DiagStatusWarning, Message: "cannot read notification state",
		}}
	}
	checks := make([]DiagnosticCheck, 0, len(health))
	for _, h := range health {
		c := DiagnosticCheck{Name: "notification_" + h.Channel}
		switch h.Status {
		case NotificationHealthFailing:
			c.Status = DiagStatusError
			c.Message = "last delivery failed"
			if h.LastError != "" {
				c.Message += ": " + h.LastError
			}
		case NotificationHealthNoDevices:
			c.Status = DiagStatusWarning
			c.Message = "configured but no devices can receive"
		case NotificationHealthUnconfigured:
			c.Status = DiagStatusOK
			c.Message = "not configured"
		default:
			c.Status = DiagStatusOK
			c.Message = "reachable"
		}
		checks = append(checks, c)
	}
	return checks
}

// diagnosticsIntegrations checks the server-scoped surfaces the instance
// itself serves (CardDAV / CalDAV — enabled is reachable, same process) and,
// where cheap, the per-user integrations (Immich, Paperless, Seafile,
// Nextcloud): distinct configured base URLs are probed with a short timeout,
// honoring each integration's block-private-URLs SSRF policy. Unreachable or
// blocked endpoints are warnings (optional integrations degrade, not fail).
// Base URLs are logged, never returned; nothing about a connection's
// credentials is touched.
func diagnosticsIntegrations(ctx context.Context, db *gorm.DB, cfg config.Config) []DiagnosticCheck {
	checks := []DiagnosticCheck{
		{Name: "integration_carddav", Status: DiagStatusOK},
		{Name: "integration_caldav", Status: DiagStatusOK},
	}
	if cfg.CardDAVEnabled {
		checks[0].Message = "enabled"
	} else {
		checks[0].Message = "disabled"
	}
	if cfg.CalDAVEnabled {
		checks[1].Message = "enabled"
	} else {
		checks[1].Message = "disabled"
	}

	if db == nil {
		return checks
	}

	systems := []struct {
		checkName    string
		model        interface{}
		blockPrivate bool
	}{
		{"integration_immich", &models.ImmichConfig{}, cfg.ImmichBlockPrivateURLs},
		{"integration_paperless", &models.PaperlessConfig{}, cfg.PaperlessBlockPrivateURLs},
		{"integration_seafile", &models.SeafileConfig{}, cfg.SeafileBlockPrivateURLs},
		{"integration_nextcloud", &models.WebDAVConfig{}, cfg.WebDAVBlockPrivateURLs},
	}

	probed := 0
	for _, s := range systems {
		var urls []string
		// Distinct base URLs across users, soft-deleted rows excluded (GORM's
		// soft-delete scope on these gorm.Model configs). Base URLs are not
		// secrets — the admin view already lists configs — but they still never
		// leave this function except to the log.
		if err := db.WithContext(ctx).Model(s.model).
			Distinct("base_url").
			Pluck("base_url", &urls).Error; err != nil {
			logger.Error().Err(err).Str("check", s.checkName).Msg("diagnostics: cannot read integration configs")
			checks = append(checks, DiagnosticCheck{
				Name: s.checkName, Status: DiagStatusWarning, Message: "cannot read configuration",
			})
			continue
		}

		if len(urls) == 0 {
			checks = append(checks, DiagnosticCheck{
				Name: s.checkName, Status: DiagStatusOK, Message: "no instances configured",
			})
			continue
		}

		if probed >= maxIntegrationProbes {
			checks = append(checks, DiagnosticCheck{
				Name: s.checkName, Status: DiagStatusWarning,
				Message: fmt.Sprintf("%d endpoint(s) configured but not probed (probe cap reached)", len(urls)),
			})
			probed += len(urls)
			continue
		}

		reachable := 0
		probedForSystem := 0
		for _, u := range urls {
			if probed >= maxIntegrationProbes {
				break
			}
			d := probeIntegration(s.checkName, u, s.blockPrivate, integrationProbeTimeout)
			if d.Status == DeepStatusOK {
				reachable++
			}
			probed++
			probedForSystem++
		}

		c := DiagnosticCheck{Name: s.checkName, Status: DiagStatusOK,
			Message: fmt.Sprintf("%d endpoint(s) reachable", len(urls))}
		switch {
		case probedForSystem < len(urls):
			// The sweep's probe budget ran out mid-system: the unprobed
			// remainder is neither reachable nor unreachable, so say exactly
			// that rather than guessing.
			c.Status = DiagStatusWarning
			c.Message = fmt.Sprintf("probe cap reached after %d of %d endpoint(s); %d reachable",
				probedForSystem, len(urls), reachable)
		case reachable < len(urls):
			c.Status = DiagStatusWarning
			c.Message = fmt.Sprintf("%d of %d endpoint(s) unreachable", len(urls)-reachable, len(urls))
		}
		checks = append(checks, c)
	}
	return checks
}

// probeIntegration probes one operator-configured integration base URL with a
// short timeout, honoring the integration's block-private-URLs SSRF policy in
// the dialer (the same defense the real clients use). The URL and the transport
// error go to the log, never into the response body.
func probeIntegration(name, baseURL string, blockPrivate bool, timeout time.Duration) HealthCheckDetail {
	client := http.DefaultClient
	if blockPrivate {
		client = &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: httputil.SafeDialContext(errDiagnosticUnreachable, errDiagnosticPrivateAddress),
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		logger.Error().Err(err).Str("integration", name).Msg("diagnostics: bad integration base URL")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "misconfigured"}
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, errDiagnosticPrivateAddress) {
			logger.Warn().Str("integration", name).Msg("diagnostics: integration blocked by private-address policy")
			return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "blocked by private-address policy"}
		}
		logger.Warn().Err(err).Str("integration", name).Msg("diagnostics: integration probe failed")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "unreachable"}
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 500 {
		logger.Warn().Str("integration", name).Int("status", resp.StatusCode).
			Msg("diagnostics: integration returned 5xx")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "upstream error"}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

// diagnosticsDiskSpace reuses the same statfs fold the alert evaluator uses
// (statfsDiskUsage, #428). The configured alert threshold (or 90% when
// disabled) is the warning line; a filesystem essentially full is an error.
func diagnosticsDiskSpace(cfg config.Config) DiagnosticCheck {
	if cfg.DBPath == "" {
		return DiagnosticCheck{Name: "disk_space", Status: DiagStatusOK, Message: "no database path configured"}
	}
	dir := filepath.Dir(cfg.DBPath)
	used, err := diskUsageFn(dir)
	if err != nil {
		logger.Error().Err(err).Str("path", dir).Msg("diagnostics: failed to stat filesystem")
		return DiagnosticCheck{Name: "disk_space", Status: DiagStatusWarning, Message: "cannot stat filesystem"}
	}
	threshold := cfg.AlertDiskUsagePercent
	if threshold <= 0 {
		threshold = 90
	}
	switch {
	case used >= 99:
		return DiagnosticCheck{Name: "disk_space", Status: DiagStatusError,
			Message: fmt.Sprintf("filesystem is %d%% full", used)}
	case used >= threshold:
		return DiagnosticCheck{Name: "disk_space", Status: DiagStatusWarning,
			Message: fmt.Sprintf("filesystem is %d%% full (threshold %d%%)", used, threshold)}
	default:
		return DiagnosticCheck{Name: "disk_space", Status: DiagStatusOK,
			Message: fmt.Sprintf("filesystem is %d%% full", used)}
	}
}

// diagnosticsBackgroundJobs reuses the job-run health fold (#391) plus the
// deep-health lock-liveness fold: any failing job or any stuck lock is a
// warning (the instance still serves, but a scheduled job is not completing).
func diagnosticsBackgroundJobs(ctx context.Context, db *gorm.DB) DiagnosticCheck {
	if db == nil {
		return DiagnosticCheck{Name: "background_jobs", Status: DiagStatusWarning, Message: "cannot read job state"}
	}

	locks := backgroundJobsCheck(db)
	var problems []string
	if locks.Status == DeepStatusDegraded {
		problems = append(problems, locks.Reason)
	}

	jobs, err := ComputeJobRunHealth(ctx, db)
	if err != nil {
		logger.Error().Err(err).Msg("diagnostics: cannot compute job-run health")
		problems = append(problems, "cannot read job-run history")
	} else {
		var failing []string
		for _, j := range jobs {
			if j.Status == JobRunStatusFailing {
				failing = append(failing, j.JobName)
			}
		}
		if len(failing) > 0 {
			sort.Strings(failing)
			problems = append(problems, "failing job(s): "+strings.Join(failing, ", "))
		}
	}

	if len(problems) > 0 {
		return DiagnosticCheck{Name: "background_jobs", Status: DiagStatusWarning,
			Message: strings.Join(problems, "; ")}
	}
	return DiagnosticCheck{Name: "background_jobs", Status: DiagStatusOK, Message: "scheduled jobs are healthy"}
}

// diagnosticsVersion reports the build's version / commit / build date — the
// "version / update state" row. Informational, always ok.
func diagnosticsVersion() DiagnosticCheck {
	b := buildinfo.Get()
	msg := "version " + b.Version
	if b.Commit != "" {
		msg += " (" + b.Commit + ")"
	}
	if b.BuildDate != "" {
		msg += ", built " + b.BuildDate
	}
	return DiagnosticCheck{Name: "version", Status: DiagStatusOK, Message: msg}
}
