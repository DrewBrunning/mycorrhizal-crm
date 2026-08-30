package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Deep-health sub-check statuses. These describe one facet of "is the CRM
// operational"; they are NOT the endpoint's HTTP outcome (see
// rollUpDeepStatus / the controller for that).
const (
	// DeepStatusOK — this facet is fine.
	DeepStatusOK = "ok"
	// DeepStatusDegraded — this facet has a problem, but the process is alive
	// and (for anything other than a DB read failure) can still serve. Never
	// 503s on its own.
	DeepStatusDegraded = "degraded"
	// DeepStatusUnhealthy — reserved for a database read failure: the app
	// cannot function at all. The only facet that makes GET /health return 503.
	DeepStatusUnhealthy = "unhealthy"
	// DeepStatusNotConfigured — an optional facet the operator has not enabled.
	// Informational; does not degrade the overall status.
	DeepStatusNotConfigured = "not_configured"
)

// deepHealthCacheTTL bounds how often the slow / side-effecting section of the
// snapshot is recomputed — the integration reachability probes, the persisted
// integrity-check / restore-drill outcomes, and the single DB write probe — so
// an unauthenticated caller polling GET /health cannot drive repeated outbound
// calls or repeated writes. A package var, not a const, so tests can zero it.
// The live section (DB read probe, migration lag, background-job locks) is
// recomputed on every call.
var deepHealthCacheTTL = 30 * time.Second

// opCheckWriteProbe is the reserved operational_check_results row the deep DB
// write probe upserts. Never surfaced as a "check" in the response — only the
// db_integrity_check / restore_drill rows are read back.
const opCheckWriteProbe = "_db_write_probe"

// stuckLockThreshold is how long a held job lock must persist before the deep
// check calls it stuck. acquireJobLock (reminder_service.go) already treats a
// lock as stale and takes it over after 5 minutes; this is that plus margin so
// a legitimately long run mid-flight is not flagged.
const stuckLockThreshold = 15 * time.Minute

