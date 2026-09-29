// Package embedded exposes a library entry point for the Mycorrhizal backend
// (issue #1257). main() is a thin wrapper around it, and a host process — the
// Android app's embedded local server (ADR 0028) — can start the same server
// in-process with a programmatic config and a caller-supplied listener.
//
// Start does everything the old main() did, in the same order: config
// validation, the at-rest/NFC/audit-chain boot passes, scheduler registration
// and its catch-up triggers, router construction, and listening. Stop reverses
// it: scheduler stop, rate-limiter sweeper stop, graceful HTTP shutdown, and
// database close. The embedded deployment mode additionally provisions a
// single local user and gates the network-only surfaces (see the
// config.Config.Deployment docs and routes.RegisterRoutes).
package embedded

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"mycorrhizal/atrest"
	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/database"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/i18n"
	"mycorrhizal/internal/fsguard"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/routes"
	"mycorrhizal/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

// Options configures Start.
type Options struct {
	// Listener is the net.Listener the HTTP server serves on. In embedded mode
	// it is required (ADR 0028: a Unix domain socket in app-private storage,
	// never TCP loopback). In server mode, nil means listen on :${PORT}.
	Listener net.Listener

	// CatchUpDelay, when > 0 and the deployment is embedded, defers the
	// boot-time Initial catch-up triggers by this long so the first request is
	// not queued behind ~16 jobs contending for the SQLite write lock (spike
	// #1256 measured 995 ms → 748 ms cold start with the triggers deferred).
	// ADR 0011 catch-up semantics are unchanged — only the start time moves.
	// Server mode always runs them immediately, as it does today; 0 also means
	// "immediately" in embedded mode (tests).
	CatchUpDelay time.Duration

	// disableInitialTriggers is a test-only seam: it suppresses the boot-time
	// catch-up goroutines so a lifecycle test can Stop without racing jobs
	// against the closed database. It is unexported deliberately — no
	// production caller can set it, so the production path always dispatches.
	disableInitialTriggers bool
}

// Server is a running backend instance returned by Start.
type Server struct {
	cfg    *config.Config
	db     *gorm.DB
	sched  *gocron.Scheduler
	http   *http.Server
	ln     net.Listener
	token  string
	userID uint
}

