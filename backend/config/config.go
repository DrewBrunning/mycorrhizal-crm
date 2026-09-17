// Package config loads and validates application configuration from
// environment variables, exposing it as a single Config value the rest of
// the backend reads from (server/DB settings, auth secrets, mail/OIDC/
// CardDAV sync options, feature flags).
package config

import (
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/atrest"
)

// OIDCConfig holds optional OIDC provider settings.
type OIDCConfig struct {
	Enabled               bool     `cfgreg:"derived=true;desc=true when OIDC_PROVIDER_URL, OIDC_CLIENT_ID and OIDC_CLIENT_SECRET are all set"`
	ProviderURL           string   `cfgreg:"env=OIDC_PROVIDER_URL;type=string;range=absolute http(s) URL;default=;required=false;restart=true;desc=OIDC provider issuer URL"`
	ClientID              string   `cfgreg:"env=OIDC_CLIENT_ID;type=string;default=;required=false;restart=true;desc=OIDC client ID"`
	ClientSecret          string   `cfgreg:"env=OIDC_CLIENT_SECRET;type=string;default=;required=false;restart=true;desc=OIDC client secret"`
	RedirectURL           string   `cfgreg:"derived=true;desc=derived from FRONTEND_URL, not configurable"` // derived from FrontendURL, not configurable
	AllowAutoProvision    bool     `cfgreg:"env=OIDC_AUTO_PROVISION;type=bool;default=false;required=false;restart=true;desc=Auto-create a local account on first OIDC login"`
	TrustEmail            bool     `cfgreg:"env=OIDC_TRUST_EMAIL;type=bool;default=false;required=false;restart=true;desc=Skip email_verified requirement when linking accounts"` // skip email_verified requirement when linking accounts (for trusted self-hosted providers)
	Scopes                []string `cfgreg:"env=OIDC_SCOPES;type=stringlist;default=openid,email,profile;required=false;restart=true;desc=Comma-separated OIDC scopes"`
	PostLogoutRedirectURL string   `cfgreg:"derived=true;desc=derived from FRONTEND_URL, not configurable"` // derived from FrontendURL, not configurable — see RedirectURL
	// BlockPrivateURLs routes the discovery/token/JWKS/UserInfo calls through
	// the SSRF-guarded dialer (httputil.SafeDialContext). Default off: a LAN
	// identity provider (Authentik/Keycloak on the same Docker network) is a
	// common self-hosted setup and must keep working. Same opt-in shape as the
	// other *_BLOCK_PRIVATE_URLS knobs (INT-02, issue #465).
	BlockPrivateURLs bool `cfgreg:"env=OIDC_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Route OIDC discovery/token/JWKS/UserInfo calls through the SSRF-guarded dialer"`
}

