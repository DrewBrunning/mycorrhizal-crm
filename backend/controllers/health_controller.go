package controllers

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/database"
	"mycorrhizal/logger"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The health surface is three endpoints, each answering a different operator
// question (issue #421):
//
//   - GET /health/live  — liveness. Is the process up? Answers instantly,
//     touches nothing. This is what a restart policy hits; a slow or failing
//     dependency must never make it fail, or the orchestrator restarts a
//     healthy app.
//   - GET /health/ready — readiness. Can THIS instance serve? Checks DB
//     connectivity, migration state, and required filesystem access. 503 while
//     any of those is not satisfied. This is what a load balancer gates traffic
//     on.
//   - GET /health       — deep health. Is the CRM actually operational?
//     Everything /ready checks, plus persisted integrity-check / restore-drill
//     outcomes, background-job locks, and server-scoped integration
//     reachability. Reports healthy | degraded | unhealthy; only a database
//     read failure yields 503. "degraded" (an optional integration is down, a
//     scheduled job is stale) is still 200 — degraded-but-alive is not down.
//
// All three are unauthenticated and carry no secrets, matching the original
// single /health.

// HealthResponse is the deep GET /health body. The flat database/version
// fields are retained for backward compatibility with the pre-split endpoint;
// the per-facet breakdown is under checks.
type HealthResponse struct {
	Status    string              `json:"status"` // healthy | degraded | unhealthy
	Timestamp string              `json:"timestamp"`
	Database  DatabaseHealth      `json:"database"`
	Version   string              `json:"version"`
	Commit    string              `json:"commit,omitempty"`
	BuildDate string              `json:"build_date,omitempty"`
	Checks    services.DeepHealth `json:"checks"`
}

// DatabaseHealth represents the database health status
type DatabaseHealth struct {
	Status       string  `json:"status"`
	ResponseTime float64 `json:"response_time_ms"`
}

// LivenessResponse is the GET /health/live body.
type LivenessResponse struct {
	Status string `json:"status"` // always "live"
}

// ReadinessResponse is the GET /health/ready body.
type ReadinessResponse struct {
	Status string                          `json:"status"` // ready | not_ready
	Checks map[string]ReadinessCheckDetail `json:"checks"`
}

// ReadinessCheckDetail is one readiness facet.
type ReadinessCheckDetail struct {
	Status string `json:"status"` // ok | failed
	Reason string `json:"reason,omitempty"`
}

// LivenessCheck handles GET /health/live. It must not touch the database, the
// filesystem, or config — only that the process is running and serving.
func LivenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, LivenessResponse{Status: "live"})
}

// ReadinessCheck handles GET /health/ready.
func ReadinessCheck(c *gin.Context) {
	checks := map[string]ReadinessCheckDetail{
		"database":   readinessDatabase(c),
		"migrations": readinessMigrations(c),
		"filesystem": readinessFilesystem(c),
	}

	status := "ready"
	httpStatus := http.StatusOK
	for _, ck := range checks {
		if ck.Status != "ok" {
			status = "not_ready"
			httpStatus = http.StatusServiceUnavailable
			break
		}
	}

	c.JSON(httpStatus, ReadinessResponse{Status: status, Checks: checks})
}

func readinessDatabase(c *gin.Context) ReadinessCheckDetail {
	db, ok := dbFromContext(c)
	if !ok {
		return ReadinessCheckDetail{Status: "failed", Reason: "no database handle"}
	}
	h := checkDatabaseHealth(db)
	if h.Status != "healthy" {
		return ReadinessCheckDetail{Status: "failed", Reason: "database is unreachable"}
	}
	return ReadinessCheckDetail{Status: "ok"}
}