// Start boots the server described by cfg and returns after it is listening.
// It mirrors main()'s old startup order exactly; the only behavioral
// differences are opt-in (embedded mode) and the fact that a failure is a
// returned error rather than a log.Fatal.
func Start(ctx context.Context, cfg *config.Config, opts Options) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("embedded: nil config")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("embedded: start aborted before boot: %w", err)
	}

	// Embedded mode serves only the caller's listener. Validate it before any
	// work at all: the later check happens after the scheduler and the
	// boot-time catch-up goroutines have started, and a failed Start there
	// leaked those goroutines (they kept reading the global logger while a
	// later Start re-initialized it — a data race under -race).
	if cfg.IsEmbedded() && opts.Listener == nil {
		return nil, errors.New("embedded: Options.Listener is required (embedded serves only the caller's listener)")
	}

	logger.InitLogger(logger.Config{Level: cfg.LogLevel, Pretty: cfg.LogPretty})
	logger.Info().Msg("Loading server...")

	logger.Info().Msg("Validating configuration...")
	if err := cfg.ValidateError(); err != nil {
		// Emit the operator-facing block before returning, so a library caller
		// that just logs the error still sees the named fields.
		for _, line := range splitLines(err.Error()) {
			logger.Error().Msg(line)
		}
		return nil, err
	}
	// The success counterpart of the block above. The deploy-smoke ordering
	// assertion greps the boot log for this exact phrase to prove config
	// validation precedes the migration run and the listener; the old
	// ValidateOrPanic path printed it, and the #1257 refactor onto
	// ValidateError dropped it (caught by deploy-smoke.yml).
	logger.Info().Msg("Configuration validated successfully")

	// Issue #936: config.Config is the single source of truth for the
	// profile-photo directory.
	models.DefaultPhotoDir = cfg.ProfilePhotoDir

	// Issue #954 / #951: advisory boot warnings (advisory, never fatal).
	for _, warning := range cfg.TrustedProxyWarnings() {
		logger.Warn().Msg(warning)
	}
	for _, warning := range cfg.PublicExposureWarnings() {
		logger.Warn().Msg(warning)
	}

	// ADR 0034 (issue #1293): native Android passkeys are an optional capability,
	// so a switch that is on but cannot take effect is logged once and left off —
	// never fatal.
	if cfg.WebAuthnAndroidEnabled && !cfg.IsEmbedded() {
		if st := cfg.AndroidPasskeys(); st.Effective {
			logger.Info().Int("fingerprints", len(st.Fingerprints)).Msg("Native Android passkeys enabled (serving /.well-known/assetlinks.json)")
		} else {
			logger.Error().Msg("WEBAUTHN_ANDROID_ENABLED is set but native Android passkeys stay OFF: " + st.Reason)
		}
	}

	// M2: config.Validate only checks the FCM file exists; the content check
	// lives here so a malformed service-account file still fails boot.
	if cfg.FCMServiceAccountFile != "" {
		if _, err := services.LoadFCMServiceAccount(cfg.FCMServiceAccountFile); err != nil {
			return nil, fmt.Errorf("invalid FCM service account file: %w", err)
		}
	}

	logger.Info().Msg("Loading database and running migrations...")
	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// From here on, any failure must close the DB we just opened.
	fail := func(err error) (*Server, error) {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}

	// COMPAT-01 (issue #472): advisory WAL-on-network-filesystem warning.
	if warning := fsguard.NetworkFilesystemWarning(cfg.DBPath); warning != nil {
		logger.Warn().
			Str("path", warning.Path).
			Str("filesystem", warning.FilesystemName).
			Msg(warning.String())
	}

	models.RegisterAuditDB(db)

	// Field-level at-rest encryption (issue #380). In embedded mode the host
	// supplies the master key through Config, so the HKDF-from-JWT fallback is
	// deliberately disabled (passing an empty jwtSecret) rather than silently
	// deriving a key the host did not choose.
	{
		jwtFallback := cfg.JWTSecretKey
		if cfg.IsEmbedded() {
			jwtFallback = ""
		}
		kek, err := atrest.ResolveMasterKey(cfg.DataEncryptionKey, cfg.DataEncryptionKeyFile, jwtFallback)
		if err != nil {
			return fail(fmt.Errorf("resolving at-rest encryption master key: %w", err))
		}
		if err := atrest.Initialize(db, kek); err != nil {
			return fail(fmt.Errorf("initializing at-rest encryption: %w", err))
		}
		if err := atrest.Backfill(db); err != nil {
			return fail(fmt.Errorf("backfilling at-rest encryption: %w", err))
		}
	}

	// I18N-02 Unicode NFC normalization (issue #485).
	if _, err := services.NormalizeContactRecordsToNFC(db); err != nil {
		return fail(fmt.Errorf("backfilling Unicode NFC normalization: %w", err))
	}

	// T18 audit hash chain (issue #381).
	if err := models.RecomputeAuditChain(db); err != nil {
		return fail(fmt.Errorf("backfilling the audit hash chain: %w", err))
	}

	logger.Info().Msg("Initializing i18n translations...")
	if err := i18n.Init(); err != nil {
		return fail(fmt.Errorf("initializing i18n: %w", err))
	}

	logger.Info().Msg("Running scheduler...")
	if !cfg.UseResend {
		logger.Warn().Msg("No Mails to be sent since Resend configuration is not set")
	}
	sched := gocron.NewScheduler(cfg.GetReminderLocation())

	if err := registerScheduledJobs(sched, db, cfg); err != nil {
		return fail(fmt.Errorf("registering scheduled jobs: %w", err))
	}

	// Boot-time "Initial" triggers (ADR 0011 catch-up). In embedded mode these
	// may be deferred so the first request is not stuck behind them (spike
	// #1256).
	dispatchInitial := func() { dispatchInitialTriggers(db, cfg) }
	switch {
	case opts.disableInitialTriggers: // test-only seam; no production caller sets it
	case cfg.IsEmbedded() && opts.CatchUpDelay > 0:
		time.AfterFunc(opts.CatchUpDelay, dispatchInitial)
	default:
		dispatchInitial()
	}

	go sched.StartBlocking()

	// Re-arm the rate limiter's stale-entry sweeper: its StartCleanupRoutine
	// counterpart runs from middleware's init(), but Stop tears it down, so a
	// stop/start cycle in one process (the embedded lifecycle) must restart it.
	middleware.StartCleanupRoutine()

	router, _, err := buildRouter(cfg, db)
	if err != nil {
		sched.Stop()
		return fail(err)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	listener := opts.Listener
	if listener == nil {
		if cfg.IsEmbedded() {
			sched.Stop()
			return fail(errors.New("embedded: Options.Listener is required (embedded serves only the caller's listener)"))
		}
		listener, err = net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
		if err != nil {
			sched.Stop()
			return fail(fmt.Errorf("listening on :%s: %w", cfg.Port, err))
		}
	}

	s := &Server{cfg: cfg, db: db, sched: sched, http: srv, ln: listener}

	if cfg.IsEmbedded() {
		if err := s.provisionLocalUser(); err != nil {
			sched.Stop()
			_ = listener.Close()
			return fail(fmt.Errorf("provisioning local user: %w", err))
		}
	}

	logger.Info().
		Str("port", cfg.Port).
		Int("read_timeout", cfg.ReadTimeout).
		Int("write_timeout", cfg.WriteTimeout).
		Int("idle_timeout", cfg.IdleTimeout).
		Msg("Starting server")

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error().Err(serveErr).Msg("Failed to run server")
		}
	}()

	logger.Info().Msg("Server is ready to handle requests")
	models.RecordSystemEvent(context.Background(), db, models.SystemEvent{
		EventType: models.SysEventApplicationStarted,
		Component: logger.ComponentApp,
		Detail:    "version=" + buildinfo.Get().Version,
	})

	return s, nil
}