// Config is the fully-loaded application configuration, populated once by
// LoadConfig from environment variables at process start.
type Config struct {
	DBPath string `cfgreg:"env=SQLITE_DB_PATH;type=string;default=mycorrhizal.db;required=true;restart=true;desc=SQLite database file path"`
	// ReminderTime and ReminderTimezone are the operator's single reminder
	// clock: REMINDER_TIME is a local wall time (HH:MM) interpreted in
	// REMINDER_TIMEZONE (IANA), and together they are server-wide — every
	// user on the deployment is scheduled against this one clock, never a
	// per-user zone (docs/adrs/0015-temporal-semantics.md "local wall time"
	// category). See GetReminderLocation.
	ReminderTime                 string   `cfgreg:"env=REMINDER_TIME;type=string;range=HH:MM 24h wall time;default=06:00;required=false;restart=true;desc=Daily reminder wall-clock time"`
	ReminderTimezone             string   `cfgreg:"env=REMINDER_TIMEZONE;type=string;range=IANA timezone name;default=UTC;required=false;restart=true;desc=Reminder clock's IANA timezone"`
	FrontendURL                  string   `cfgreg:"env=FRONTEND_URL;type=string;range=absolute origin, or * for dev only;default=*;required=false;restart=true;desc=Frontend origin used for CORS and OIDC redirect URLs"`
	Port                         string   `cfgreg:"env=PORT;type=int;range=1..65535;default=8080;required=false;restart=true;desc=HTTP listen port"`
	TrustedProxies               []string `cfgreg:"env=TRUSTED_PROXIES;type=stringlist;range=IP or CIDR, no 0.0.0.0/0 or ::/0;default=(loopback 127.0.0.1/32, ::1/128);required=false;restart=true;desc=Reverse-proxy addresses trusted for X-Forwarded-For"`
	UseResend                    bool     `cfgreg:"derived=true;desc=true when RESEND_API_KEY and RESEND_FROM_EMAIL are both set"`
	ResendAPIKey                 string   `cfgreg:"env=RESEND_API_KEY;type=string;default=;required=false;restart=true;desc=Resend email API key"`
	ResendFromEmail              string   `cfgreg:"env=RESEND_FROM_EMAIL;type=string;default=;required=false;restart=true;desc=Resend sender address"`
	UseSMTP                      bool     `cfgreg:"derived=true;desc=true when SMTP_HOST and SMTP_FROM_EMAIL are both set"`
	SMTPHost                     string   `cfgreg:"env=SMTP_HOST;type=string;default=;required=false;restart=true;desc=SMTP server hostname"`
	SMTPPort                     int      `cfgreg:"env=SMTP_PORT;type=int;range=1..65535 when SMTP enabled;default=587;required=false;restart=true;desc=SMTP server port"`
	SMTPUsername                 string   `cfgreg:"env=SMTP_USERNAME;type=string;default=;required=false;restart=true;desc=SMTP auth username"`
	SMTPPassword                 string   `cfgreg:"env=SMTP_PASSWORD;type=string;default=;required=false;restart=true;desc=SMTP auth password"`
	SMTPFromEmail                string   `cfgreg:"env=SMTP_FROM_EMAIL;type=string;default=;required=false;restart=true;desc=SMTP sender address"`
	SMTPUseTLS                   bool     `cfgreg:"env=SMTP_USE_TLS;type=bool;default=false;required=false;restart=true;desc=Use implicit TLS (e.g. port 465) instead of STARTTLS"` // implicit TLS (e.g. port 465); otherwise STARTTLS is used when available
	JWTSecretKey                 string   `cfgreg:"env=JWT_SECRET_KEY;type=string;range=>=32 bytes, not a known placeholder, sufficient entropy;default=;required=true;restart=true;desc=Secret key signing auth JWTs"`
	JWTExpiryHours               int      `cfgreg:"env=JWT_EXPIRY_HOURS;type=int;range=1..8760;default=96;required=false;restart=true;desc=JWT absolute expiry, in hours"`
	ReadTimeout                  int      `cfgreg:"env=HTTP_READ_TIMEOUT;type=int;range=1..300;default=15;required=false;restart=true;desc=HTTP server read timeout, in seconds"`                                                // HTTP server read timeout in seconds
	WriteTimeout                 int      `cfgreg:"env=HTTP_WRITE_TIMEOUT;type=int;range=1..300;default=15;required=false;restart=true;desc=HTTP server write timeout, in seconds"`                                              // HTTP server write timeout in seconds
	IdleTimeout                  int      `cfgreg:"env=HTTP_IDLE_TIMEOUT;type=int;range=1..300;default=60;required=false;restart=true;desc=HTTP server idle timeout, in seconds"`                                                // HTTP server idle timeout in seconds
	ProfilePhotoDir              string   `cfgreg:"env=PROFILE_PHOTO_DIR;type=string;range=absolute path;default=;required=true;restart=true;desc=Directory storing profile photos"`                                             // Directory for storing profile photos (must be absolute path)
	AttachmentsDir               string   `cfgreg:"env=ATTACHMENTS_DIR;type=string;range=absolute path;default=(PROFILE_PHOTO_DIR's parent)/attachments;required=false;restart=true;desc=Directory storing contact attachments"` // Directory for storing contact attachments (N7; alongside the photo dir, must be absolute path)
	CardDAVEnabled               bool     `cfgreg:"env=CARDDAV_ENABLED;type=bool;default=false;required=false;restart=true;desc=Enable the CardDAV contact-sync server"`                                                         // Enable CardDAV server for contact sync
	CalDAVEnabled                bool     `cfgreg:"env=CALDAV_ENABLED;type=bool;default=false;required=false;restart=true;desc=Enable the CalDAV interaction/life-event sync server"`                                            // Enable CalDAV server for Interaction/LifeEvent sync (T12b)
	CalDAVTwoWayEnabled          bool     `cfgreg:"env=CALDAV_TWO_WAY_ENABLED;type=bool;default=false;required=false;restart=true;desc=Allow calendar sync to push local edits back out"`                                        // Allow calendar sync to push local edits back out (T13)
	CookieSecure                 bool     `cfgreg:"env=COOKIE_SECURE;type=bool;default=false;required=false;restart=true;desc=Set Secure flag on the auth cookie (requires HTTPS)"`                                              // Set Secure flag on auth cookie (requires HTTPS)
	CookieDomain                 string   `cfgreg:"env=COOKIE_DOMAIN;type=string;range=hostname, optionally dot-prefixed;default=;required=false;restart=true;desc=Domain for the auth cookie"`                                  // Domain for auth cookie (empty = current domain only)
	RegistrationDisabled         bool     `cfgreg:"env=DISABLE_REGISTRATION;type=bool;default=false;required=false;restart=true;desc=Disable new user registration"`
	WebhookBlockPrivateURLs      bool     `cfgreg:"env=WEBHOOK_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block webhook deliveries to private/loopback addresses"`
	CalDAVSyncIntervalHours      int      `cfgreg:"env=CALDAV_SYNC_INTERVAL_HOURS;type=int;range=>=1, invalid value refuses to boot;default=6;required=false;restart=true;desc=Interval for the scheduled calendar sync job, in hours"`
	CalDAVBlockPrivateURLs       bool     `cfgreg:"env=CALDAV_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block calendar sync requests to private/loopback addresses"`
	DeleteRetentionDays          int      `cfgreg:"env=DELETED_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=30;required=false;restart=true;desc=Days soft-deleted rows survive before the purge job hard-deletes them"`
	AuditRetentionDays           int      `cfgreg:"env=AUDIT_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=90;required=false;restart=true;desc=Days audit events survive before the retention purge removes them"`
	ContactShareRetentionDays    int      `cfgreg:"env=CONTACT_SHARE_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=30;required=false;restart=true;desc=Days a ContactShare snapshot survives before the purge job hard-deletes it"`
	SystemEventRetentionDays     int      `cfgreg:"env=SYSTEM_EVENT_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=30;required=false;restart=true;desc=Days system_events rows survive before the retention purge removes them"`
	WebhookDeliveryRetentionDays int      `cfgreg:"env=WEBHOOK_DELIVERY_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=30;required=false;restart=true;desc=Days webhook_deliveries rows survive before the purge job hard-deletes them"`
	JobRunRetentionDays          int      `cfgreg:"env=JOB_RUN_RETENTION_DAYS;type=int;range=>=0, 0 disables the purge;default=30;required=false;restart=true;desc=Days job_runs rows survive before the retention purge removes them"`
	IdempotencyKeyRetentionHours int      `cfgreg:"env=IDEMPOTENCY_KEY_RETENTION_HOURS;type=int;range=any integer, <=0 disables;default=24;required=false;restart=true;desc=Hours idempotency_keys rows survive before the TTL purge removes them"`
	SessionIdleTimeoutHours      int      `cfgreg:"env=SESSION_IDLE_TIMEOUT_HOURS;type=int;range=0 (disabled) or 1..JWT_EXPIRY_HOURS;default=12;required=false;restart=true;desc=Hours a session may sit unused before AuthMiddleware rejects it"`

	// General-API rate limiting, per client IP. Configurable because the
	// hardcoded values had already been raised once to stop a full Playwright
	// run exhausting the bucket, and because a deployment where several
	// people share one egress IP (a household behind NAT, or a reverse proxy
	// without correct X-Forwarded-For) shares a single bucket between them.
	// Defaults preserve the previous hardcoded behaviour exactly.
	APIRateLimitInterval time.Duration `cfgreg:"env=API_RATE_LIMIT_INTERVAL_MS;type=duration;range=>0ms;default=600ms;required=false;restart=true;desc=Sustained rate-limit refill interval (milliseconds)"` // Sustained refill interval, one token per interval
	APIRateLimitBurst    int           `cfgreg:"env=API_RATE_LIMIT_BURST;type=int;range=>=1;default=1000;required=false;restart=true;desc=Rate-limit bucket size, largest instantaneous burst allowed"`      // Bucket size, i.e. the largest instantaneous burst allowed

	// Instance-wide failed-authentication velocity detection (issue #940). The
	// per-identifier and per-IP limits below stop a single-source attack, but a
	// distributed password spray keeps every individual budget under threshold.
	// This measures failures across ALL identifiers in a sliding window and,
	// when both thresholds are crossed, engages a short instance-wide login
	// throttle and raises the auth_spray alert. Zero values are clamped to the
	// safe defaults rather than disabling the signal.
	AuthSprayEnabled             bool `cfgreg:"env=AUTH_SPRAY_ENABLED;type=bool;default=true;required=false;restart=true;desc=Master switch for the instance-wide failed-auth velocity signal"`                                                               // Master switch for the velocity signal + throttle
	AuthSprayWindowSeconds       int  `cfgreg:"env=AUTH_SPRAY_WINDOW_SECONDS;type=int;range=>=1, invalid/low values clamped to 60 with a WARN;default=60;required=false;restart=true;desc=Sliding window velocity is measured over, in seconds"`              // Sliding window velocity is measured over
	AuthSprayFailureThreshold    int  `cfgreg:"env=AUTH_SPRAY_FAILURE_THRESHOLD;type=int;range=>=1, invalid/low values clamped to 60 with a WARN;default=60;required=false;restart=true;desc=Failures within the window that arm the signal"`                 // Failures within the window that arm the signal
	AuthSprayIdentifierThreshold int  `cfgreg:"env=AUTH_SPRAY_IDENTIFIER_THRESHOLD;type=int;range=>=1, invalid/low values clamped to 15 with a WARN;default=15;required=false;restart=true;desc=Distinct identifiers within the window that arm the signal"`  // Distinct identifiers within the window that arm the signal
	AuthSprayThrottleSeconds     int  `cfgreg:"env=AUTH_SPRAY_THROTTLE_SECONDS;type=int;range=>=1, invalid/low values clamped to 300 with a WARN;default=300;required=false;restart=true;desc=How long a tripped signal refuses unknown sources, in seconds"` // How long a tripped signal refuses unknown sources

	ImmichSyncIntervalHours       int    `cfgreg:"env=IMMICH_SYNC_INTERVAL_HOURS;type=int;range=>=1, invalid value refuses to boot;default=6;required=false;restart=true;desc=Interval for the scheduled Immich enrichment sync, in hours"`
	ImmichBlockPrivateURLs        bool   `cfgreg:"env=IMMICH_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block Immich fetches to private/loopback addresses"`
	PaperlessBlockPrivateURLs     bool   `cfgreg:"env=PAPERLESS_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block Paperless-ngx fetches to private/loopback addresses"`
	SeafileBlockPrivateURLs       bool   `cfgreg:"env=SEAFILE_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block Seafile fetches to private/loopback addresses"`
	WebDAVBlockPrivateURLs        bool   `cfgreg:"env=WEBDAV_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block Nextcloud/ownCloud WebDAV fetches to private/loopback addresses"`
	MonicaBlockPrivateURLs        bool   `cfgreg:"env=MONICA_BLOCK_PRIVATE_URLS;type=bool;default=false;required=false;restart=true;desc=Block Monica import-assistant fetches to private/loopback addresses"`
	FCMServiceAccountFile         string `cfgreg:"env=FCM_SERVICE_ACCOUNT_FILE;type=string;range=path to an existing file;default=;required=false;restart=true;desc=Path to the Firebase service-account JSON for FCM mobile push"`
	DBIntegrityCheckEnabled       bool   `cfgreg:"env=DB_INTEGRITY_CHECK_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the scheduled live-DB PRAGMA integrity_check job"`
	DBIntegrityCheckIntervalHours int    `cfgreg:"env=DB_INTEGRITY_CHECK_INTERVAL_HOURS;type=int;range=>=1, invalid value refuses to boot;default=24;required=false;restart=true;desc=Interval for the scheduled DB integrity check, in hours"`
	DBRestoreDrillEnabled         bool   `cfgreg:"env=DB_RESTORE_DRILL_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the scheduled backup-restore drill job"`
	DBRestoreDrillIntervalHours   int    `cfgreg:"env=DB_RESTORE_DRILL_INTERVAL_HOURS;type=int;range=>=1, invalid value refuses to boot;default=168;required=false;restart=true;desc=Interval for the scheduled restore drill, in hours"`
	// DBRestoreDrillMaxDurationSeconds is the operator's RTO budget for the
	// database piece of a restore (issue #506). When > 0, a restore-drill run
	// whose measured wall-clock exceeds it still passes but logs a WARN and
	// annotates the restore_test_completed timeline row — restore-time drift
	// visible before an incident, not during one. 0 (default) = no budget; the
	// drill's duration_ms is recorded on every run regardless.
	DBRestoreDrillMaxDurationSeconds int `cfgreg:"env=DB_RESTORE_DRILL_MAX_DURATION_SECONDS;type=int;range=>=0, invalid value refuses to boot;default=0;required=false;restart=true;desc=RTO budget for the database piece of a restore; 0 means no budget"`

	// Alerting on state transitions (issue #428). The scheduled evaluator
	// (services.EvaluateAlerts) detects failure/recovery transitions on the
	// tracked subsystems (#427) plus a few threshold checks, and dispatches one
	// notification per transition through the existing webhook + notification
	// channels. Personal channels (email/ntfy/Gotify/push) go to admin users
	// only; webhooks broadcast as usual.
	AlertingEnabled             bool `cfgreg:"env=ALERTING_ENABLED;type=bool;default=true;required=false;restart=true;desc=Master switch for the alert evaluator"`                                                                                                                         // Master switch for the alert evaluator
	AlertEvalIntervalMinutes    int  `cfgreg:"env=ALERT_EVAL_INTERVAL_MINUTES;type=int;range=>=1, invalid value refuses to boot;default=15;required=false;restart=true;desc=How often the alert evaluator runs, in minutes"`                                                               // How often the evaluator runs
	AlertDiskUsagePercent       int  `cfgreg:"env=ALERT_DISK_USAGE_PERCENT;type=int;range=0..99, invalid value refuses to boot;default=90;required=false;restart=true;desc=Raise disk_space when used% >= this; 0 disables the condition"`                                                 // Raise disk_space when used% >= this; 0 disables the condition
	AlertSyncFailureThreshold   int  `cfgreg:"env=ALERT_SYNC_FAILURE_THRESHOLD;type=int;range=>=1, invalid value refuses to boot;default=3;required=false;restart=true;desc=Consecutive sync failures before sync:* fires"`                                                                // Consecutive sync failures before sync:* fires
	AlertNotifyFailureThreshold int  `cfgreg:"env=ALERT_NOTIFY_FAILURE_THRESHOLD;type=int;range=>=1, invalid value refuses to boot;default=3;required=false;restart=true;desc=Consecutive notification failures before the notifications condition fires"`                                 // Consecutive notification failures before the notifications condition fires
	AlertBackupMaxAgeHours      int  `cfgreg:"env=ALERT_BACKUP_MAX_AGE_HOURS;type=int;range=>=0, invalid value refuses to boot;default=0;required=false;restart=true;desc=Raise backup_stale when the last backup success is older than this; 0 means 2x DB_RESTORE_DRILL_INTERVAL_HOURS"` // Raise backup_stale when the last backup success is older than this; 0 => 2 * DBRestoreDrillIntervalHours
	AlertJobStaleMultiplier     int  `cfgreg:"env=ALERT_JOB_STALE_MULTIPLIER;type=int;range=>=2, invalid value refuses to boot;default=3;required=false;restart=true;desc=Raise job_stopped when a job's last successful run is older than interval times this"`                           // Raise job_stopped when a job's last successful run is older than interval * this
	AlertIncidentQuietHours     int  `cfgreg:"env=ALERT_INCIDENT_QUIET_HOURS;type=int;range=>=1, invalid value refuses to boot;default=6;required=false;restart=true;desc=integrations recovers when no new integration_failed event lands within this window, in hours"`                  // integrations recovers when no new integration_failed event lands within this window
	AlertBackupEnabled          bool `cfgreg:"env=ALERT_BACKUP_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the backup / backup_stale conditions"`                                                                                                               // Enable the backup / backup_stale conditions
	AlertDBIntegrityEnabled     bool `cfgreg:"env=ALERT_DB_INTEGRITY_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the db_integrity condition"`                                                                                                                   // Enable the db_integrity condition
	AlertJobStoppedEnabled      bool `cfgreg:"env=ALERT_JOB_STOPPED_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the job_stopped condition"`                                                                                                                     // Enable the job_stopped condition
	AlertAuthSprayEnabled       bool `cfgreg:"env=ALERT_AUTH_SPRAY_ENABLED;type=bool;default=true;required=false;restart=true;desc=Enable the auth_spray condition"`                                                                                                                       // Enable the auth_spray condition (issue #940)
	HIBPCheckEnabled            bool `cfgreg:"env=HIBP_CHECK_ENABLED;type=bool;default=false;required=false;restart=true;desc=Check new/changed passwords against HIBP's k-anonymity range API"`                                                                                           // Check new/changed passwords against HIBP's k-anonymity range API (issue #376). Off by default: an outbound call on a self-hosted app is a deliberate opt-in, not a safe default — see docs/security/asvs-l2.md's P3.
	UpdateCheckEnabled          bool `cfgreg:"env=UPDATE_CHECK_ENABLED;type=bool;default=false;required=false;restart=true;desc=Compare the running build against the latest GitHub release"`                                                                                              // Compare the running build against the latest GitHub release (issue #650). Off by default: an outbound call on a self-hosted app is a deliberate opt-in, not a safe default — see docs/security/asvs-l2.md's P6.

	// Storage-growth trend thresholds (issue #652). The /admin/system-status
	// storage block folds usage_percent against these two tiers into
	// ok | warning | critical (with -5% hysteresis), and the daily storage
	// sampler retains StorageSampleRetentionDays of history.
	StorageWarnPercent         int `cfgreg:"env=STORAGE_WARN_PERCENT;type=int;range=1..99, invalid value refuses to boot;default=75;required=false;restart=true;desc=usage% >= this turns the storage threshold warning"`
	StorageCriticalPercent     int `cfgreg:"env=STORAGE_CRITICAL_PERCENT;type=int;range=>STORAGE_WARN_PERCENT and <=100, invalid value refuses to boot;default=90;required=false;restart=true;desc=usage% >= this turns the storage threshold critical"`
	StorageSampleRetentionDays int `cfgreg:"env=STORAGE_SAMPLE_RETENTION_DAYS;type=int;range=>=7, invalid value refuses to boot;default=180;required=false;restart=true;desc=Days of storage_samples history kept"`
	OIDC                       OIDCConfig

	// DataEncryptionKey is the base64-encoded 32-byte master key for
	// field-level at-rest encryption (issue #380, ASVS V6.4/V8.3). When unset,
	// atrest falls back to DataEncryptionKeyFile, then to an HKDF-SHA256
	// derivation from JWTSecretKey so existing deployments get encryption
	// with zero config. In-process callers resolve the actual key by passing
	// these two fields plus JWTSecretKey to atrest.ResolveMasterKey — the
	// validated Config fields ARE the resolution inputs, not a second,
	// independent env read (issue #938). See backend/atrest/atrest.go.
	DataEncryptionKey     string `cfgreg:"env=DATA_ENCRYPTION_KEY;type=string;range=base64-encoded 32 random bytes;default=;required=false;restart=true;desc=At-rest field-encryption master key"`                     // base64, 32 bytes
	DataEncryptionKeyFile string `cfgreg:"env=DATA_ENCRYPTION_KEY_FILE;type=string;range=path to an existing file;default=;required=false;restart=true;desc=Path to a file whose trimmed contents are the master key"` // path to a file whose trimmed contents are the base64 key

	// MetricsToken gates the Prometheus GET /metrics endpoint (issue #389).
	// Opt-in: when empty the route is not registered at all. When set, every
	// scrape must carry `Authorization: Bearer <MetricsToken>`. Minimum 16
	// characters (enforced in Validate) — a short scrape credential is worse
	// than none.
	MetricsToken string `cfgreg:"env=METRICS_TOKEN;type=string;range=>=16 characters when set;default=;required=false;restart=true;desc=Bearer token gating GET /metrics; unset leaves the route unregistered"`

	// MinClientVersion is the oldest Android client `versionName` this server
	// still supports, advertised verbatim on GET /health as
	// `min_client_version` (issue #528 —
	// docs/client-compatibility-policy.md, ANDROID-01/#478). Empty (the
	// default) means no floor has been declared: every released client stays
	// compatible. It is set via MIN_CLIENT_VERSION only when a MAINT-02
	// breaking change actually strands older clients — a floor is a deliberate,
	// reviewed event, never a side effect of a release.
	MinClientVersion string `cfgreg:"env=MIN_CLIENT_VERSION;type=string;range=major[.minor[.patch]] optionally with -prerelease/+build;default=;required=false;restart=true;desc=Oldest Android client versionName this server still supports"`

	// Process-level settings (issue #936): previously read directly from
	// os.Getenv at scattered call sites — DEMO_MODE in the controllers,
	// LOG_LEVEL/LOG_PRETTY/GIN_MODE in main.go's logger bootstrap — bypassing
	// Config entirely, so no register could be complete without them. Now
	// Config is the single source; main.go builds the logger from these
	// fields instead of raw env reads, and gin's own GIN_MODE env read
	// (which happens inside the gin package, outside our control) is left
	// alone, but every other place in this codebase that cared about
	// GIN_MODE now reads c.GinMode instead of re-reading the env var.
	DemoMode  bool   `cfgreg:"env=DEMO_MODE;type=bool;default=false;required=false;restart=true;desc=Disable registration-adjacent writes (password change, photo upload) for a public demo deployment"`
	LogLevel  string `cfgreg:"env=LOG_LEVEL;type=enum;enum=debug|info|warn|error|fatal|panic;default=info;required=false;restart=true;desc=Structured log verbosity"`
	LogPretty bool   `cfgreg:"env=LOG_PRETTY;type=bool;default=false;required=false;restart=true;desc=Console-formatted (vs. JSON) log output; always on when GIN_MODE != release, regardless of this setting"`
	GinMode   string `cfgreg:"env=GIN_MODE;type=enum;enum=debug|release|test;default=debug;required=false;restart=true;desc=Gin framework mode; also read directly by the gin package itself"`

	// parseErrors carries integer-parse failures (issue #937) from
	// LoadConfig's checkedInt calls through to Validate(), so a set-but-
	// unparseable value ("ALERT_DISK_USAGE_PERCENT=high") fails boot with a
	// named message instead of silently falling back to the default.
	// Unexported: not part of the public Config surface, never set by a
	// caller constructing a Config directly (a hand-built Config that skips
	// LoadConfig has no raw env values to have failed parsing in the first
	// place, so a nil/empty slice there is correct, not a gap).
	parseErrors []ValidationError
}

