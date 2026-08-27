package main

import (
	"context"
	"fmt"
	"mycorrhizal/atrest"
	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/database"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/routes"
	"mycorrhizal/services"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

// runJob executes fn under a per-run correlation ID ("job:<name>:<uuid>") with
// panic recovery and standardized start/finish logging (issue #425). A
// recovered panic is additionally recorded as a job_failed operational event
// so it lands on the admin timeline (issue #424); the ordinary completion is
// a log line only, since the scheduler ticks often and a per-tick row would
// swamp the event stream. "job_completed" here means the wrapper returned
// without panicking — a job that swallows an internal error still counts as
// completed, and that internal failure surfaces as its own subsystem event
// (sync_failed, backup_failed, ...).
func runJob(db *gorm.DB, name string, fn func()) {
	ctx := logger.JobContext(name)
	start := time.Now()
	logger.Ctx(ctx).Info().
		Str(logger.FieldEvent, models.SysEventJobStarted).
		Str(logger.FieldOperation, name).
		Msg("scheduled job started")

	defer func() {
		durMS := time.Since(start).Milliseconds()
		if r := recover(); r != nil {
			logger.Ctx(ctx).Error().
				Str(logger.FieldEvent, models.SysEventJobFailed).
				Str(logger.FieldComponent, logger.ComponentScheduler).
				Str(logger.FieldOperation, name).
				Str(logger.FieldResult, logger.ResultFailure).
				Int64(logger.FieldDurationMS, durMS).
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("scheduled job panicked — recovered")
			models.RecordSystemEvent(ctx, db, models.SystemEvent{
				EventType:  models.SysEventJobFailed,
				Component:  logger.ComponentScheduler,
				Operation:  name,
				Result:     models.SysResult(logger.ResultFailure),
				DurationMS: &durMS,
				Error:      fmt.Sprintf("panic: %v", r),
			})
			return
		}
		logger.Ctx(ctx).Info().
			Str(logger.FieldEvent, models.SysEventJobCompleted).
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Str(logger.FieldOperation, name).
			Str(logger.FieldResult, logger.ResultSuccess).
			Int64(logger.FieldDurationMS, durMS).
			Msg("scheduled job completed")
	}()
	fn()
}

// safeGo runs fn in a goroutine via runJob (panic recovery + correlation ID),
// so an unhandled panic in a background task doesn't crash the server.
func safeGo(db *gorm.DB, name string, fn func()) {
	go runJob(db, name, fn)
}

// recoverJob wraps fn for a recurring gocron registration (s.Every(...).Do(...)).
// gocron invokes the function it's given directly and, in the pinned v1.37.0,
// does not recover a job's own panics unless gocron.SetPanicHandler is set
// globally (it isn't here). Without this, a panic on any scheduled invocation
// would crash the whole process on its next scheduled tick. Always pass the
// wrapped result to .Do(), never fn itself.
func recoverJob(db *gorm.DB, name string, fn func()) func() {
	return func() { runJob(db, name, fn) }
}

func main() {
	// Initialize logger first
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	isPretty := os.Getenv("LOG_PRETTY")
	prettyLog := isPretty == "true" || isPretty == "1"

	// In development, use pretty logs by default
	if os.Getenv("GIN_MODE") != "release" {
		prettyLog = true
	}

	logger.InitLogger(logger.Config{
		Level:  logLevel,
		Pretty: prettyLog,
	})

	logger.Info().Msg("Loading server...")

	logger.Info().Msg("Loading configuration...")
	cfg := config.LoadConfig()

	logger.Info().Msg("Validating configuration...")
	cfg.ValidateOrPanic()

	// M2: config.Validate only checks that FCM_SERVICE_ACCOUNT_FILE exists
	// (config cannot import services without an import cycle). The content
	// check — valid JSON with project_id/client_email/private_key — lives
	// here so a malformed service-account file still fails boot, per the
	// M2 design decision ("reject at boot rather than failing the first
	// send"), instead of only surfacing as a warning on the first reminder run.
	if cfg.FCMServiceAccountFile != "" {
		if _, err := services.LoadFCMServiceAccount(cfg.FCMServiceAccountFile); err != nil {
			logger.Fatal().Err(err).Msg("Invalid FCM service account file")
		}
	}

	logger.Info().Msg("Loading database and running migrations...")
	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize database")
	}
	// T18 audit trail: give the GORM hooks a standalone session for their
	// fire-and-forget audit writes (never the hook's transaction).
	models.RegisterAuditDB(db)

	// Field-level at-rest encryption (issue #380): load the wrapped DEK under
	// the master key, then encrypt any rows written before encryption was
	// enabled (idempotent, row-count-preserving). A wrong DATA_ENCRYPTION_KEY
	// fails closed here — the wrapped DEK cannot be unwrapped and boot aborts
	// before any data is served. See backend/atrest/atrest.go. This must run
	// before the audit hash chain backfill below: the chain hashes the
	// logical (decrypted) audit_events.before_snapshot value via the GORM
	// serializer, so encryption needs to be armed first.
	{
		kek, err := atrest.EncryptionKey()
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to resolve at-rest encryption master key")
		}
		if err := atrest.Initialize(db, kek); err != nil {
			logger.Fatal().Err(err).Msg("Failed to initialize at-rest encryption")
		}
		if err := atrest.Backfill(db); err != nil {
			logger.Fatal().Err(err).Msg("Failed to backfill at-rest encryption")
		}
	}

	// T18 audit hash chain (issue #381): backfill hash/prev_hash for any rows
	// written before migration 000034 and re-link after any purge. Idempotent
	// and write-free once the chain is consistent; failing closed on error
	// keeps the tamper-evidence property from silently degrading.
	if err := models.RecomputeAuditChain(db); err != nil {
		logger.Fatal().Err(err).Msg("Failed to backfill the audit hash chain")
	}

	logger.Info().Msg("Initializing i18n translations...")
	if err := i18n.Init(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize i18n")
	}

	logger.Info().Msg("Running scheduler...")
	// Schedule the reminder task daily
	if !cfg.UseResend {
		logger.Warn().Msg("No Mails to be sent since Resend configuration is not set")
	}
	s := gocron.NewScheduler(cfg.GetReminderLocation())
	task := func() {
		// Use rate-limited version to prevent duplicate emails during rapid restarts
		if err := services.SendRemindersWithRateLimit(db, *cfg); err != nil {
			logger.Error().Err(err).Msg("Error sending reminders")
		}
	}
	s.Every(1).Day().At(cfg.ReminderTime).Do(recoverJob(db, "reminder-scheduled-run", task))
	go safeGo(db, "reminder-initial-run", task)
	s.Every(5).Minutes().Do(recoverJob(db, "webhook-retries-scheduled-run", func() {
		services.ProcessWebhookRetries(db, *cfg)
	}))
	// Sync calendar subscriptions regularly (rate-limited via job lock)
	calendarSyncTask := func() {
		services.SyncCalendarsWithRateLimit(db, *cfg)
	}
	s.Every(cfg.CalDAVSyncIntervalHours).Hours().Do(recoverJob(db, "calendar-sync-scheduled-run", calendarSyncTask))
	go safeGo(db, "calendar-sync-initial-run", calendarSyncTask)

	// Purge soft-deleted rows past their retention window (T26).
	s.Every(24).Hours().Do(recoverJob(db, "purge-scheduled-run", func() {
		services.PurgeDeletedRows(db, *cfg)
	}))
	go safeGo(db, "purge-initial-run", func() {
		services.PurgeDeletedRows(db, *cfg)
	})

	// Purge expired audit events past their retention window (T18).
	s.Every(24).Hours().Do(recoverJob(db, "audit-purge-scheduled-run", func() {
		services.PurgeExpiredAuditEventsScheduled(db, *cfg)
	}))
	go safeGo(db, "audit-purge-initial-run", func() {
		services.PurgeExpiredAuditEventsScheduled(db, *cfg)
	})

	// Purge expired system_events past their retention window (issue #424).
	s.Every(24).Hours().Do(recoverJob(db, "system-event-purge-scheduled-run", func() {
		services.PurgeExpiredSystemEventsScheduled(db, *cfg)
	}))
	go safeGo(db, "system-event-purge-initial-run", func() {
		services.PurgeExpiredSystemEventsScheduled(db, *cfg)
	})

	// Purge expired webhook deliveries past their retention window (issue
	// #622). Job-lock guarded so a multi-instance deploy does not double-purge.
	s.Every(24).Hours().Do(recoverJob(db, "webhook-delivery-purge-scheduled-run", func() {
		services.PurgeExpiredWebhookDeliveriesScheduled(db, *cfg)
	}))
	go safeGo(db, "webhook-delivery-purge-initial-run", func() {
		services.PurgeExpiredWebhookDeliveriesScheduled(db, *cfg)
	})

	// Emit overdue-cadence webhooks daily (T19). Job-lock guarded so a
	// multi-instance deploy does not double-fire.
	s.Every(24).Hours().Do(recoverJob(db, "cadence-overdue-scheduled-run", func() {
		services.ProcessOverdueCadences(db, *cfg)
	}))
	go safeGo(db, "cadence-overdue-initial-run", func() {
		services.ProcessOverdueCadences(db, *cfg)
	})

	// Detect event-driven reach-out suggestions daily (issue #177). Job-lock
	// guarded so a multi-instance deploy does not double-fire.
	s.Every(24).Hours().Do(recoverJob(db, "reach-out-detection-scheduled-run", func() {
		services.DetectReachOutSuggestions(db, *cfg)
	}))
	go safeGo(db, "reach-out-detection-initial-run", func() {
		services.DetectReachOutSuggestions(db, *cfg)
	})

	// Sync Immich enrichment regularly (T16). Job-lock guarded so a
	// multi-instance deploy does not double-sync.
	immichSyncTask := func() {
		services.SyncImmichWithRateLimit(db, *cfg)
	}
	s.Every(cfg.ImmichSyncIntervalHours).Hours().Do(recoverJob(db, "immich-sync-scheduled-run", immichSyncTask))
	go safeGo(db, "immich-sync-initial-run", immichSyncTask)

	// Check the live database for corruption on a schedule (issue #273).
	// Job-lock guarded, config-gated (DB_INTEGRITY_CHECK_ENABLED).
	s.Every(cfg.DBIntegrityCheckIntervalHours).Hours().Do(recoverJob(db, "db-integrity-check-scheduled-run", func() {
		services.CheckDBIntegrityScheduled(db, *cfg)
	}))
	go safeGo(db, "db-integrity-check-initial-run", func() {
		services.CheckDBIntegrityScheduled(db, *cfg)
	})

	// Periodically prove a backup actually restores (issue #275). Job-lock
	// guarded, config-gated (DB_RESTORE_DRILL_ENABLED).
	s.Every(cfg.DBRestoreDrillIntervalHours).Hours().Do(recoverJob(db, "restore-drill-scheduled-run", func() {
		services.RunRestoreDrillScheduled(db, *cfg)
	}))
	go safeGo(db, "restore-drill-initial-run", func() {
		services.RunRestoreDrillScheduled(db, *cfg)
	})

	// Evaluate alert conditions on a schedule (issue #428): detect
	// failure/recovery transitions on the tracked subsystems and notify on
	// them. Job-lock guarded, config-gated (ALERTING_ENABLED).
	s.Every(cfg.AlertEvalIntervalMinutes).Minutes().Do(recoverJob(db, "alert-eval-scheduled-run", func() {
		services.EvaluateAlerts(db, *cfg)
	}))
	go safeGo(db, "alert-eval-initial-run", func() {
		services.EvaluateAlerts(db, *cfg)
	})

	go s.StartBlocking()

	// gin.New() rather than gin.Default(): the app installs its own
	// middleware.LoggingMiddleware() below, which is redaction-aware (query
	// values are allow-listed, see logger.RedactQueryValues). gin.Default()
	// additionally attaches gin's own Logger(), which writes a second,
	// unredacted request line ("GET /api/v1/contacts?search=<a name>") to
	// stdout — an instance-wide disclosure of the same personal data the
	// custom logger exists to keep out (issue #510). Recovery() is kept.
	r := gin.New()
	r.Use(gin.Recovery())

	// Limit multipart form memory to 10MB to prevent DoS via large request bodies
	r.MaxMultipartMemory = 10 << 20 // 10 MB

	// For production, set FRONTEND_URL to specific origin(s) like "https://yourdomain.com"
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Depth", "If-Match", "If-None-Match"},
		ExposeHeaders:    []string{"Content-Length", "ETag"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour, // Cache preflight for 12 hours
	}

	// Handle wildcard "*" for development: allow any origin
	if cfg.FrontendURL == "*" {
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return true // Allow all origins in development
		}
	} else {
		// Production: allow specific origin(s)
		corsConfig.AllowOrigins = []string{cfg.FrontendURL}
	}

	r.Use(cors.New(corsConfig))

	// Add security headers (HSTS only when serving over HTTPS, signaled by COOKIE_SECURE)
	r.Use(middleware.SecurityHeadersMiddleware(cfg.CookieSecure))

	// Apply the configured general-API rate limits before any route is
	// registered (the middleware package installs safe defaults in its own
	// init(); this only replaces them when the environment sets them).
	middleware.ConfigureAPIRateLimiter(cfg.APIRateLimitInterval, cfg.APIRateLimitBurst)

	// Add request body size limit middleware (10MB default) to prevent DoS
	r.Use(middleware.DefaultBodySizeLimitMiddleware())

	// Add request ID middleware for tracing
	r.Use(middleware.RequestIDMiddleware())

	// Add logging middleware (after request ID)
	r.Use(middleware.LoggingMiddleware())

	// Add error handling middleware
	r.Use(apperrors.ErrorHandlerMiddleware())

	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Fatal().Err(err).Strs("proxies", cfg.TrustedProxies).Msg("Failed to set trusted proxies")
	}

	// Inject db and cfg into context
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})

	// Initialize OIDC provider if configured
	var oidcProvider *services.OIDCProvider
	if cfg.OIDC.Enabled {
		logger.Info().Str("provider", cfg.OIDC.ProviderURL).Msg("Initializing OIDC provider...")
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var oidcErr error
		oidcProvider, oidcErr = services.InitOIDCProvider(oidcCtx, cfg)
		oidcCancel()
		if oidcErr != nil {
			logger.Fatal().Err(oidcErr).Msg("Failed to initialize OIDC provider")
		}
		logger.Info().Msg("OIDC provider initialized successfully")
	}

	// Register all routes from routes.go
	routes.RegisterRoutes(r, cfg, db, oidcProvider)

	// Create HTTP server with timeout configuration
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	logger.Info().
		Str("port", cfg.Port).
		Int("read_timeout", cfg.ReadTimeout).
		Int("write_timeout", cfg.WriteTimeout).
		Int("idle_timeout", cfg.IdleTimeout).
		Msg("Starting server")

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to run server")
		}
	}()

	logger.Info().Msg("Server is ready to handle requests")
	models.RecordSystemEvent(context.Background(), db, models.SystemEvent{
		EventType: models.SysEventApplicationStarted,
		Component: logger.ComponentApp,
		Detail:    "version=" + buildinfo.Get().Version,
	})

	// Block until we receive a shutdown signal
	<-quit
	logger.Info().Msg("Shutting down server...")
	models.RecordSystemEvent(context.Background(), db, models.SystemEvent{
		EventType: models.SysEventApplicationStopped,
		Component: logger.ComponentApp,
	})

	// Stop the scheduler first to prevent new jobs from starting
	logger.Info().Msg("Stopping scheduler...")
	s.Stop()

	// Stop the rate limiter's stale-entry sweeper. Its counterpart
	// StartCleanupRoutine runs from the middleware package's init(), so
	// without this the goroutine (and the cleanupDone channel that exists
	// solely to signal it) had no caller at all -- the shutdown hook was
	// written but never wired up.
	middleware.StopCleanupRoutine()

	// Create a deadline to wait for active requests to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown of HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Server forced to shutdown")
	}

	// Close database connection
	logger.Info().Msg("Closing database connection...")
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Close(); err != nil {
			logger.Error().Err(err).Msg("Error closing database connection")
		}
	}

	logger.Info().Msg("Server exited gracefully")
}