// Stop shuts the server down: the scheduler and rate-limiter sweeper stop, the
// HTTP server is drained gracefully, and the database is closed. The scheduler
// is stopped first so no new job starts while requests are draining.
func (s *Server) Stop(ctx context.Context) error {
	models.RecordSystemEvent(context.Background(), s.db, models.SystemEvent{
		EventType: models.SysEventApplicationStopped,
		Component: logger.ComponentApp,
	})

	logger.Info().Msg("Stopping scheduler...")
	s.sched.Stop()

	middleware.StopCleanupRoutine()

	logger.Info().Msg("Shutting down server...")
	shutdownErr := s.http.Shutdown(ctx)
	if shutdownErr != nil {
		logger.Error().Err(shutdownErr).Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Closing database connection...")
	if sqlDB, err := s.db.DB(); err == nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			logger.Error().Err(closeErr).Msg("Error closing database connection")
			if shutdownErr == nil {
				shutdownErr = closeErr
			}
		}
	}

	logger.Info().Msg("Server exited gracefully")
	return shutdownErr
}

// Jobs returns the scheduler's registered jobs. A test uses it to assert that
// the embedded-mode gating actually left the disabled jobs unregistered.
func (s *Server) Jobs() []*gocron.Job { return s.sched.Jobs() }

// DB returns the live database handle.
func (s *Server) DB() *gorm.DB { return s.db }

// LocalSessionToken returns the JWT minted for the embedded single user. It is
// empty in server mode. The host hands it to its own client as the local
// profile's credential (ADR 0028 Decision 2).
func (s *Server) LocalSessionToken() string { return s.token }

// LocalUserID returns the embedded single user's id (0 in server mode).
func (s *Server) LocalUserID() uint { return s.userID }