// Defaults for the storage-trend thresholds (issue #652). Exported so the
// storage threshold computation can reuse them for a raw config.Config built
// without LoadConfig (tests) — zero-value Config values resolve to these.
const (
	// DefaultDBRestoreDrillIntervalHours is the shipped cadence of the
	// scheduled restore drill: weekly. The backup-freshness ceiling stated in
	// docs/deployment.md's "Recovery objectives (RPO and RTO)" section is
	// derived from it (2 x this = the default ALERT_BACKUP_MAX_AGE_HOURS), and
	// TestRecoveryObjectivesDocMatchesShippedDefaults pins that the documented
	// number still follows from this constant (issue #506).
	DefaultDBRestoreDrillIntervalHours = 168

	// DefaultStorageWarnPercent is the used-percent at which the storage
	// threshold on /admin/system-status turns "warning" (default 75).
	DefaultStorageWarnPercent = 75
	// DefaultStorageCriticalPercent is the used-percent at which it turns
	// "critical" (default 90).
	DefaultStorageCriticalPercent = 90
	// DefaultStorageSampleRetentionDays is how long storage_samples rows
	// survive before the sampler prunes them (default 180).
	DefaultStorageSampleRetentionDays = 180
)

// LoadConfig reads environment variables (with sensible defaults) into a
// new Config.
func LoadConfig() *Config {

	defaultJWTExpiry := 96
	jwtExpiryHours, err := strconv.Atoi(getEnv("JWT_EXPIRY_HOURS", strconv.Itoa(defaultJWTExpiry)))
	if err != nil {
		log.Println("WARN: Invalid JWT expiry set. Please provide an integer value.")
		jwtExpiryHours = defaultJWTExpiry
	}

	// Parse timeout values with defaults
	readTimeout := getIntEnv("HTTP_READ_TIMEOUT", 15)
	writeTimeout := getIntEnv("HTTP_WRITE_TIMEOUT", 15)
	idleTimeout := getIntEnv("HTTP_IDLE_TIMEOUT", 60)

	// checkedInt is getIntEnv for the fields issue #937 moved from
	// clamp-with-WARN to fail-fast: a value that fails to parse is collected
	// into parseErrs (surfaced by Validate(), below) instead of silently
	// falling back to the default. Range/bounds for these same fields are
	// checked in Validate() too — LoadConfig no longer clamps them.
	var parseErrs []ValidationError
	checkedInt := func(key string, fallback int) int {
		v, perr := getIntEnvChecked(key, fallback)
		if perr != nil {
			parseErrs = append(parseErrs, *perr)
		}
		return v
	}

	cfg := &Config{
		DBPath:                        getEnv("SQLITE_DB_PATH", "mycorrhizal.db"),
		ReminderTime:                  getEnv("REMINDER_TIME", "06:00"),
		ReminderTimezone:              getEnv("REMINDER_TIMEZONE", "UTC"),
		FrontendURL:                   getEnv("FRONTEND_URL", "*"),
		Port:                          getEnv("PORT", "8080"),
		ResendAPIKey:                  getEnv("RESEND_API_KEY", ""),
		ResendFromEmail:               getEnv("RESEND_FROM_EMAIL", ""),
		SMTPHost:                      getEnv("SMTP_HOST", ""),
		SMTPPort:                      getIntEnv("SMTP_PORT", 587),
		SMTPUsername:                  getEnv("SMTP_USERNAME", ""),
		SMTPPassword:                  getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail:                 getEnv("SMTP_FROM_EMAIL", ""),
		SMTPUseTLS:                    getBoolEnv("SMTP_USE_TLS", false),
		JWTSecretKey:                  getEnv("JWT_SECRET_KEY", ""),
		JWTExpiryHours:                jwtExpiryHours,
		TrustedProxies:                getProxies(getEnv("TRUSTED_PROXIES", "")),
		ReadTimeout:                   readTimeout,
		WriteTimeout:                  writeTimeout,
		IdleTimeout:                   idleTimeout,
		ProfilePhotoDir:               getEnv("PROFILE_PHOTO_DIR", ""),
		AttachmentsDir:                getEnv("ATTACHMENTS_DIR", filepath.Join(filepath.Dir(getEnv("PROFILE_PHOTO_DIR", "")), "attachments")),
		CardDAVEnabled:                getBoolEnv("CARDDAV_ENABLED", false),
		CalDAVEnabled:                 getBoolEnv("CALDAV_ENABLED", false),
		CalDAVTwoWayEnabled:           getBoolEnv("CALDAV_TWO_WAY_ENABLED", false),
		CookieSecure:                  getBoolEnv("COOKIE_SECURE", false),
		CookieDomain:                  getEnv("COOKIE_DOMAIN", ""),
		RegistrationDisabled:          getBoolEnv("DISABLE_REGISTRATION", false),
		WebhookBlockPrivateURLs:       getBoolEnv("WEBHOOK_BLOCK_PRIVATE_URLS", false),
		CalDAVSyncIntervalHours:       checkedInt("CALDAV_SYNC_INTERVAL_HOURS", 6),
		CalDAVBlockPrivateURLs:        getBoolEnv("CALDAV_BLOCK_PRIVATE_URLS", false),
		DeleteRetentionDays:           getIntEnv("DELETED_RETENTION_DAYS", 30),
		AuditRetentionDays:            getIntEnv("AUDIT_RETENTION_DAYS", 90),
		ContactShareRetentionDays:     getIntEnv("CONTACT_SHARE_RETENTION_DAYS", 30),
		SystemEventRetentionDays:      getIntEnv("SYSTEM_EVENT_RETENTION_DAYS", 30),
		WebhookDeliveryRetentionDays:  getIntEnv("WEBHOOK_DELIVERY_RETENTION_DAYS", 30),
		JobRunRetentionDays:           getIntEnv("JOB_RUN_RETENTION_DAYS", 30),
		IdempotencyKeyRetentionHours:  getIntEnv("IDEMPOTENCY_KEY_RETENTION_HOURS", 24),
		SessionIdleTimeoutHours:       getIntEnv("SESSION_IDLE_TIMEOUT_HOURS", 12),
		APIRateLimitInterval:          time.Duration(getIntEnv("API_RATE_LIMIT_INTERVAL_MS", 600)) * time.Millisecond,
		APIRateLimitBurst:             getIntEnv("API_RATE_LIMIT_BURST", 1000),
		AuthSprayEnabled:              getBoolEnv("AUTH_SPRAY_ENABLED", true),
		AuthSprayWindowSeconds:        getIntEnv("AUTH_SPRAY_WINDOW_SECONDS", 60),
		AuthSprayFailureThreshold:     getIntEnv("AUTH_SPRAY_FAILURE_THRESHOLD", 60),
		AuthSprayIdentifierThreshold:  getIntEnv("AUTH_SPRAY_IDENTIFIER_THRESHOLD", 15),
		AuthSprayThrottleSeconds:      getIntEnv("AUTH_SPRAY_THROTTLE_SECONDS", 300),
		ImmichSyncIntervalHours:       checkedInt("IMMICH_SYNC_INTERVAL_HOURS", 6),
		ImmichBlockPrivateURLs:        getBoolEnv("IMMICH_BLOCK_PRIVATE_URLS", false),
		PaperlessBlockPrivateURLs:     getBoolEnv("PAPERLESS_BLOCK_PRIVATE_URLS", false),
		SeafileBlockPrivateURLs:       getBoolEnv("SEAFILE_BLOCK_PRIVATE_URLS", false),
		WebDAVBlockPrivateURLs:        getBoolEnv("WEBDAV_BLOCK_PRIVATE_URLS", false),
		MonicaBlockPrivateURLs:        getBoolEnv("MONICA_BLOCK_PRIVATE_URLS", false),
		FCMServiceAccountFile:         getEnv("FCM_SERVICE_ACCOUNT_FILE", ""),
		DBIntegrityCheckEnabled:       getBoolEnv("DB_INTEGRITY_CHECK_ENABLED", true),
		DBIntegrityCheckIntervalHours: checkedInt("DB_INTEGRITY_CHECK_INTERVAL_HOURS", 24),
		DBRestoreDrillEnabled:         getBoolEnv("DB_RESTORE_DRILL_ENABLED", true),
		DBRestoreDrillIntervalHours:   checkedInt("DB_RESTORE_DRILL_INTERVAL_HOURS", DefaultDBRestoreDrillIntervalHours),
		AlertingEnabled:               getBoolEnv("ALERTING_ENABLED", true),
		AlertEvalIntervalMinutes:      checkedInt("ALERT_EVAL_INTERVAL_MINUTES", 15),
		AlertDiskUsagePercent:         checkedInt("ALERT_DISK_USAGE_PERCENT", 90),
		AlertSyncFailureThreshold:     checkedInt("ALERT_SYNC_FAILURE_THRESHOLD", 3),
		AlertNotifyFailureThreshold:   checkedInt("ALERT_NOTIFY_FAILURE_THRESHOLD", 3),
		AlertBackupMaxAgeHours:        checkedInt("ALERT_BACKUP_MAX_AGE_HOURS", 0),
		AlertJobStaleMultiplier:       checkedInt("ALERT_JOB_STALE_MULTIPLIER", 3),
		AlertIncidentQuietHours:       checkedInt("ALERT_INCIDENT_QUIET_HOURS", 6),
		AlertBackupEnabled:            getBoolEnv("ALERT_BACKUP_ENABLED", true),
		AlertDBIntegrityEnabled:       getBoolEnv("ALERT_DB_INTEGRITY_ENABLED", true),
		AlertJobStoppedEnabled:        getBoolEnv("ALERT_JOB_STOPPED_ENABLED", true),
		AlertAuthSprayEnabled:         getBoolEnv("ALERT_AUTH_SPRAY_ENABLED", true),
		HIBPCheckEnabled:              getBoolEnv("HIBP_CHECK_ENABLED", false),
		UpdateCheckEnabled:            getBoolEnv("UPDATE_CHECK_ENABLED", false),
		StorageWarnPercent:            checkedInt("STORAGE_WARN_PERCENT", DefaultStorageWarnPercent),
		StorageCriticalPercent:        checkedInt("STORAGE_CRITICAL_PERCENT", DefaultStorageCriticalPercent),
		StorageSampleRetentionDays:    checkedInt("STORAGE_SAMPLE_RETENTION_DAYS", DefaultStorageSampleRetentionDays),
		DataEncryptionKey:             getEnv("DATA_ENCRYPTION_KEY", ""),
		DataEncryptionKeyFile:         getEnv("DATA_ENCRYPTION_KEY_FILE", ""),
		MetricsToken:                  getEnv("METRICS_TOKEN", ""),
		MinClientVersion:              getEnv("MIN_CLIENT_VERSION", ""),
		DemoMode:                      getBoolEnv("DEMO_MODE", false),
		LogLevel:                      getEnv("LOG_LEVEL", "info"),
		GinMode:                       getEnv("GIN_MODE", "debug"),
	}

	// Assigned outside the aligned literal above so a longer key name does not
	// reflow every line in it. Opt-in RTO budget for the restore drill (#506).
	cfg.DBRestoreDrillMaxDurationSeconds = checkedInt("DB_RESTORE_DRILL_MAX_DURATION_SECONDS", 0)

	// issue #937: collected by checkedInt above; surfaced (with the range
	// checks that used to live here as WARN+clamp blocks) by Validate().
	cfg.parseErrors = parseErrs

	// LogPretty (issue #936): preserves the exact behavior main.go used to
	// compute directly from os.Getenv — an explicit LOG_PRETTY is honored,
	// but a non-release GIN_MODE always forces pretty output regardless of
	// what LOG_PRETTY says, so local/dev logs stay readable by default.
	cfg.LogPretty = getBoolEnv("LOG_PRETTY", false)
	if cfg.GinMode != "release" {
		cfg.LogPretty = true
	}

	// CALDAV_SYNC_INTERVAL_HOURS, IMMICH_SYNC_INTERVAL_HOURS,
	// DB_INTEGRITY_CHECK_INTERVAL_HOURS, DB_RESTORE_DRILL_INTERVAL_HOURS,
	// DB_RESTORE_DRILL_MAX_DURATION_SECONDS, and the ALERT_* thresholds below
	// used to be clamped to a safe value here (some silently, issue #937).
	// They're now range-checked in Validate() instead — a bad value refuses
	// to boot, naming the variable, rather than running with a value the
	// operator didn't choose. See the "issue #937" block in Validate().

	// Instance-wide failed-auth velocity (issue #940). A zero/negative value
	// would make the signal trip on the first failure (or never), so clamp to
	// the safe defaults rather than letting a typo silently disable a security
	// control. AUTH_SPRAY_ENABLED is the deliberate off switch.
	if cfg.AuthSprayWindowSeconds < 1 {
		log.Println("WARN: AUTH_SPRAY_WINDOW_SECONDS must be at least 1, using 60")
		cfg.AuthSprayWindowSeconds = 60
	}
	if cfg.AuthSprayFailureThreshold < 1 {
		log.Println("WARN: AUTH_SPRAY_FAILURE_THRESHOLD must be at least 1, using 60")
		cfg.AuthSprayFailureThreshold = 60
	}
	if cfg.AuthSprayIdentifierThreshold < 1 {
		log.Println("WARN: AUTH_SPRAY_IDENTIFIER_THRESHOLD must be at least 1, using 15")
		cfg.AuthSprayIdentifierThreshold = 15
	}
	if cfg.AuthSprayThrottleSeconds < 1 {
		log.Println("WARN: AUTH_SPRAY_THROTTLE_SECONDS must be at least 1, using 300")
		cfg.AuthSprayThrottleSeconds = 300
	}

	// Storage-trend thresholds (issue #652) used to be clamped here too
	// (issue #937); STORAGE_WARN_PERCENT/STORAGE_CRITICAL_PERCENT/
	// STORAGE_SAMPLE_RETENTION_DAYS are now range-checked in Validate().

	// An email channel is enabled only when it is fully configured
	cfg.UseResend = cfg.ResendAPIKey != "" && cfg.ResendFromEmail != ""
	cfg.UseSMTP = cfg.SMTPHost != "" && cfg.SMTPFromEmail != ""

	oidcProviderURL := getEnv("OIDC_PROVIDER_URL", "")
	oidcClientID := getEnv("OIDC_CLIENT_ID", "")
	oidcClientSecret := getEnv("OIDC_CLIENT_SECRET", "")
	cfg.OIDC = OIDCConfig{
		Enabled:               oidcProviderURL != "" && oidcClientID != "" && oidcClientSecret != "",
		ProviderURL:           oidcProviderURL,
		ClientID:              oidcClientID,
		ClientSecret:          oidcClientSecret,
		RedirectURL:           cfg.FrontendURL + "/api/v1/auth/oidc/callback",
		AllowAutoProvision:    getBoolEnv("OIDC_AUTO_PROVISION", false),
		TrustEmail:            getBoolEnv("OIDC_TRUST_EMAIL", false),
		Scopes:                getScopesEnv(getEnv("OIDC_SCOPES", "")),
		PostLogoutRedirectURL: cfg.FrontendURL + "/login",
		BlockPrivateURLs:      getBoolEnv("OIDC_BLOCK_PRIVATE_URLS", false),
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		intValue, err := strconv.Atoi(value)
		if err != nil {
			log.Printf("WARN: Invalid integer value for %s: %s. Using default: %d", key, value, fallback)
			return fallback
		}
		return intValue
	}
	return fallback
}

