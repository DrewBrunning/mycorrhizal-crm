package controllers

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/metrics"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSystemStatus returns the authenticated, admin-scoped operational snapshot
// (issue #388): rolled-up deep-health status, the full deep-health breakdown,
// build identity, process uptime, migration numbers, a live config-validation
// read-back, the enabled feature-flag set, SQLite operational facts, storage
// sizing, and the opt-in update-availability block (issue #650).
//
// This is the counterpart to the unauthenticated GET /health surface, which
// deliberately withholds build/version, migration numbers and storage facts,
// and to GET /metrics, which is Prometheus-only behind METRICS_TOKEN. Like the
// other /admin diagnostic endpoints it is instance-wide (not user-scoped),
// read-only, and gated by AuthMiddleware + AdminMiddleware.
//
// No secret (JWT secret, metrics token, SMTP host, OIDC client secret) appears
// anywhere in the response: the feature block is booleans only, and
// config.ValidationError carries a field name plus a remediation message, never
// the offending value.
func GetSystemStatus(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)

	deep := services.DeepHealthSnapshot(db, cfg)

	resp := SystemStatusResponse{
		Overall:   deep.Status,
		Health:    deep,
		Version:   buildinfo.Get(),
		Uptime:    systemStatusUptime(),
		Migration: systemStatusMigration(db),
		Config:    systemStatusConfig(cfg),
		Database:  systemStatusDatabase(db, cfg),
		Storage:   systemStatusStorage(cfg),
		Update:    services.BuildUpdateCheckStatus(c.Request.Context(), cfg),
	}

	c.JSON(http.StatusOK, resp)
}

// SystemStatusResponse is the GET /admin/system-status body.
type SystemStatusResponse struct {
	Overall   string                `json:"overall"` // healthy | degraded | unhealthy
	Health    services.DeepHealth   `json:"health"`
	Version   buildinfo.Info        `json:"version"`
	Uptime    SystemStatusUptime    `json:"uptime"`
	Migration SystemStatusMigration `json:"migration"`
	Config    SystemStatusConfig    `json:"config"`
	Database  SystemStatusDatabase  `json:"database"`
	Storage   SystemStatusStorage   `json:"storage"`
	// Update is the opt-in update-availability block (issue #650): enabled is
	// the config flag, and current/latest/update_available/checked_at are only
	// populated when the flag is on and a lookup succeeded. When disabled the
	// block is just {enabled: false} and no outbound call is made.
	Update services.UpdateCheckStatus `json:"update"`
}