// Addr returns the address the server is listening on.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// provisionLocalUser creates exactly one user on first start (a random,
// unrecoverable password — there is no login surface to use it), reuses it on
// later starts, and mints a session JWT for it. The host's client presents
// that token like any other session token.
func (s *Server) provisionLocalUser() error {
	var count int64
	if err := s.db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}

	var user models.User
	if count == 0 {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		hashed, err := services.HashPassword(base64.RawURLEncoding.EncodeToString(raw))
		if err != nil {
			return err
		}
		user = models.User{
			Username: "local",
			Email:    "local@embedded.invalid",
			Password: hashed,
			Language: "en",
			IsAdmin:  true,
		}
		if err := s.db.Create(&user).Error; err != nil {
			return err
		}
		models.RecordAuditEvent(models.AuditEntityUser, fmt.Sprintf("%d", user.ID), models.AuditOpRegister, user.ID)
		if err := services.EnsureSelfContact(s.db, &user); err != nil {
			return err
		}
	} else {
		if err := s.db.Order("id").First(&user).Error; err != nil {
			return err
		}
	}

	token, err := services.IssueSession(s.db, user, s.cfg, "embedded", "")
	if err != nil {
		return err
	}
	s.token = token
	s.userID = user.ID
	return nil
}

// buildRouter constructs the gin engine exactly as main() did: no default
// logger (the redaction-aware middleware replaces it), the security/body/rate
// limiters, the db/cfg injector, the optional OIDC provider, and the route
// table. Embedded mode skips CORS entirely (there is no browser origin).
func buildRouter(cfg *config.Config, db *gorm.DB) (*gin.Engine, *services.OIDCProvider, error) {
	// gin.New() rather than gin.Default(): the app installs its own
	// middleware.LoggingMiddleware() below, which is redaction-aware (query
	// values are allow-listed, see logger.RedactQueryValues). gin.Default()
	// additionally attaches gin's own Logger(), which writes a second,
	// unredacted request line ("GET /api/v1/contacts?search=<a name>") to
	// stdout — an instance-wide disclosure of the same personal data the
	// custom logger exists to keep out (issue #510). Recovery() is kept.
	router := gin.New()
	router.Use(gin.Recovery())

	router.MaxMultipartMemory = 10 << 20 // 10 MB

	if !cfg.IsEmbedded() {
		corsConfig := cors.Config{
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Depth", "If-Match", "If-None-Match"},
			ExposeHeaders:    []string{"Content-Length", "ETag"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}
		if cfg.FrontendURL == "*" {
			corsConfig.AllowOriginFunc = func(origin string) bool { return true }
		} else {
			corsConfig.AllowOrigins = []string{cfg.FrontendURL}
		}
		router.Use(cors.New(corsConfig))
	}

	router.Use(middleware.SecurityHeadersMiddleware(cfg.CookieSecure))
	middleware.ConfigureAPIRateLimiter(cfg.APIRateLimitInterval, cfg.APIRateLimitBurst)

	authSprayHold := 2 * time.Duration(cfg.AlertEvalIntervalMinutes) * time.Minute
	if minHold := time.Duration(cfg.AuthSprayThrottleSeconds) * time.Second; minHold > authSprayHold {
		authSprayHold = minHold
	}
	middleware.ConfigureAuthVelocity(middleware.AuthVelocityConfig{
		Enabled:             cfg.AuthSprayEnabled,
		Window:              time.Duration(cfg.AuthSprayWindowSeconds) * time.Second,
		FailureThreshold:    cfg.AuthSprayFailureThreshold,
		IdentifierThreshold: cfg.AuthSprayIdentifierThreshold,
		Throttle:            time.Duration(cfg.AuthSprayThrottleSeconds) * time.Second,
		IncidentHold:        authSprayHold,
	})

	router.Use(middleware.DefaultBodySizeLimitMiddleware())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.LoggingMiddleware())
	router.Use(middleware.MetricsMiddleware())
	router.Use(apperrors.ErrorHandlerMiddleware())

	if err := router.SetTrustedProxies(cfg.EffectiveTrustedProxies()); err != nil {
		return nil, nil, fmt.Errorf("setting trusted proxies: %w", err)
	}

	router.Use(middleware.DeprecationHeaders())

	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})

	var oidcProvider *services.OIDCProvider
	if cfg.OIDC.Enabled {
		logger.Info().Str("provider", cfg.OIDC.ProviderURL).Msg("Initializing OIDC provider...")
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var oidcErr error
		oidcProvider, oidcErr = services.InitOIDCProvider(oidcCtx, cfg)
		oidcCancel()
		if oidcErr != nil {
			return nil, nil, fmt.Errorf("initializing OIDC provider: %w", oidcErr)
		}
		logger.Info().Msg("OIDC provider initialized successfully")
	}

	routes.RegisterRoutes(router, cfg, db, oidcProvider)
	return router, oidcProvider, nil
}