func readinessMigrations(c *gin.Context) ReadinessCheckDetail {
	db, ok := dbFromContext(c)
	if !ok {
		return ReadinessCheckDetail{Status: "failed", Reason: "no database handle"}
	}
	applied, dirty, ok, err := database.AppliedMigrationVersion(db)
	if err != nil {
		logger.Error().Err(err).Msg("readiness: cannot read migration state")
		return ReadinessCheckDetail{Status: "failed", Reason: "cannot read migration state"}
	}
	if !ok {
		return ReadinessCheckDetail{Status: "failed", Reason: "no migrations have been applied"}
	}
	if dirty {
		return ReadinessCheckDetail{Status: "failed", Reason: "migration is in a dirty state"}
	}
	latest, err := database.LatestMigrationVersion()
	if err != nil {
		logger.Error().Err(err).Msg("readiness: cannot resolve latest migration version")
		return ReadinessCheckDetail{Status: "failed", Reason: "cannot resolve latest migration version"}
	}
	if applied < latest {
		return ReadinessCheckDetail{Status: "failed", Reason: "pending migrations (schema is behind the binary)"}
	}
	return ReadinessCheckDetail{Status: "ok"}
}

func readinessFilesystem(c *gin.Context) ReadinessCheckDetail {
	cfg := currentConfig(c)
	for label, dir := range map[string]string{
		"profile photo directory": cfg.ProfilePhotoDir,
		"attachments directory":   cfg.AttachmentsDir,
	} {
		if dir == "" {
			continue
		}
		if reason := probeWritableDir(dir); reason != "" {
			// The absolute path and errno go to the log, not the
			// unauthenticated response body (ASVS 7.4.1).
			logger.Error().Str("dir", dir).Msg("readiness: " + label + " " + reason)
			return ReadinessCheckDetail{Status: "failed", Reason: label + " " + reason}
		}
	}
	return ReadinessCheckDetail{Status: "ok"}
}

// probeWritableDir returns "" when dir exists, is a directory, and a file can
// be created in it; otherwise a generic, path-free reason (the detail is
// logged by the caller).
func probeWritableDir(dir string) string {
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

// HealthCheck handles the deep health check endpoint, GET /health.
func HealthCheck(c *gin.Context) {
	db, _ := dbFromContext(c)
	cfg := currentConfig(c)

	dbHealth := DatabaseHealth{Status: "unhealthy"}
	var deep services.DeepHealth
	if db != nil {
		dbHealth = checkDatabaseHealth(db)
		deep = services.DeepHealthSnapshot(db, cfg)
	} else {
		deep.Status = services.DeepStatusUnhealthy
		deep.Database = services.HealthCheckDetail{Status: services.DeepStatusUnhealthy, Reason: "no database handle"}
	}

	httpStatus := http.StatusOK
	if deep.Status == services.DeepStatusUnhealthy {
		httpStatus = http.StatusServiceUnavailable
	}

	build := buildinfo.Get()
	c.JSON(httpStatus, HealthResponse{
		Status:    deep.Status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Database:  dbHealth,
		Version:   build.Version,
		Commit:    build.Commit,
		BuildDate: build.BuildDate,
		Checks:    deep,
	})
}

// dbFromContext safely pulls the *gorm.DB the middleware injects, without the
// panic c.MustGet would raise on a misconfigured router — a health endpoint
// should report "unavailable", not 500.
func dbFromContext(c *gin.Context) (*gorm.DB, bool) {
	v, exists := c.Get("db")
	if !exists {
		return nil, false
	}
	db, ok := v.(*gorm.DB)
	return db, ok
}

// checkDatabaseHealth checks if the database is accessible and responsive
func checkDatabaseHealth(db *gorm.DB) DatabaseHealth {
	start := time.Now()

	sqlDB, err := db.DB()
	if err != nil {
		return DatabaseHealth{
			Status:       "unhealthy",
			ResponseTime: 0,
		}
	}

	// Ping the database
	err = sqlDB.Ping()
	duration := time.Since(start).Milliseconds()

	if err != nil {
		return DatabaseHealth{
			Status:       "unhealthy",
			ResponseTime: float64(duration),
		}
	}

	return DatabaseHealth{
		Status:       "healthy",
		ResponseTime: float64(duration),
	}
}