// SystemStatusUptime reports when the process started and how long ago.
type SystemStatusUptime struct {
	StartedAt     string `json:"started_at"` // RFC3339
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// SystemStatusMigration is the schema-version picture: how far the database is
// migrated, how far the binary could take it, and whether the last migration
// left the version dirty.
type SystemStatusMigration struct {
	Applied uint `json:"applied"`
	Latest  uint `json:"latest"`
	Pending int  `json:"pending"`
	Dirty   bool `json:"dirty"`
}

// SystemStatusConfig is the live config read-back: what cfg.Validate() reports
// right now, plus the set of optional subsystems currently enabled.
type SystemStatusConfig struct {
	// Validation is never nil — it marshals as [] on a clean boot (frontend
	// trap #8). Each entry is a field name and a remediation message; no
	// entry carries the offending value.
	Validation []SystemStatusConfigValidation `json:"validation"`
	Features   SystemStatusFeatures           `json:"features"`
}

// SystemStatusConfigValidation mirrors config.ValidationError on the wire.
type SystemStatusConfigValidation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// SystemStatusFeatures is the enabled-or-not state of each optional subsystem.
// Booleans only — this is a capability map, never a config dump.
type SystemStatusFeatures struct {
	CardDAV          bool `json:"carddav"`
	CalDAV           bool `json:"caldav"`
	OIDC             bool `json:"oidc"`
	Email            bool `json:"email"`
	Metrics          bool `json:"metrics"`
	DBIntegrityCheck bool `json:"db_integrity_check"`
	DBRestoreDrill   bool `json:"db_restore_drill"`
}

// SystemStatusDatabase carries SQLite operational facts that /health withholds
// from an unauthenticated caller.
type SystemStatusDatabase struct {
	SQLiteVersion string `json:"sqlite_version"`
	JournalMode   string `json:"journal_mode"`
	WALBytes      int64  `json:"wal_bytes"` // size of the -wal sidecar, 0 when absent
}

// SystemStatusStorage is on-disk sizing: the database footprint, the
// filesystem holding it, and each configured storage directory.
type SystemStatusStorage struct {
	DatabaseBytes int64                  `json:"database_bytes"`
	Filesystem    SystemStatusFilesystem `json:"filesystem"`
	// Directories is never nil — it marshals as [] when no storage directory
	// is configured (frontend trap #8).
	Directories []services.DirectoryUsage `json:"directories"`
}

// SystemStatusFilesystem is free/total bytes of the filesystem holding the
// database file.
type SystemStatusFilesystem struct {
	FreeBytes  int64 `json:"free_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

func systemStatusUptime() SystemStatusUptime {
	start := metrics.ProcessStart()
	return SystemStatusUptime{
		StartedAt:     start.UTC().Format(time.RFC3339),
		UptimeSeconds: int64(time.Since(start).Seconds()),
	}
}

func systemStatusMigration(db *gorm.DB) SystemStatusMigration {
	out := SystemStatusMigration{}

	applied, dirty, ok, err := database.AppliedMigrationVersion(db)
	if err == nil && ok {
		out.Applied = applied
		out.Dirty = dirty
	}

	if latest, err := database.LatestMigrationVersion(); err == nil {
		out.Latest = latest
	}

	if out.Latest > out.Applied {
		out.Pending = int(out.Latest - out.Applied)
	}
	return out
}

func systemStatusConfig(cfg config.Config) SystemStatusConfig {
	validation := make([]SystemStatusConfigValidation, 0)
	for _, ve := range cfg.Validate() {
		validation = append(validation, SystemStatusConfigValidation{
			Field:   ve.Field,
			Message: ve.Message,
		})
	}

	return SystemStatusConfig{
		Validation: validation,
		Features: SystemStatusFeatures{
			CardDAV:          cfg.CardDAVEnabled,
			CalDAV:           cfg.CalDAVEnabled,
			OIDC:             cfg.OIDC.Enabled,
			Email:            cfg.EmailEnabled(),
			Metrics:          cfg.MetricsToken != "",
			DBIntegrityCheck: cfg.DBIntegrityCheckEnabled,
			DBRestoreDrill:   cfg.DBRestoreDrillEnabled,
		},
	}
}

func systemStatusDatabase(db *gorm.DB, cfg config.Config) SystemStatusDatabase {
	var out SystemStatusDatabase

	// Best-effort reads: a diagnostic endpoint should degrade to blank fields,
	// not 500, if a PRAGMA momentarily fails.
	_ = db.Raw("SELECT sqlite_version()").Scan(&out.SQLiteVersion).Error
	_ = db.Raw("PRAGMA journal_mode").Scan(&out.JournalMode).Error

	if fi, err := os.Stat(cfg.DBPath + "-wal"); err == nil {
		out.WALBytes = fi.Size()
	}
	return out
}

func systemStatusStorage(cfg config.Config) SystemStatusStorage {
	free, total, ok := metrics.FilesystemBytes(filepath.Dir(cfg.DBPath))
	fs := SystemStatusFilesystem{}
	if ok {
		fs.FreeBytes = int64(free)
		fs.TotalBytes = int64(total)
	}

	dirs := services.StorageUsage([]string{cfg.ProfilePhotoDir, cfg.AttachmentsDir})

	return SystemStatusStorage{
		DatabaseBytes: metrics.DatabaseBytes(cfg.DBPath),
		Filesystem:    fs,
		Directories:   dirs,
	}
}