// getIntEnvChecked is getIntEnv's counterpart for the fields issue #937
// moved from clamp-with-WARN to fail-fast. Unlike getIntEnv, it lets the
// caller distinguish "unset" (fine, use the default) from "set but
// unparseable" (must fail boot): the former returns a nil error, the
// latter a *ValidationError naming the variable, the offending raw value,
// and the expected form — collected into Config.parseErrors by LoadConfig
// and surfaced by Validate(), so a typo'd value fails boot instead of
// silently running on the default.
func getIntEnvChecked(key string, fallback int) (int, *ValidationError) {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback, nil
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return fallback, &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("Invalid value '%s' for %s. Must be a whole number.", value, key),
		}
	}
	return intValue, nil
}

func getBoolEnv(key string, fallback bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			log.Printf("WARN: Invalid boolean value for %s: %s. Using default: %v", key, value, fallback)
			return fallback
		}
		return boolValue
	}
	return fallback
}

func getProxies(proxies string) []string {
	if proxies == "" {
		return nil
	}

	proxyList := strings.Split(proxies, ",")
	for i, proxy := range proxyList {
		proxyList[i] = strings.TrimSpace(proxy) // Remove whitespaces
	}
	return proxyList
}

// DefaultTrustedProxies is the trusted-proxy set the server falls back to when
// TRUSTED_PROXIES is unset or empty. The shipped all-in-one image always runs
// nginx in front of the backend over loopback (docker/nginx.conf proxies to
// 127.0.0.1:8081), so with no trusted proxy every request would collapse into
// a single 127.0.0.1 rate-limit bucket — a self-inflicted shared-bucket DoS,
// and one that makes per-client limiting meaningless (issue #954). Trusting
// loopback is safe even for a directly exposed instance: a remote client can
// never present a loopback RemoteAddr, so only the bundled proxy (or another
// local process) can ever match it.
func DefaultTrustedProxies() []string {
	return []string{"127.0.0.1/32", "::1/128"}
}