// dispatchInitialTriggers launches one immediate run per job (job-lock
// de-duplicated, ADR 0011) so a process that was down past a job's interval
// catches up instead of waiting a full cycle. The embedded-disabled jobs are
// skipped here too, matching registerScheduledJobs.
func dispatchInitialTriggers(db *gorm.DB, cfg *config.Config) {
	type initial struct {
		jobName string
		run     func()
	}
	reports := []initial{
		{models.JobNameDailyReminders, func() {
			safeGoReport(db, models.JobNameDailyReminders, models.JobTriggerInitial, reminderTask(db, *cfg))
		}},
		{models.JobNameCalendarSync, func() { safeGo(db, models.JobNameCalendarSync, models.JobTriggerInitial, calendarSyncTask(db, *cfg)) }},
		{models.JobNamePurgeDeleted, func() { safeGo(db, models.JobNamePurgeDeleted, models.JobTriggerInitial, purgeDeletedTask(db, *cfg)) }},
		{models.JobNameAuditPurge, func() { safeGo(db, models.JobNameAuditPurge, models.JobTriggerInitial, auditPurgeTask(db, *cfg)) }},
		{models.JobNameSystemEventPurge, func() {
			safeGo(db, models.JobNameSystemEventPurge, models.JobTriggerInitial, systemEventPurgeTask(db, *cfg))
		}},
		{models.JobNameJobRunPurge, func() { safeGo(db, models.JobNameJobRunPurge, models.JobTriggerInitial, jobRunPurgeTask(db, *cfg)) }},
		{models.JobNameWebhookDeliveryPurge, func() {
			safeGo(db, models.JobNameWebhookDeliveryPurge, models.JobTriggerInitial, webhookDeliveryPurgeTask(db, *cfg))
		}},
		{models.JobNameIdempotencyKeyPurge, func() {
			safeGo(db, models.JobNameIdempotencyKeyPurge, models.JobTriggerInitial, idempotencyKeyPurgeTask(db, *cfg))
		}},
		{models.JobNameSessionPurge, func() { safeGo(db, models.JobNameSessionPurge, models.JobTriggerInitial, sessionPurgeTask(db)) }},
		{models.JobNameCadenceOverdue, func() {
			safeGoReport(db, models.JobNameCadenceOverdue, models.JobTriggerInitial, cadenceOverdueTask(db, *cfg))
		}},
		{models.JobNameReachOutDetection, func() {
			safeGoReport(db, models.JobNameReachOutDetection, models.JobTriggerInitial, reachOutTask(db, *cfg))
		}},
		{models.JobNameImmichSync, func() { safeGo(db, models.JobNameImmichSync, models.JobTriggerInitial, immichSyncTask(db, *cfg)) }},
		{models.JobNameDBIntegrityCheck, func() {
			safeGo(db, models.JobNameDBIntegrityCheck, models.JobTriggerInitial, dbIntegrityTask(db, *cfg))
		}},
		{models.JobNameRestoreDrill, func() { safeGo(db, models.JobNameRestoreDrill, models.JobTriggerInitial, restoreDrillTask(db, *cfg)) }},
		{models.JobNameAlertEval, func() { safeGo(db, models.JobNameAlertEval, models.JobTriggerInitial, alertEvalTask(db, *cfg)) }},
		{models.JobNameStorageSample, func() { safeGo(db, models.JobNameStorageSample, models.JobTriggerInitial, storageSampleTask(db, *cfg)) }},
	}
	for _, in := range reports {
		if cfg.IsEmbedded() && embeddedDisabledJobs[in.jobName] {
			continue
		}
		in.run()
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