// HealthCheckDetail is one facet's status plus, when not ok, a short reason.
type HealthCheckDetail struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// BackgroundJobStatus is the last-known state of one scheduled job, read from
// the job_executions bookkeeping table.
type BackgroundJobStatus struct {
	Name      string     `json:"name"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	Locked    bool       `json:"locked"`
	Stuck     bool       `json:"stuck"`
}

// BackgroundJobsCheck reports scheduled-job visibility: every job_executions
// row, flagged degraded when any lock has been held far longer than a run
// should take (a crashed worker that never released it).
type BackgroundJobsCheck struct {
	Status string                `json:"status"`
	Reason string                `json:"reason,omitempty"`
	Jobs   []BackgroundJobStatus `json:"jobs,omitempty"`
}

// DeepHealth is the "is the CRM operational" picture behind GET /health.
type DeepHealth struct {
	// Status is the rolled-up facet status: healthy | degraded | unhealthy.
	Status         string                       `json:"status"`
	Database       HealthCheckDetail            `json:"database"`
	Migrations     HealthCheckDetail            `json:"migrations"`
	IntegrityCheck HealthCheckDetail            `json:"integrity_check"`
	RestoreDrill   HealthCheckDetail            `json:"restore_drill"`
	BackgroundJobs BackgroundJobsCheck          `json:"background_jobs"`
	Integrations   map[string]HealthCheckDetail `json:"integrations"`
}

// deepHealthCache memoizes the slow / side-effecting section of the snapshot.
var deepHealthCache struct {
	mu  sync.Mutex
	at  time.Time
	val cachedDeepSection
}

type cachedDeepSection struct {
	dbWrite        HealthCheckDetail
	integrityCheck HealthCheckDetail
	restoreDrill   HealthCheckDetail
	integrations   map[string]HealthCheckDetail
}

// DeepHealthSnapshot assembles the deep-health picture. The live section (DB
// read probe, migration lag, background-job locks) is computed every call; the
// rest is served from a deepHealthCacheTTL cache.
func DeepHealthSnapshot(db *gorm.DB, cfg config.Config) DeepHealth {
	cached := cachedDeepHealthSection(db, cfg)

	h := DeepHealth{
		Migrations:     migrationLagCheck(db),
		BackgroundJobs: backgroundJobsCheck(db),
		IntegrityCheck: cached.integrityCheck,
		RestoreDrill:   cached.restoreDrill,
		Integrations:   cached.integrations,
	}

	if read := liveDatabaseReadCheck(db); read.Status == DeepStatusUnhealthy {
		h.Database = read
	} else {
		// Read works; the DB facet then reflects the (cached) write probe, so a
		// read-only filesystem / full disk shows as degraded rather than ok.
		h.Database = cached.dbWrite
	}

	h.Status = rollUpDeepStatus(h)
	return h
}

// ResetDeepHealthCache clears the memoized section. Test helper.
func ResetDeepHealthCache() {
	deepHealthCache.mu.Lock()
	deepHealthCache.at = time.Time{}
	deepHealthCache.val = cachedDeepSection{}
	deepHealthCache.mu.Unlock()
}

// SetDeepHealthCacheTTL overrides the cache window (0 disables caching). Test
// seam; returns the previous value so a test can restore it.
func SetDeepHealthCacheTTL(d time.Duration) time.Duration {
	prev := deepHealthCacheTTL
	deepHealthCacheTTL = d
	return prev
}

func cachedDeepHealthSection(db *gorm.DB, cfg config.Config) cachedDeepSection {
	deepHealthCache.mu.Lock()
	defer deepHealthCache.mu.Unlock()

	if !deepHealthCache.at.IsZero() && time.Since(deepHealthCache.at) < deepHealthCacheTTL {
		return deepHealthCache.val
	}

	val := cachedDeepSection{
		dbWrite: dbWriteProbe(db),
		integrityCheck: persistedCheckDetail(db, models.JobNameDBIntegrityCheck,
			cfg.DBIntegrityCheckEnabled, cfg.DBIntegrityCheckIntervalHours),
		restoreDrill: persistedCheckDetail(db, models.JobNameRestoreDrill,
			cfg.DBRestoreDrillEnabled, cfg.DBRestoreDrillIntervalHours),
		integrations: probeServerIntegrations(cfg),
	}

	deepHealthCache.at = time.Now()
	deepHealthCache.val = val
	return val
}

// liveDatabaseReadCheck is the only facet that can 503 the endpoint.
func liveDatabaseReadCheck(db *gorm.DB) HealthCheckDetail {
	sqlDB, err := db.DB()
	if err != nil {
		logger.Error().Err(err).Msg("deep health: no database handle")
		return HealthCheckDetail{Status: DeepStatusUnhealthy, Reason: "no database handle"}
	}
	if err := sqlDB.Ping(); err != nil {
		logger.Error().Err(err).Msg("deep health: database ping failed")
		return HealthCheckDetail{Status: DeepStatusUnhealthy, Reason: "database unreachable"}
	}
	var one int
	if err := db.Raw("SELECT 1").Scan(&one).Error; err != nil {
		logger.Error().Err(err).Msg("deep health: database read probe failed")
		return HealthCheckDetail{Status: DeepStatusUnhealthy, Reason: "database read failed"}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

// dbWriteProbe performs one genuine write — a single-row upsert into
// operational_check_results under a reserved key (no user data, bounded) — so a
// read-only or full disk is caught. Runs at most once per deepHealthCacheTTL.
func dbWriteProbe(db *gorm.DB) HealthCheckDetail {
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		var row models.OperationalCheckResult
		e := tx.Where("check_name = ?", opCheckWriteProbe).First(&row).Error
		if e == gorm.ErrRecordNotFound {
			return tx.Create(&models.OperationalCheckResult{
				CheckName: opCheckWriteProbe, Status: models.OpCheckStatusOK, CheckedAt: now,
			}).Error
		}
		if e != nil {
			return e
		}
		return tx.Model(&row).Update("checked_at", now).Error
	})
	if err != nil {
		logger.Error().Err(err).Msg("deep health: database write probe failed")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "database write failed"}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

func migrationLagCheck(db *gorm.DB) HealthCheckDetail {
	applied, dirty, ok, err := database.AppliedMigrationVersion(db)
	if err != nil {
		logger.Error().Err(err).Msg("deep health: cannot read migration state")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "cannot read migration state"}
	}
	if !ok {
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "no migrations have been applied"}
	}
	if dirty {
		return HealthCheckDetail{Status: DeepStatusDegraded,
			Reason: fmt.Sprintf("migration %d is in a dirty state", applied)}
	}
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		logger.Error().Err(err).Msg("deep health: cannot resolve latest migration version")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "cannot resolve latest migration version"}
	}
	if applied > latest {
		// The database knows migrations this binary does not — a rollback in
		// progress. The startup path refuses on this state (issue #439); the
		// health check names it so an operator sees WHY the instance is not
		// serving instead of a generic boot failure.
		return HealthCheckDetail{Status: DeepStatusDegraded,
			Reason: fmt.Sprintf("database schema is ahead of the binary (applied=%d, latest=%d) — this binary was rolled back", applied, latest)}
	}
	if applied < latest {
		return HealthCheckDetail{Status: DeepStatusDegraded,
			Reason: fmt.Sprintf("database schema is behind the binary (applied=%d, latest=%d)", applied, latest)}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

func backgroundJobsCheck(db *gorm.DB) BackgroundJobsCheck {
	var rows []models.JobExecution
	if err := db.Order("job_name asc").Find(&rows).Error; err != nil {
		logger.Error().Err(err).Msg("deep health: cannot read job_executions")
		return BackgroundJobsCheck{Status: DeepStatusDegraded, Reason: "cannot read background-job state"}
	}

	out := BackgroundJobsCheck{Status: DeepStatusOK}
	var stuck []string
	now := time.Now()
	for _, r := range rows {
		lastRun := r.LastRunAt
		js := BackgroundJobStatus{Name: r.JobName, LastRunAt: &lastRun, Locked: r.LockedAt != nil}
		if r.LockedAt != nil && now.Sub(*r.LockedAt) > stuckLockThreshold {
			js.Stuck = true
			stuck = append(stuck, r.JobName)
		}
		out.Jobs = append(out.Jobs, js)
	}
	if len(stuck) > 0 {
		sort.Strings(stuck)
		out.Status = DeepStatusDegraded
		out.Reason = "stuck job lock(s): " + strings.Join(stuck, ", ")
	}
	return out
}

// persistedCheckDetail turns a persisted operational_check_results row into a
// facet: degraded if the job is enabled but the row is missing, not "ok", or
// older than twice its configured interval (never ran, or the scheduler
// stalled). not_configured when the job is disabled.
func persistedCheckDetail(db *gorm.DB, checkName string, enabled bool, intervalHours int) HealthCheckDetail {
	if !enabled {
		return HealthCheckDetail{Status: DeepStatusNotConfigured}
	}
	var row models.OperationalCheckResult
	err := db.Where("check_name = ?", checkName).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "enabled but has never recorded a result"}
	}
	if err != nil {
		logger.Error().Err(err).Str("check", checkName).Msg("deep health: cannot read operational check result")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "cannot read last result"}
	}
	if row.Status != models.OpCheckStatusOK {
		// The row.Detail string can name tables / carry row counts (restore
		// drill) or schema internals (integrity check); it goes to the log and
		// the failure webhook, NOT into this unauthenticated response body.
		logger.Warn().Str("check", checkName).Str("result", row.Status).Str("detail", row.Detail).
			Msg("deep health: last operational check did not pass")
		return HealthCheckDetail{Status: DeepStatusDegraded,
			Reason: "the last run reported " + row.Status}
	}
	staleAfter := time.Duration(intervalHours) * time.Hour * 2
	if staleAfter > 0 && time.Since(row.CheckedAt) > staleAfter {
		return HealthCheckDetail{Status: DeepStatusDegraded,
			Reason: fmt.Sprintf("last ok result is stale (%s ago)", time.Since(row.CheckedAt).Round(time.Minute))}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

// probeServerIntegrations checks only server-scoped outbound dependencies.
// Per-user integrations (Immich/Paperless/Seafile/Nextcloud/ntfy/Gotify) are
// deliberately not iterated here: this endpoint is unauthenticated and
// server-wide, and fanning out N-users x M-integrations of outbound calls on
// it would be an abuse vector. An unreachable dependency is always degraded,
// never unhealthy.
func probeServerIntegrations(cfg config.Config) map[string]HealthCheckDetail {
	out := map[string]HealthCheckDetail{}

	if cfg.OIDC.Enabled && cfg.OIDC.ProviderURL != "" {
		out["oidc"] = probeHTTP("oidc", strings.TrimRight(cfg.OIDC.ProviderURL, "/")+"/.well-known/openid-configuration", 2*time.Second)
	}

	switch {
	case cfg.UseResend:
		out["email"] = probeTCP("email", "api.resend.com:443", 2*time.Second)
	case cfg.UseSMTP:
		out["email"] = probeTCP("email", net.JoinHostPort(cfg.SMTPHost, fmt.Sprint(cfg.SMTPPort)), 2*time.Second)
	}

	if cfg.FCMServiceAccountFile != "" {
		if f, err := os.Open(cfg.FCMServiceAccountFile); err != nil {
			logger.Error().Err(err).Msg("deep health: FCM service-account file not readable")
			out["push_fcm"] = HealthCheckDetail{Status: DeepStatusDegraded,
				Reason: "service-account file not readable"}
		} else {
			_ = f.Close()
			out["push_fcm"] = HealthCheckDetail{Status: DeepStatusOK}
		}
	}

	return out
}

// probeTCP / probeHTTP: the target address and the transport error go to the
// log, never into the response body — an unauthenticated caller must not learn
// the operator's internal SMTP host / OIDC URL or the raw dial error (ASVS
// 7.4.1). The integration key ("email" / "oidc") already says which one.
func probeTCP(name, addr string, timeout time.Duration) HealthCheckDetail {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		logger.Warn().Err(err).Str("integration", name).Str("addr", addr).
			Msg("deep health: integration TCP probe failed")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "unreachable"}
	}
	_ = conn.Close()
	return HealthCheckDetail{Status: DeepStatusOK}
}

func probeHTTP(name, url string, timeout time.Duration) HealthCheckDetail {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Error().Err(err).Str("integration", name).Msg("deep health: bad integration probe URL")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "misconfigured"}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warn().Err(err).Str("integration", name).Msg("deep health: integration HTTP probe failed")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "unreachable"}
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 500 {
		logger.Warn().Str("integration", name).Int("status", resp.StatusCode).
			Msg("deep health: integration HTTP probe returned 5xx")
		return HealthCheckDetail{Status: DeepStatusDegraded, Reason: "upstream error"}
	}
	return HealthCheckDetail{Status: DeepStatusOK}
}

// rollUpDeepStatus: a database read failure dominates (the endpoint 503s); any
// other degraded/unhealthy facet makes the whole thing degraded; otherwise
// healthy.
func rollUpDeepStatus(h DeepHealth) string {
	if h.Database.Status == DeepStatusUnhealthy {
		return DeepStatusUnhealthy
	}
	facets := []string{
		h.Database.Status, h.Migrations.Status,
		h.IntegrityCheck.Status, h.RestoreDrill.Status, h.BackgroundJobs.Status,
	}
	for _, d := range h.Integrations {
		facets = append(facets, d.Status)
	}
	for _, s := range facets {
		if s == DeepStatusDegraded || s == DeepStatusUnhealthy {
			return DeepStatusDegraded
		}
	}
	return "healthy"
}