// EffectiveTrustedProxies is what the server hands to gin's SetTrustedProxies:
// the operator's configured list when non-empty, otherwise the loopback
// default that matches the shipped nginx topology.
func (c *Config) EffectiveTrustedProxies() []string {
	if len(c.TrustedProxies) == 0 {
		return DefaultTrustedProxies()
	}
	return c.TrustedProxies
}

// isCatchAllProxy reports whether a trusted-proxy entry trusts every source,
// i.e. a CIDR whose prefix length is zero (0.0.0.0/0 or ::/0). These are the
// values that let a client forge its own X-Forwarded-For (issue #954) and are
// refused at boot.
func isCatchAllProxy(proxy string) bool {
	_, network, err := net.ParseCIDR(proxy)
	if err != nil {
		return false
	}
	ones, _ := network.Mask.Size()
	return ones == 0
}

// TrustedProxyWarnings returns advisory boot messages about the trusted-proxy
// posture (issue #954) that must be surfaced but must not block startup. The
// case it catches: a release deployment with no TRUSTED_PROXIES configured.
// The loopback default keeps the shipped all-in-one image correct, but an
// operator who fronts the app with an external proxy (bypassing the bundled
// nginx) must add that proxy's address, or every client shares one bucket and
// one log IP. Advisory only — bare-metal deployments with no proxy are fine,
// and are why this warns rather than refuses.
func (c *Config) TrustedProxyWarnings() []string {
	if c.GinMode != "release" {
		return nil
	}
	if len(c.TrustedProxies) == 0 {
		return []string{"TRUSTED_PROXIES is empty; trusting loopback (127.0.0.1/32, ::1/128) for the bundled nginx. If an external reverse proxy fronts this instance, add its address to TRUSTED_PROXIES — otherwise every client shares the proxy's rate-limit bucket and logged IP."}
	}
	return nil
}

// getScopesEnv parses a comma-separated OIDC scope list, defaulting to
// openid/email/profile when unset since scopes must never end up empty.
func getScopesEnv(scopes string) []string {
	if scopes == "" {
		return []string{"openid", "email", "profile"}
	}

	scopeList := strings.Split(scopes, ",")
	for i, scope := range scopeList {
		scopeList[i] = strings.TrimSpace(scope)
	}
	return scopeList
}

// knownPlaceholderJWTSecrets are the exact secret values that ship as
// copy-paste defaults in this repo's .env.example files (the value has never
// changed since the fork). They are published constants by definition, so
// even though they clear the length floor, booting with one must fail —
// anyone who can read the repo can sign tokens with it.
var knownPlaceholderJWTSecrets = []string{
	"your-very-long-very-secret-jwt-key-change-this-in-production",
}

// placeholderJWTSecretMarkers are unambiguous fragments that indicate a
// template/example secret rather than a real one. Matched case-insensitively
// as substrings so a padded or re-cased copy of a placeholder is still
// caught. Kept deliberately narrow: a false positive here is a legitimate
// deployment refusing to boot, so only markers that no real random secret or
// passphrase would contain belong on this list.
var placeholderJWTSecretMarkers = []string{
	"change-this-in-production",
	"changeme",
	"change-me",
	"change_me",
	"your-secret",
	"yoursecret",
	"replace-me",
	"replaceme",
	"replace_me",
	"placeholder",
}

// isKnownPlaceholderJWTSecret reports whether a JWT secret is a published
// placeholder value, matched exactly or by an unambiguous marker. Case- and
// whitespace-insensitive, since an operator copying .env.example verbatim is
// exactly the failure mode this guard exists for.
func isKnownPlaceholderJWTSecret(secret string) bool {
	lower := strings.ToLower(strings.TrimSpace(secret))
	for _, p := range knownPlaceholderJWTSecrets {
		if lower == p {
			return true
		}
	}
	for _, m := range placeholderJWTSecretMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// minJWTSecretEntropyBits is the lowest total Shannon entropy (in bits) a
// JWT secret may carry. It sits far below what any random 32-byte secret
// produces (base64: ~190 bits; random alphanumeric: ~150 bits), so the only
// secrets it rejects are degenerate values that clear the length floor but
// are still trivially guessable — a repeated character, a near-uniform
// two-character alternation, a single digit. A passphrase of random words
// clears it comfortably. It is a deliberate heuristic backstop, not a
// predictability guarantee: a string that is long, high-cardinality and
// *structured* (a word repeated, the alphabet in order) still scores high,
// which is why the placeholder markers above exist and why the primary floor
// stays the ≥32-byte length requirement.
const minJWTSecretEntropyBits = 40.0

// jwtSecretEntropyBits estimates the total Shannon entropy of a secret in
// bits. It iterates over runes so a non-ASCII passphrase is scored by its
// actual character diversity, not penalized for UTF-8 byte length. The metric
// is a cheap backstop for degenerate secrets only — see minJWTSecretEntropyBits.
func jwtSecretEntropyBits(secret string) float64 {
	counts := make(map[rune]int)
	var total int
	for _, r := range secret {
		counts[r]++
		total++
	}
	if total == 0 {
		return 0
	}
	var bits float64
	for _, n := range counts {
		p := float64(n) / float64(total)
		bits -= p * math.Log2(p)
	}
	return bits * float64(total)
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

// clientVersionPattern matches the version shapes an Android `versionName`
// (and the server build version it is compared against) can legitimately
// take: `major`, `major.minor`, or `major.minor.patch`, optionally followed
// by a `-prerelease` or `+build` suffix, and optionally prefixed with `v`.
// The floor value in MIN_CLIENT_VERSION is compared against client version
// strings, so it must be a shape both sides can parse; anything else (a typo,
// a stray path) is rejected at boot rather than silently failing open on the
// client later. It deliberately does NOT require a full semver triple: a
// two-part `0.6` floor is a valid, meaningful declaration.
var clientVersionPattern = regexp.MustCompile(`^v?\d+(\.\d+){0,2}(-[0-9A-Za-z][0-9A-Za-z.-]*)?(\+[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

func isValidClientVersion(s string) bool {
	return clientVersionPattern.MatchString(strings.TrimSpace(s))
}

// cookieDomainPattern matches a hostname suitable for a cookie's Domain
// attribute: one or more dot-separated RFC 1123 labels (letters, digits,
// hyphens; no leading/trailing hyphen per label), optionally preceded by a
// single leading dot — the conventional way to request subdomain matching
// (issue #935). Deliberately does not require a public-suffix check: a
// self-hosted deployment's domain (a LAN hostname, a .local/.internal
// suffix) is not necessarily a real public TLD.
var cookieDomainPattern = regexp.MustCompile(`^\.?([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

func isValidCookieDomain(s string) bool {
	return cookieDomainPattern.MatchString(strings.TrimSpace(s))
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("Configuration Error [%s]: %s", e.Field, e.Message)
}

// Validate checks if the configuration is valid and returns detailed errors if not
func (c *Config) Validate() []ValidationError {
	// parseErrors (issue #937): a raw env value LoadConfig could not parse as
	// an integer for one of the fields below. Surfaced first so a malformed
	// value and an out-of-range value on the same field both show up, rather
	// than the range check silently re-validating LoadConfig's fallback.
	errors := append([]ValidationError(nil), c.parseErrors...)

	// Validate JWT Secret Key - critical for security. The checks are ordered
	// from most-certain to most-heuristic so a broken secret surfaces the
	// first specific reason, not a vague later one: empty, then too short,
	// then a known published placeholder, then too little entropy. All three
	// gates exist because every one of them makes every token forgeable: an
	// empty/short secret is brute-forceable, and a long-but-published or
	// long-but-predictable secret is forgeable by anyone who has read the
	// repo or the operator's shell history.
	if c.JWTSecretKey == "" {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: "JWT secret key is required for authentication. Set JWT_SECRET_KEY environment variable.",
		})
	} else if len(c.JWTSecretKey) < 32 {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: fmt.Sprintf("JWT secret key is too short (%d bytes). Must be at least 32 bytes for security.", len(c.JWTSecretKey)),
		})
	} else if isKnownPlaceholderJWTSecret(c.JWTSecretKey) {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: "JWT secret key is a known placeholder value (it ships as the default in .env.example). Every token signed with it is forgeable by anyone who has read the repo. Generate a fresh secret, e.g. `openssl rand -base64 32`.",
		})
	} else if jwtSecretEntropyBits(c.JWTSecretKey) < minJWTSecretEntropyBits {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: fmt.Sprintf("JWT secret key has too little entropy (est. %.0f bits). A long but predictable secret is as forgeable as a short one; generate a random secret, e.g. `openssl rand -base64 32`.", jwtSecretEntropyBits(c.JWTSecretKey)),
		})
	}

	// Validate Database Path
	if c.DBPath == "" {
		errors = append(errors, ValidationError{
			Field:   "SQLITE_DB_PATH",
			Message: "Database path cannot be empty. Set SQLITE_DB_PATH environment variable.",
		})
	}

	// Validate at-rest encryption master key, when one is explicitly set.
	// Unset is fine (atrest falls back to DATA_ENCRYPTION_KEY_FILE, then to an
	// HKDF derivation from JWT_SECRET_KEY), but a set-but-broken key must fail
	// boot: an operator who thinks they configured a key and didn't would
	// otherwise run with a weaker derivation than they believe.
	//
	// This reuses atrest.DecodeMasterKey — the same function
	// atrest.ResolveMasterKey calls to actually decode the key — rather than
	// a second, independent base64/length check, so validation and use can
	// never silently disagree about what makes a key valid (issue #938).
	if c.DataEncryptionKey != "" {
		if _, err := atrest.DecodeMasterKey(c.DataEncryptionKey); err != nil {
			errors = append(errors, ValidationError{
				Field:   "DATA_ENCRYPTION_KEY",
				Message: "DATA_ENCRYPTION_KEY must be base64-encoded 32 random bytes, e.g. `openssl rand -base64 32`. Unset it to use the JWT-derived key.",
			})
		}
	}
	if c.DataEncryptionKeyFile != "" {
		if _, err := os.Stat(c.DataEncryptionKeyFile); err != nil {
			errors = append(errors, ValidationError{
				Field:   "DATA_ENCRYPTION_KEY_FILE",
				Message: fmt.Sprintf("DATA_ENCRYPTION_KEY_FILE '%s' does not exist: %v", c.DataEncryptionKeyFile, err),
			})
		}
	}
	if c.DataEncryptionKey != "" && c.DataEncryptionKeyFile != "" {
		errors = append(errors, ValidationError{
			Field:   "DATA_ENCRYPTION_KEY",
			Message: "Set only one of DATA_ENCRYPTION_KEY or DATA_ENCRYPTION_KEY_FILE, not both.",
		})
	}

	// Validate Profile Photo Directory - must be set and absolute path for security
	if c.ProfilePhotoDir == "" {
		errors = append(errors, ValidationError{
			Field:   "PROFILE_PHOTO_DIR",
			Message: "Profile photo directory is required. Set PROFILE_PHOTO_DIR environment variable to an absolute path.",
		})
	} else if !filepath.IsAbs(c.ProfilePhotoDir) {
		errors = append(errors, ValidationError{
			Field:   "PROFILE_PHOTO_DIR",
			Message: fmt.Sprintf("Profile photo directory '%s' must be an absolute path for security.", c.ProfilePhotoDir),
		})
	}

	// Validate attachments directory - derived from the photo dir's parent by
	// default, so it must be absolute whenever the photo dir (also required,
	// above) is.
	if c.AttachmentsDir != "" && !filepath.IsAbs(c.AttachmentsDir) {
		errors = append(errors, ValidationError{
			Field:   "ATTACHMENTS_DIR",
			Message: fmt.Sprintf("Attachments directory '%s' must be an absolute path for security.", c.AttachmentsDir),
		})
	}

	// Validate Port
	if c.Port == "" {
		errors = append(errors, ValidationError{
			Field:   "PORT",
			Message: "Server port cannot be empty. Set PORT environment variable.",
		})
	} else {
		portNum, err := strconv.Atoi(c.Port)
		if err != nil || portNum < 1 || portNum > 65535 {
			errors = append(errors, ValidationError{
				Field:   "PORT",
				Message: fmt.Sprintf("Invalid port number '%s'. Must be between 1 and 65535.", c.Port),
			})
		}
	}

	// Validate Reminder Time format (HH:MM)
	timePattern := regexp.MustCompile(`^([0-1][0-9]|2[0-3]):[0-5][0-9]$`)
	if !timePattern.MatchString(c.ReminderTime) {
		errors = append(errors, ValidationError{
			Field:   "REMINDER_TIME",
			Message: fmt.Sprintf("Invalid time format '%s'. Must be in HH:MM format (e.g., 06:00).", c.ReminderTime),
		})
	}

	// Validate Reminder Timezone (must be a valid IANA timezone name)
	if _, err := time.LoadLocation(c.ReminderTimezone); err != nil {
		errors = append(errors, ValidationError{
			Field:   "REMINDER_TIMEZONE",
			Message: fmt.Sprintf("Invalid timezone '%s'. Must be a valid IANA timezone name (e.g., 'UTC', 'Europe/Berlin', 'America/New_York').", c.ReminderTimezone),
		})
	}

	// Validate Frontend URL
	if c.FrontendURL == "" {
		errors = append(errors, ValidationError{
			Field:   "FRONTEND_URL",
			Message: "Frontend URL cannot be empty. Set FRONTEND_URL environment variable (use '*' for development).",
		})
	}

	// FRONTEND_URL="*" combined with AllowCredentials:true (see main.go) makes the
	// server reflect any Origin while still accepting the auth cookie cross-site.
	// This is currently mitigated by SameSite=Lax on that cookie, so it's not a
	// live hole today - but it's fragile defense-in-depth, not a guarantee, so
	// refuse to boot with it in release mode. "*" remains fine for local dev
	// (GIN_MODE unset or "debug").
	if c.FrontendURL == "*" && c.GinMode == "release" {
		errors = append(errors, ValidationError{
			Field:   "FRONTEND_URL",
			Message: "FRONTEND_URL cannot be '*' when GIN_MODE=release. '*' is dev-only (see .env.example); set FRONTEND_URL to your actual frontend origin(s) in production.",
		})
	}

	// COOKIE_SECURE defaults to false so plain-HTTP self-hosting (LAN,
	// localhost, a reverse proxy that hasn't added TLS yet) keeps working
	// with zero config — unlike the FRONTEND_URL check above, this can't
	// key off GIN_MODE=release, since that's also true for the default
	// docker-compose HTTP setup and would refuse to boot the common case.
	// What's unambiguously a mistake, in dev or prod, is telling the app
	// the frontend is served over HTTPS while leaving the auth/OIDC/
	// id_token cookies without the Secure flag: there's no legitimate
	// reason to want that combination, only a forgotten setting.
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.FrontendURL)), "https://") && !c.CookieSecure {
		errors = append(errors, ValidationError{
			Field:   "COOKIE_SECURE",
			Message: "COOKIE_SECURE must be true when FRONTEND_URL is https://. Set COOKIE_SECURE=true (see .env.example) — serving over HTTPS with a non-Secure auth cookie has no legitimate use, only a missed setting.",
		})
	}

	// COOKIE_DOMAIN (issue #935): unvalidated, a malformed value is emitted
	// onto the auth cookie's Domain attribute verbatim. Empty is fine (the
	// documented "current domain only" default); a leading "." is allowed —
	// that's the standard way to request subdomain matching — but the rest
	// must look like a real hostname.
	if c.CookieDomain != "" && !isValidCookieDomain(c.CookieDomain) {
		errors = append(errors, ValidationError{
			Field:   "COOKIE_DOMAIN",
			Message: fmt.Sprintf("Invalid COOKIE_DOMAIN '%s'. Must be a hostname (e.g. 'example.com' or '.example.com' to match subdomains), or unset for the current domain only.", c.CookieDomain),
		})
	}

	// Validate JWT Expiry Hours
	if c.JWTExpiryHours < 1 || c.JWTExpiryHours > 8760 {
		errors = append(errors, ValidationError{
			Field:   "JWT_EXPIRY_HOURS",
			Message: fmt.Sprintf("Invalid JWT expiry hours '%d'. Must be between 1 and 8760 (1 year).", c.JWTExpiryHours),
		})
	}

	// Validate the session idle timeout (issue #866). 0 disables idle
	// enforcement; anything positive must not exceed the absolute JWT ceiling
	// (a larger value could never fire) and must be at least an hour (the
	// knob's unit — sub-hour idle windows are not a supported shape).
	if c.SessionIdleTimeoutHours < 0 || c.SessionIdleTimeoutHours > c.JWTExpiryHours {
		errors = append(errors, ValidationError{
			Field:   "SESSION_IDLE_TIMEOUT_HOURS",
			Message: fmt.Sprintf("Invalid session idle timeout '%d'. Must be 0 (disabled) or between 1 and JWT_EXPIRY_HOURS (%d).", c.SessionIdleTimeoutHours, c.JWTExpiryHours),
		})
	}

	// Validate HTTP Timeouts (in seconds)
	if c.ReadTimeout < 1 || c.ReadTimeout > 300 {
		errors = append(errors, ValidationError{
			Field:   "HTTP_READ_TIMEOUT",
			Message: fmt.Sprintf("Invalid read timeout '%d'. Must be between 1 and 300 seconds.", c.ReadTimeout),
		})
	}
	if c.WriteTimeout < 1 || c.WriteTimeout > 300 {
		errors = append(errors, ValidationError{
			Field:   "HTTP_WRITE_TIMEOUT",
			Message: fmt.Sprintf("Invalid write timeout '%d'. Must be between 1 and 300 seconds.", c.WriteTimeout),
		})
	}
	if c.IdleTimeout < 1 || c.IdleTimeout > 300 {
		errors = append(errors, ValidationError{
			Field:   "HTTP_IDLE_TIMEOUT",
			Message: fmt.Sprintf("Invalid idle timeout '%d'. Must be between 1 and 300 seconds.", c.IdleTimeout),
		})
	}

	// API_RATE_LIMIT_BURST/API_RATE_LIMIT_INTERVAL_MS (issue #935): a
	// zero/negative burst makes the limiter reject every request (or,
	// depending on implementation details, none at all) — either way, a
	// self-inflicted denial of service the operator did not intend. A
	// zero/negative interval is likewise never meaningful: the limiter
	// refills one token per interval.
	if c.APIRateLimitBurst < 1 {
		errors = append(errors, ValidationError{
			Field:   "API_RATE_LIMIT_BURST",
			Message: fmt.Sprintf("Invalid API_RATE_LIMIT_BURST '%d'. Must be at least 1.", c.APIRateLimitBurst),
		})
	}
	if c.APIRateLimitInterval <= 0 {
		errors = append(errors, ValidationError{
			Field:   "API_RATE_LIMIT_INTERVAL_MS",
			Message: fmt.Sprintf("Invalid API_RATE_LIMIT_INTERVAL_MS '%s'. Must be a positive duration in milliseconds.", c.APIRateLimitInterval),
		})
	}

	// A configured metrics scrape token must be long enough to be worth
	// having (issue #389). Empty is fine — it just leaves /metrics unregistered.
	if c.MetricsToken != "" && len(c.MetricsToken) < 16 {
		errors = append(errors, ValidationError{
			Field:   "METRICS_TOKEN",
			Message: "METRICS_TOKEN must be at least 16 characters when set.",
		})
	}

	// MIN_CLIENT_VERSION (issue #528) is advertised on the unauthenticated
	// GET /health and compared against Android versionNames, so a set-but-garbled
	// floor must fail boot: an operator who thinks they raised the floor and
	// didn't would ship an unadvertised breaking change. Empty (the default)
	// is the policy's normal state — no floor declared.
	if c.MinClientVersion != "" && !isValidClientVersion(c.MinClientVersion) {
		errors = append(errors, ValidationError{
			Field:   "MIN_CLIENT_VERSION",
			Message: fmt.Sprintf("Invalid minimum client version %q. Must look like a client versionName: major, major.minor, or major.minor.patch, optionally with a -prerelease or +build suffix (e.g. \"0.6.0\" or \"0.6.0-rc.1\").", c.MinClientVersion),
		})
	}

	// Validate Trusted Proxies format (IP addresses or CIDR notation)
	for _, proxy := range c.TrustedProxies {
		if proxy == "" {
			continue
		}
		// Check if it's a valid IP address
		if ip := net.ParseIP(proxy); ip != nil {
			continue
		}
		// Check if it's a valid CIDR
		if _, _, err := net.ParseCIDR(proxy); err == nil {
			continue
		}
		errors = append(errors, ValidationError{
			Field:   "TRUSTED_PROXIES",
			Message: fmt.Sprintf("Invalid proxy '%s'. Must be a valid IP address or CIDR notation.", proxy),
		})
	}

	// Refuse a catch-all "trusted proxy" (0.0.0.0/0 or ::/0). Trusting every
	// source means gin believes whatever X-Forwarded-For the client supplied,
	// so a caller can rotate that header to escape the rate-limit bucket and
	// poison the IP recorded in logs (issue #954). There is no legitimate
	// deployment that needs to trust the whole internet; a /0 is never a real
	// proxy address. This is refused (not warned) because it silently defeats
	// the limiters rather than merely mis-bucketing them.
	for _, proxy := range c.TrustedProxies {
		if isCatchAllProxy(proxy) {
			errors = append(errors, ValidationError{
				Field:   "TRUSTED_PROXIES",
				Message: fmt.Sprintf("TRUSTED_PROXIES trusts every source ('%s'). That lets any client forge X-Forwarded-For and bypass IP rate limiting; list only the actual proxy addresses instead.", proxy),
			})
		}
	}

	// OIDC (issue #934): a partial configuration used to boot with SSO
	// silently disabled and only a log line naming the problem — an operator
	// who set one or two of the three required vars believed SSO was on.
	// Any var set at all means OIDC was intended, so an incomplete set now
	// fails boot instead, naming exactly what's missing.
	oidcSet := 0
	var oidcMissing []string
	for _, v := range []struct{ name, value string }{
		{"OIDC_PROVIDER_URL", c.OIDC.ProviderURL},
		{"OIDC_CLIENT_ID", c.OIDC.ClientID},
		{"OIDC_CLIENT_SECRET", c.OIDC.ClientSecret},
	} {
		if v.value != "" {
			oidcSet++
		} else {
			oidcMissing = append(oidcMissing, v.name)
		}
	}
	if oidcSet > 0 && oidcSet < 3 {
		errors = append(errors, ValidationError{
			Field:   "OIDC",
			Message: fmt.Sprintf("OIDC is partially configured (%d of 3 required variables set). Set %s too, or unset OIDC_PROVIDER_URL/OIDC_CLIENT_ID/OIDC_CLIENT_SECRET entirely to leave SSO disabled.", oidcSet, strings.Join(oidcMissing, ", ")),
		})
	}

	// A fully-configured OIDC provider must have a usable URL and a scope
	// list with no empty entries — neither was ever format-checked, so a
	// typo'd provider URL or a stray comma in OIDC_SCOPES (e.g.
	// "openid,,email") used to boot clean and fail only at the first login
	// attempt. Gated on oidcSet == 3, computed above from the fields
	// themselves, rather than c.OIDC.Enabled: Enabled is only ever derived
	// correctly by LoadConfig, and Validate must give the same answer for a
	// Config built any other way (tests, or any future caller).
	if oidcSet == 3 {
		if u, err := url.Parse(c.OIDC.ProviderURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errors = append(errors, ValidationError{
				Field:   "OIDC_PROVIDER_URL",
				Message: fmt.Sprintf("OIDC_PROVIDER_URL %q must be an absolute http(s) URL, e.g. https://idp.example.com/realms/main.", c.OIDC.ProviderURL),
			})
		}
		for _, scope := range c.OIDC.Scopes {
			if strings.TrimSpace(scope) == "" {
				errors = append(errors, ValidationError{
					Field:   "OIDC_SCOPES",
					Message: fmt.Sprintf("OIDC_SCOPES %q contains an empty entry — check for a stray or trailing comma.", strings.Join(c.OIDC.Scopes, ",")),
				})
				break
			}
		}
	}

	// M2: the FCM service account file is parsed and fully validated by the
	// services package at delivery time (config cannot import services without
	// a cycle). Here we only fail fast on the unambiguous mistake of pointing
	// at a path that doesn't exist.
	if fcmPath := c.FCMServiceAccountFile; fcmPath != "" {
		if _, err := os.Stat(fcmPath); err != nil {
			errors = append(errors, ValidationError{
				Field:   "FCM_SERVICE_ACCOUNT_FILE",
				Message: fmt.Sprintf("FCM service account file '%s' does not exist: %v. Unset it to disable mobile push, or point it at a valid Firebase service-account JSON.", fcmPath, err),
			})
		}
	}

	// Validate Resend configuration if emails are enabled
	if c.UseResend {
		if c.ResendAPIKey == "" {
			errors = append(errors, ValidationError{
				Field:   "RESEND_API_KEY",
				Message: "Resend API key is required when email is enabled.",
			})
		}
		if c.ResendFromEmail == "" {
			errors = append(errors, ValidationError{
				Field:   "RESEND_FROM_EMAIL",
				Message: "Resend sender email is required when email is enabled.",
			})
		}
	}

	// Validate SMTP configuration if SMTP delivery is enabled
	if c.UseSMTP {
		if c.SMTPHost == "" {
			errors = append(errors, ValidationError{
				Field:   "SMTP_HOST",
				Message: "SMTP host is required when SMTP email is enabled.",
			})
		}
		if c.SMTPFromEmail == "" {
			errors = append(errors, ValidationError{
				Field:   "SMTP_FROM_EMAIL",
				Message: "SMTP sender email is required when SMTP email is enabled.",
			})
		}
		if c.SMTPPort < 1 || c.SMTPPort > 65535 {
			errors = append(errors, ValidationError{
				Field:   "SMTP_PORT",
				Message: fmt.Sprintf("Invalid SMTP port '%d'. Must be between 1 and 65535.", c.SMTPPort),
			})
		}
	}

	// DELETED_RETENTION_DAYS is the soft-delete undo window. A positive value
	// is a number of days; 0 is the documented "disable the purge and keep
	// soft-deleted rows forever" value (see .env.example), so it is accepted. A
	// negative value is never meaningful — the purge service already guards
	// against hard-deleting the whole window (services/purge_service.go), but
	// failing at boot names the typo instead of silently running with the purge
	// disabled.
	if c.DeleteRetentionDays < 0 {
		errors = append(errors, ValidationError{
			Field:   "DELETED_RETENTION_DAYS",
			Message: fmt.Sprintf("Invalid retention '%d'. Must be 0 (disable the purge and keep soft-deleted rows forever) or a positive number of days.", c.DeleteRetentionDays),
		})
	}

	// The remaining retention-day knobs (issue #935) share DELETED_RETENTION_DAYS'
	// exact semantics — 0 disables the purge, negative is never meaningful —
	// but had no check at all, so a negative value silently armed each purge
	// job with a cutoff in the future.
	for _, r := range []struct {
		field string
		days  int
	}{
		{"AUDIT_RETENTION_DAYS", c.AuditRetentionDays},
		{"CONTACT_SHARE_RETENTION_DAYS", c.ContactShareRetentionDays},
		{"SYSTEM_EVENT_RETENTION_DAYS", c.SystemEventRetentionDays},
		{"WEBHOOK_DELIVERY_RETENTION_DAYS", c.WebhookDeliveryRetentionDays},
		{"JOB_RUN_RETENTION_DAYS", c.JobRunRetentionDays},
	} {
		if r.days < 0 {
			errors = append(errors, ValidationError{
				Field:   r.field,
				Message: fmt.Sprintf("Invalid retention '%d'. Must be 0 (disable the purge) or a positive number of days.", r.days),
			})
		}
	}

	// OIDC_PROVIDER_URL absolute-URL validation (issue #935) already landed
	// as part of #934/PR #1106's OIDC completeness gate, above — gated on
	// oidcSet == 3 there, which is the more correct condition (it only
	// checks the URL once OIDC is actually fully configured, rather than on
	// any non-empty value). Nothing to add here.

	// issue #937: these fields used to be silently clamped (or, for
	// AlertSyncFailureThreshold/AlertNotifyFailureThreshold/
	// AlertIncidentQuietHours/AlertBackupMaxAgeHours, clamped with no WARN
	// at all) in LoadConfig. A wrong number here changes safety-relevant
	// behavior — sync cadence, alert thresholds, backup-staleness — so a
	// bad value now refuses to boot instead of silently running on a value
	// the operator didn't choose.
	intRange := func(field string, value, min, max int, maxLabel string) {
		if value < min || (max >= 0 && value > max) {
			errors = append(errors, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("Invalid %s '%d'. Must be %s.", field, value, maxLabel),
			})
		}
	}
	intRange("CALDAV_SYNC_INTERVAL_HOURS", c.CalDAVSyncIntervalHours, 1, -1, "at least 1")
	intRange("IMMICH_SYNC_INTERVAL_HOURS", c.ImmichSyncIntervalHours, 1, -1, "at least 1")
	intRange("DB_INTEGRITY_CHECK_INTERVAL_HOURS", c.DBIntegrityCheckIntervalHours, 1, -1, "at least 1")
	intRange("DB_RESTORE_DRILL_INTERVAL_HOURS", c.DBRestoreDrillIntervalHours, 1, -1, "at least 1")
	intRange("DB_RESTORE_DRILL_MAX_DURATION_SECONDS", c.DBRestoreDrillMaxDurationSeconds, 0, -1, "0 (no budget) or positive")
	intRange("ALERT_EVAL_INTERVAL_MINUTES", c.AlertEvalIntervalMinutes, 1, -1, "at least 1")
	intRange("ALERT_DISK_USAGE_PERCENT", c.AlertDiskUsagePercent, 0, 99, "between 0 and 99")
	intRange("ALERT_SYNC_FAILURE_THRESHOLD", c.AlertSyncFailureThreshold, 1, -1, "at least 1")
	intRange("ALERT_NOTIFY_FAILURE_THRESHOLD", c.AlertNotifyFailureThreshold, 1, -1, "at least 1")
	intRange("ALERT_JOB_STALE_MULTIPLIER", c.AlertJobStaleMultiplier, 2, -1, "at least 2")
	intRange("ALERT_INCIDENT_QUIET_HOURS", c.AlertIncidentQuietHours, 1, -1, "at least 1")
	intRange("ALERT_BACKUP_MAX_AGE_HOURS", c.AlertBackupMaxAgeHours, 0, -1, "0 (use 2x the restore-drill interval) or positive")
	intRange("STORAGE_WARN_PERCENT", c.StorageWarnPercent, 1, 99, "between 1 and 99")
	intRange("STORAGE_SAMPLE_RETENTION_DAYS", c.StorageSampleRetentionDays, 7, -1, "at least 7")

	// STORAGE_CRITICAL_PERCENT is checked against STORAGE_WARN_PERCENT's own
	// (already-validated-above) value, not a fixed floor, so it doesn't fit
	// the intRange helper.
	if c.StorageCriticalPercent <= c.StorageWarnPercent || c.StorageCriticalPercent > 100 {
		errors = append(errors, ValidationError{
			Field:   "STORAGE_CRITICAL_PERCENT",
			Message: fmt.Sprintf("Invalid STORAGE_CRITICAL_PERCENT '%d'. Must be above STORAGE_WARN_PERCENT (%d) and at most 100.", c.StorageCriticalPercent, c.StorageWarnPercent),
		})
	}

	// LOG_LEVEL and GIN_MODE (issue #936) are new Config fields as of this
	// change; give them a real enum check from day one rather than
	// reintroducing the silent-fallback problem elsewhere in this file.
	switch c.LogLevel {
	case "debug", "info", "warn", "error", "fatal", "panic":
	default:
		errors = append(errors, ValidationError{
			Field:   "LOG_LEVEL",
			Message: fmt.Sprintf("Invalid log level '%s'. Must be one of: debug, info, warn, error, fatal, panic.", c.LogLevel),
		})
	}
	switch c.GinMode {
	case "debug", "release", "test":
	default:
		errors = append(errors, ValidationError{
			Field:   "GIN_MODE",
			Message: fmt.Sprintf("Invalid GIN_MODE '%s'. Must be one of: debug, release, test.", c.GinMode),
		})
	}

	return errors
}

// EmailEnabled reports whether at least one email delivery channel is configured.
func (c *Config) EmailEnabled() bool {
	return c.UseResend || c.UseSMTP
}

// GetReminderLocation returns the parsed time.Location for the configured ReminderTimezone.
// Falls back to UTC if the timezone is invalid (validation should prevent this in practice).
// This is the single zone in which the scheduler's daily wall-clock REMINDER_TIME is
// interpreted and in which the scheduled digest computes its day boundary — it is the
// whole product's one reminder clock (docs/adrs/0015-temporal-semantics.md).
func (c *Config) GetReminderLocation() *time.Location {
	loc, err := time.LoadLocation(c.ReminderTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ValidateOrPanic validates the configuration and panics with detailed error message if invalid
func (c *Config) ValidateOrPanic() {
	// MAINT-01 (issue #490): a deprecated configuration variable keeps working,
	// unchanged, for its whole window; the only change is this WARN naming its
	// replacement. Emitted before validation so it shows even on a boot that
	// then panics. Empty today — see deprecatedEnvVars.
	checkDeprecatedEnvVars(os.LookupEnv, func(msg string) { log.Println(msg) })

	errors := c.Validate()
	if len(errors) > 0 {
		log.Println("❌ Configuration validation failed:")
		log.Println("")
		for _, err := range errors {
			log.Printf("  • %s\n", err.Error())
		}
		log.Println("")
		log.Println("Please fix the configuration errors above and restart the server.")
		log.Println("Refer to backend/.env.example for configuration examples.")
		panic("Configuration validation failed")
	}
	log.Println("✓ Configuration validated successfully")
}

// deprecatedEnvVar records one retired configuration variable: the release it
// was deprecated in, and the variable that replaces it.
type deprecatedEnvVar struct {
	Replacement string
	Since       string
}

// deprecatedEnvVars is the configuration half of the MAINT-01 (issue #490)
// runtime signal — the counterpart to the `config`-surface rows in
// docs/deprecations.md. Every entry here has a register row. The map is empty
// until the first configuration variable is deprecated; the first one to be
// deprecated adds its entry in the same change that adds its register row.
var deprecatedEnvVars = map[string]deprecatedEnvVar{}

// checkDeprecatedEnvVars emits one WARN per *set* deprecated variable, naming
// its replacement — an unset deprecated variable produces nothing. lookup and
// warn are injected so the behavior is unit-testable without touching the
// process environment or the logger.
func checkDeprecatedEnvVars(lookup func(string) (string, bool), warn func(string)) {
	for name, d := range deprecatedEnvVars {
		if _, set := lookup(name); set {
			warn(fmt.Sprintf("WARN: %s is deprecated (since %s); use %s instead", name, d.Since, d.Replacement))
		}
	}
}

// blockPrivateURLsFlag names one SSRF-guard opt-in and its current value, for
// PublicExposureWarnings.
type blockPrivateURLsFlag struct {
	env     string
	enabled bool
}

// blockPrivateURLsFlags lists every *_BLOCK_PRIVATE_URLS-shaped opt-in
// (INT-02, issue #465) in one place, so PublicExposureWarnings can't drift
// from the actual set of guarded outbound clients.
func (c *Config) blockPrivateURLsFlags() []blockPrivateURLsFlag {
	return []blockPrivateURLsFlag{
		{"WEBHOOK_BLOCK_PRIVATE_URLS", c.WebhookBlockPrivateURLs},
		{"CALDAV_BLOCK_PRIVATE_URLS", c.CalDAVBlockPrivateURLs},
		{"IMMICH_BLOCK_PRIVATE_URLS", c.ImmichBlockPrivateURLs},
		{"PAPERLESS_BLOCK_PRIVATE_URLS", c.PaperlessBlockPrivateURLs},
		{"SEAFILE_BLOCK_PRIVATE_URLS", c.SeafileBlockPrivateURLs},
		{"WEBDAV_BLOCK_PRIVATE_URLS", c.WebDAVBlockPrivateURLs},
		{"MONICA_BLOCK_PRIVATE_URLS", c.MonicaBlockPrivateURLs},
		{"OIDC_BLOCK_PRIVATE_URLS", c.OIDC.BlockPrivateURLs},
	}
}

// PublicExposureWarnings returns advisory boot messages about the SSRF-guard
// posture (issue #951). Every *_BLOCK_PRIVATE_URLS flag defaults to off so a
// trusted-LAN self-host (a webhook target or Immich/CardDAV/OIDC instance on
// the same Docker network) keeps working with zero config — that default is
// correct and deliberately not changed here. What this catches is the
// combination the threat model's self-hosted boundary doesn't cover: a
// deployment that looks reachable from outside a trusted LAN (FRONTEND_URL
// is https://, or COOKIE_SECURE=true — the same signal Validate's
// COOKIE_SECURE check uses) while a guard an authenticated user's input can
// reach is still off. Advisory only, like TrustedProxyWarnings — refusing to
// boot would break the common trusted-LAN setup this app is built for, and
// there is no way to distinguish "public-facing" from "HTTPS on a trusted
// LAN" with certainty from config alone.
func (c *Config) PublicExposureWarnings() []string {
	impliesPublic := strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.FrontendURL)), "https://") || c.CookieSecure
	if !impliesPublic {
		return nil
	}

	var off []string
	for _, f := range c.blockPrivateURLsFlags() {
		if !f.enabled {
			off = append(off, f.env)
		}
	}
	if len(off) == 0 {
		return nil
	}

	return []string{fmt.Sprintf(
		"FRONTEND_URL is https:// and/or COOKIE_SECURE=true, which implies this deployment may be reachable from outside a trusted LAN — but these SSRF guards are still off: %s. An authenticated user can point them at loopback, other LAN hosts, or the cloud-metadata endpoint (169.254.169.254). Set the ones that apply to true if this instance is public-facing or hosts accounts you do not personally vet; see docs/security/deployment-baseline.md.",
		strings.Join(off, ", "),
	)}
}
