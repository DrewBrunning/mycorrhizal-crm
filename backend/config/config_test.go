package config

import (
	"encoding/base64"
	"testing"
	"time"

	"mycorrhizal/atrest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig returns a Config that passes Validate() with no errors, so
// individual tests can mutate just the field(s) they care about.
func validConfig() *Config {
	return &Config{
		DBPath:           "test.db",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		FrontendURL:      "https://crm.example.com",
		CookieSecure:     true,
		Port:             "8080",
		JWTSecretKey:     "a-very-long-jwt-secret-key-that-is-32-chars",
		JWTExpiryHours:   96,
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
		ProfilePhotoDir:  "/var/data/photos",
	}
}

func hasFieldError(errs []ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestValidate_ValidConfigHasNoErrors(t *testing.T) {
	cfg := validConfig()
	errs := cfg.Validate()
	assert.Empty(t, errs)
}

func TestValidate_MetricsToken(t *testing.T) {
	t.Run("empty is fine (endpoint stays unregistered)", func(t *testing.T) {
		cfg := validConfig()
		cfg.MetricsToken = ""
		assert.False(t, hasFieldError(cfg.Validate(), "METRICS_TOKEN"))
	})
	t.Run("too short is rejected", func(t *testing.T) {
		cfg := validConfig()
		cfg.MetricsToken = "short"
		assert.True(t, hasFieldError(cfg.Validate(), "METRICS_TOKEN"))
	})
	t.Run("16+ chars accepted", func(t *testing.T) {
		cfg := validConfig()
		cfg.MetricsToken = "0123456789abcdef"
		assert.False(t, hasFieldError(cfg.Validate(), "METRICS_TOKEN"))
	})
}

func TestValidate_FrontendURLWildcard(t *testing.T) {
	tests := []struct {
		name        string
		ginMode     string
		expectError bool
	}{
		{name: "wildcard with GIN_MODE unset (dev)", ginMode: "", expectError: false},
		{name: "wildcard with GIN_MODE=debug", ginMode: "debug", expectError: false},
		{name: "wildcard with GIN_MODE=test", ginMode: "test", expectError: false},
		{name: "wildcard with GIN_MODE=release", ginMode: "release", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GIN_MODE", tt.ginMode)

			cfg := validConfig()
			cfg.FrontendURL = "*"
			errs := cfg.Validate()

			if tt.expectError {
				assert.True(t, hasFieldError(errs, "FRONTEND_URL"), "expected a FRONTEND_URL validation error, got: %v", errs)
			} else {
				assert.False(t, hasFieldError(errs, "FRONTEND_URL"), "did not expect a FRONTEND_URL validation error, got: %v", errs)
			}
		})
	}
}

func TestValidate_SpecificFrontendURLAllowedInRelease(t *testing.T) {
	t.Setenv("GIN_MODE", "release")

	cfg := validConfig()
	cfg.FrontendURL = "https://crm.example.com"
	errs := cfg.Validate()

	assert.False(t, hasFieldError(errs, "FRONTEND_URL"), "a specific FRONTEND_URL should be allowed in release mode, got: %v", errs)
}

func TestValidate_EmptyFrontendURLStillRejected(t *testing.T) {
	t.Setenv("GIN_MODE", "")

	cfg := validConfig()
	cfg.FrontendURL = ""
	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "FRONTEND_URL"), "empty FRONTEND_URL should still be rejected regardless of GIN_MODE, got: %v", errs)
}

func TestValidate_CookieSecureRequiredForHTTPSFrontend(t *testing.T) {
	cfg := validConfig()
	cfg.FrontendURL = "https://crm.example.com"
	cfg.CookieSecure = false

	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "COOKIE_SECURE"), "expected a COOKIE_SECURE validation error for an https:// frontend with CookieSecure=false, got: %v", errs)
}

func TestValidate_CookieSecureNotRequiredForHTTPFrontend(t *testing.T) {
	cfg := validConfig()
	cfg.FrontendURL = "http://localhost:7300"
	cfg.CookieSecure = false

	errs := cfg.Validate()

	assert.False(t, hasFieldError(errs, "COOKIE_SECURE"), "an http:// frontend should not require CookieSecure — that's the default docker-compose/local-dev case, got: %v", errs)
}

func TestValidate_CookieSecureRequiredForHTTPSFrontend_CaseAndWhitespaceInsensitive(t *testing.T) {
	// gin-contrib/cors trims and lowercases FRONTEND_URL before comparing it
	// against the request Origin, so an oddly-cased or padded value still
	// serves as a real https origin in practice — the guard must catch it
	// too, not just the canonical lowercase form.
	cfg := validConfig()
	cfg.FrontendURL = " HTTPS://crm.example.com "
	cfg.CookieSecure = false

	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "COOKIE_SECURE"), "expected a COOKIE_SECURE validation error for an oddly-cased/padded https:// frontend with CookieSecure=false, got: %v", errs)
}

func TestValidate_CookieSecureTrueAlwaysAllowed(t *testing.T) {
	cfg := validConfig()
	cfg.FrontendURL = "http://localhost:7300"
	cfg.CookieSecure = true

	errs := cfg.Validate()

	assert.False(t, hasFieldError(errs, "COOKIE_SECURE"), "CookieSecure=true should never itself be a validation error, got: %v", errs)
}

func TestGetScopesEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty defaults to openid/email/profile", input: "", expected: []string{"openid", "email", "profile"}},
		{name: "single scope", input: "openid", expected: []string{"openid"}},
		{name: "comma-separated with whitespace trimmed", input: " openid, email ", expected: []string{"openid", "email"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getScopesEnv(tt.input))
		})
	}
}

func TestValidate_JWTSecretKey(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecretKey = "short"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "JWT_SECRET_KEY"), "short JWT secret should error")

	cfg.JWTSecretKey = ""
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "JWT_SECRET_KEY"), "empty JWT secret should error")
}

func TestValidate_JWTSecretKey_RejectsKnownPlaceholder(t *testing.T) {
	// The exact value that ships in .env.example — long enough to clear the
	// length floor, but a published constant. This is the issue #393 core:
	// shipping the repo's own placeholder unchanged must fail boot.
	cfg := validConfig()
	cfg.JWTSecretKey = "your-very-long-very-secret-jwt-key-change-this-in-production"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "JWT_SECRET_KEY"), "the .env.example placeholder secret must be rejected, got: %v", errs)
}

func TestValidate_JWTSecretKey_RejectsPlaceholderVariants(t *testing.T) {
	// Copy-paste placeholders aren't always byte-identical: an operator may
	// re-case, pad, or use a different-but-equally-published template. All of
	// these must fail with the placeholder error, not slip past the length
	// floor.
	tests := []struct {
		name   string
		secret string
	}{
		{name: "env.example placeholder", secret: "your-very-long-very-secret-jwt-key-change-this-in-production"},
		{name: "uppercase re-casing of placeholder", secret: "YOUR-VERY-LONG-VERY-SECRET-JWT-KEY-CHANGE-THIS-IN-PRODUCTION"},
		{name: "whitespace-padded placeholder", secret: "  your-very-long-very-secret-jwt-key-change-this-in-production  "},
		{name: "change-me marker", secret: "super-secret-change-me-now-please-32char"},
		{name: "change_me marker", secret: "supersecretchange_menowplease-32char-xx"},
		{name: "changeme marker", secret: "this-changeme-is-not-a-real-secret-32ch"},
		{name: "your-secret marker", secret: "this-is-your-secret-key-do-not-use-32"},
		{name: "replace-me marker", secret: "please-replace-me-with-a-real-secret-32"},
		{name: "placeholder marker", secret: "a-long-placeholder-secret-key-32-chars"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.JWTSecretKey = tt.secret
			errs := cfg.Validate()
			assert.True(t, hasFieldError(errs, "JWT_SECRET_KEY"), "placeholder secret %q must be rejected, got: %v", tt.secret, errs)
		})
	}
}

func TestValidate_JWTSecretKey_RejectsLowEntropy(t *testing.T) {
	// Secrets long enough to clear the length floor but with trivial
	// entropy — a repeated character, a two-character alternation — are as
	// forgeable as a short one and must fail too. (A repeated *word* like
	// "secretsecret…" scores higher on Shannon entropy and is not covered;
	// see jwtSecretEntropyBits' doc comment.)
	tests := []struct {
		name   string
		secret string
	}{
		{name: "repeated character", secret: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "repeated digit", secret: "77777777777777777777777777777777"},
		{name: "two-character alternation", secret: "abababababababababababababababababab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.JWTSecretKey = tt.secret
			errs := cfg.Validate()
			assert.True(t, hasFieldError(errs, "JWT_SECRET_KEY"), "low-entropy secret %q must be rejected, got: %v", tt.secret, errs)
		})
	}
}

func TestValidate_JWTSecretKey_AcceptsStrongSecret(t *testing.T) {
	// A genuinely random-looking secret — the kind `openssl rand -base64 32`
	// produces — must pass all three gates.
	tests := []struct {
		name   string
		secret string
	}{
		{name: "base64-style random", secret: "Xk9zP2mNq4vL8rT1cV6bY3uE5wR0aJ7fS"},
		{name: "random alphanumeric with mixed case", secret: "kF9qZ2xV7cN5bM1jH8sD4lP0tR6wE3yA"},
		{name: "random words passphrase", secret: "harbor granite ravine lantern corset velvet!22"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.JWTSecretKey = tt.secret
			errs := cfg.Validate()
			assert.False(t, hasFieldError(errs, "JWT_SECRET_KEY"), "strong secret %q must be accepted, got: %v", tt.secret, errs)
		})
	}
}

func TestIsKnownPlaceholderJWTSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		expect bool
	}{
		{name: "exact env.example placeholder", secret: "your-very-long-very-secret-jwt-key-change-this-in-production", expect: true},
		{name: "uppercase placeholder", secret: "YOUR-VERY-LONG-VERY-SECRET-JWT-KEY-CHANGE-THIS-IN-PRODUCTION", expect: true},
		{name: "padded placeholder", secret: "  your-very-long-very-secret-jwt-key-change-this-in-production  ", expect: true},
		{name: "change-this-in-production marker", secret: "anything-change-this-in-production-suffix", expect: true},
		{name: "changeme marker", secret: "semi-changeme-anywhere-32-characters", expect: true},
		{name: "your-secret marker", secret: "your-secret-is-not-real-32-characters", expect: true},
		{name: "placeholder marker", secret: "some-placeholder-secret-32-characters", expect: true},
		{name: "random-looking secret", secret: "Xk9zP2mNq4vL8rT1cV6bY3uE5wR0aJ7fS", expect: false},
		{name: "legit secret containing the word secret", secret: "twilight-secret-garden-passphrase-42!", expect: false},
		{name: "dev-only secret", secret: "dev-secret-key-for-local-testing-only-not-for-prod", expect: false},
		{name: "e2e test secret", secret: "test-secret-key-for-e2e-testing-minimum-32-chars", expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, isKnownPlaceholderJWTSecret(tt.secret))
		})
	}
}

func TestJWTSecretEntropyBits(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		min    float64
		max    float64
	}{
		{name: "repeated character is ~0", secret: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", min: 0, max: 0.001},
		{name: "random-looking secret scores high", secret: "Xk9zP2mNq4vL8rT1cV6bY3uE5wR0aJ7fS", min: minJWTSecretEntropyBits, max: 1e9},
		{name: "passphrase scores high", secret: "harbor granite ravine lantern corset velvet!22", min: minJWTSecretEntropyBits, max: 1e9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bits := jwtSecretEntropyBits(tt.secret)
			assert.GreaterOrEqual(t, bits, tt.min, "entropy must be at least %f", tt.min)
			assert.LessOrEqual(t, bits, tt.max, "entropy must be at most %f", tt.max)
		})
	}
}

func TestValidate_SQLiteDBPath(t *testing.T) {
	cfg := validConfig()
	cfg.DBPath = ""
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "SQLITE_DB_PATH"), "empty DB path should error")
}

func TestValidate_Port(t *testing.T) {
	cfg := validConfig()
	cfg.Port = "0"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "PORT"), "port 0 should error")

	cfg.Port = "not-a-number"
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "PORT"), "non-numeric port should error")
}

func TestValidate_ReminderTime(t *testing.T) {
	cfg := validConfig()
	cfg.ReminderTime = "25:00"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "REMINDER_TIME"), "invalid hour should error")

	cfg.ReminderTime = "12:99"
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "REMINDER_TIME"), "invalid minute should error")

	cfg.ReminderTime = "abc"
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "REMINDER_TIME"), "non-time string should error")
}

func TestValidate_JWTExpiryHours(t *testing.T) {
	cfg := validConfig()
	cfg.JWTExpiryHours = 0
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "JWT_EXPIRY_HOURS"), "zero expiry should error")

	cfg.JWTExpiryHours = -1
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "JWT_EXPIRY_HOURS"), "negative expiry should error")
}

func TestValidate_SessionIdleTimeoutHours(t *testing.T) {
	cfg := validConfig() // SessionIdleTimeoutHours defaults to 0 (disabled)
	assert.False(t, hasFieldError(cfg.Validate(), "SESSION_IDLE_TIMEOUT_HOURS"), "0 is valid (disabled)")

	cfg.SessionIdleTimeoutHours = 12
	assert.False(t, hasFieldError(cfg.Validate(), "SESSION_IDLE_TIMEOUT_HOURS"), "a value under the JWT ceiling is valid")

	cfg.SessionIdleTimeoutHours = -1
	assert.True(t, hasFieldError(cfg.Validate(), "SESSION_IDLE_TIMEOUT_HOURS"), "negative should error")

	cfg.SessionIdleTimeoutHours = cfg.JWTExpiryHours + 1
	assert.True(t, hasFieldError(cfg.Validate(), "SESSION_IDLE_TIMEOUT_HOURS"), "above the JWT absolute ceiling should error")
}

func TestValidate_Timeouts(t *testing.T) {
	cfg := validConfig()
	cfg.ReadTimeout = 0
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "HTTP_READ_TIMEOUT"), "zero read timeout should error")

	cfg = validConfig()
	cfg.WriteTimeout = -1
	errs = cfg.Validate()
	assert.True(t, hasFieldError(errs, "HTTP_WRITE_TIMEOUT"), "negative write timeout should error")
}

func TestValidate_ReminderTimezone(t *testing.T) {
	cfg := validConfig()
	cfg.ReminderTimezone = "NotAReal/Timezone"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "REMINDER_TIMEZONE"), "invalid timezone should error")
}

// DELETED_RETENTION_DAYS is the soft-delete undo window. Positive values are a
// number of days; 0 is the documented "disable the purge and keep soft-deleted
// rows forever" value (.env.example), so it must validate; a negative value is
// never meaningful and must fail boot rather than silently disabling the purge.
// See issue #971.
func TestValidate_DeleteRetentionDays(t *testing.T) {
	t.Run("positive is accepted", func(t *testing.T) {
		cfg := validConfig()
		cfg.DeleteRetentionDays = 30
		assert.False(t, hasFieldError(cfg.Validate(), "DELETED_RETENTION_DAYS"))
	})

	t.Run("zero is accepted (documented purge-disable value)", func(t *testing.T) {
		cfg := validConfig()
		cfg.DeleteRetentionDays = 0
		assert.False(t, hasFieldError(cfg.Validate(), "DELETED_RETENTION_DAYS"),
			"0 is the documented 'disable the purge' value and must not be rejected")
	})

	t.Run("negative is rejected naming the variable and value", func(t *testing.T) {
		cfg := validConfig()
		cfg.DeleteRetentionDays = -1
		errs := cfg.Validate()
		require.True(t, hasFieldError(errs, "DELETED_RETENTION_DAYS"))

		var msg string
		for _, e := range errs {
			if e.Field == "DELETED_RETENTION_DAYS" {
				msg = e.Message
			}
		}
		assert.Contains(t, msg, "-1")
		assert.Contains(t, msg, "0", "the message must state the acceptable form (0 disables, positive is days)")
	})
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	cfg := LoadConfig()
	assert.NotNil(t, cfg)

	// Default timeout values
	assert.Equal(t, 15, cfg.ReadTimeout)
	assert.Equal(t, 15, cfg.WriteTimeout)
	assert.Equal(t, 60, cfg.IdleTimeout)

	// Default retention
	assert.Equal(t, 30, cfg.DeleteRetentionDays)
	assert.Equal(t, 90, cfg.AuditRetentionDays)
	assert.Equal(t, 30, cfg.ContactShareRetentionDays)
	assert.Equal(t, 30, cfg.WebhookDeliveryRetentionDays)

	// Default reminder schedule. Pinned because this value is also stated in
	// three places outside the code — .env.example, backend/.env.example and
	// docs/getting-started.md — and they had already drifted apart once
	// (12:00 in code and docs, 06:00 in both env samples).
	assert.Equal(t, "06:00", cfg.ReminderTime)
	assert.Equal(t, "UTC", cfg.ReminderTimezone)

	// Opt-in outbound flags default off (issue #376 HIBP, issue #650 update
	// check) — an outbound call on a self-hosted app is an operator decision.
	assert.False(t, cfg.HIBPCheckEnabled)
	assert.False(t, cfg.UpdateCheckEnabled)

	// Storage-trend thresholds and sampler retention (issue #652).
	assert.Equal(t, 75, cfg.StorageWarnPercent)
	assert.Equal(t, 90, cfg.StorageCriticalPercent)
	assert.Equal(t, 180, cfg.StorageSampleRetentionDays)

	// Restore-drill cadence and RTO budget (issue #506). The cadence is pinned
	// to the exported constant because docs/deployment.md's "Recovery objectives
	// (RPO and RTO)" section derives the 336 h backup-freshness ceiling from it;
	// the budget is opt-in, so it defaults to 0 (no budget).
	assert.Equal(t, 168, cfg.DBRestoreDrillIntervalHours)
	assert.Equal(t, DefaultDBRestoreDrillIntervalHours, cfg.DBRestoreDrillIntervalHours)
	assert.Equal(t, 0, cfg.DBRestoreDrillMaxDurationSeconds)
}

func TestLoadConfig_RestoreDrillMaxDurationSeconds(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	t.Setenv("DB_RESTORE_DRILL_MAX_DURATION_SECONDS", "900")
	assert.Equal(t, 900, LoadConfig().DBRestoreDrillMaxDurationSeconds)

	// Negative is clamped to 0 (no budget) rather than refusing to boot — same
	// posture as the ALERT_* knobs.
	t.Setenv("DB_RESTORE_DRILL_MAX_DURATION_SECONDS", "-5")
	assert.Equal(t, 0, LoadConfig().DBRestoreDrillMaxDurationSeconds)
}

func TestLoadConfig_StorageThresholds(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")

	t.Setenv("STORAGE_WARN_PERCENT", "80")
	t.Setenv("STORAGE_CRITICAL_PERCENT", "95")
	t.Setenv("STORAGE_SAMPLE_RETENTION_DAYS", "365")
	cfg := LoadConfig()
	assert.Equal(t, 80, cfg.StorageWarnPercent)
	assert.Equal(t, 95, cfg.StorageCriticalPercent)
	assert.Equal(t, 365, cfg.StorageSampleRetentionDays)

	// A critical at or below warn is meaningless — clamp to the default.
	t.Setenv("STORAGE_WARN_PERCENT", "85")
	t.Setenv("STORAGE_CRITICAL_PERCENT", "85")
	assert.Equal(t, 90, LoadConfig().StorageCriticalPercent)

	// A retention window that can't hold even a week of samples is clamped.
	t.Setenv("STORAGE_SAMPLE_RETENTION_DAYS", "3")
	assert.Equal(t, 180, LoadConfig().StorageSampleRetentionDays)
}

func TestLoadConfig_UpdateCheckEnabledEnv(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")

	// Default (unset) is off.
	require.False(t, LoadConfig().UpdateCheckEnabled)

	t.Setenv("UPDATE_CHECK_ENABLED", "true")
	assert.True(t, LoadConfig().UpdateCheckEnabled)

	t.Setenv("UPDATE_CHECK_ENABLED", "0")
	assert.False(t, LoadConfig().UpdateCheckEnabled)
}

func TestLoadConfig_DeleteRetentionDays(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("DELETED_RETENTION_DAYS", "14")

	cfg := LoadConfig()
	assert.Equal(t, 14, cfg.DeleteRetentionDays)
}

// A negative DELETED_RETENTION_DAYS survives the env read (getIntEnv does not
// clamp retention) and must be caught by Validate — the operator typo should
// fail boot rather than silently disable the purge. Issue #971.
func TestLoadConfig_NegativeDeleteRetentionDaysFailsValidation(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("DELETED_RETENTION_DAYS", "-1")

	cfg := LoadConfig()
	require.Equal(t, -1, cfg.DeleteRetentionDays)
	assert.True(t, hasFieldError(cfg.Validate(), "DELETED_RETENTION_DAYS"),
		"a negative DELETED_RETENTION_DAYS must fail boot-time validation")
}

// 0 is the documented "disable the purge and keep soft-deleted rows forever"
// value (.env.example), so it reads through unchanged and validates.
func TestLoadConfig_ZeroDeleteRetentionDaysDisablesPurgeAndValidates(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("DELETED_RETENTION_DAYS", "0")

	cfg := LoadConfig()
	require.Equal(t, 0, cfg.DeleteRetentionDays)
	assert.False(t, hasFieldError(cfg.Validate(), "DELETED_RETENTION_DAYS"))
}

func TestLoadConfig_ContactShareRetentionDays(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("CONTACT_SHARE_RETENTION_DAYS", "60")

	cfg := LoadConfig()
	assert.Equal(t, 60, cfg.ContactShareRetentionDays)
}

func TestLoadConfig_WebhookDeliveryRetentionDays(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("WEBHOOK_DELIVERY_RETENTION_DAYS", "60")

	cfg := LoadConfig()
	assert.Equal(t, 60, cfg.WebhookDeliveryRetentionDays)
}

func TestLoadConfig_DBIntegrityCheckIntervalHoursClampedToMinimumOne(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("DB_INTEGRITY_CHECK_INTERVAL_HOURS", "0")

	cfg := LoadConfig()
	assert.Equal(t, 1, cfg.DBIntegrityCheckIntervalHours, "an interval below 1 must be clamped, not left non-positive")
}

func TestLoadConfig_DBRestoreDrillIntervalHoursClampedToMinimumOne(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("DB_RESTORE_DRILL_INTERVAL_HOURS", "-5")

	cfg := LoadConfig()
	assert.Equal(t, 1, cfg.DBRestoreDrillIntervalHours, "a negative interval must be clamped, not left negative")
}

func TestValidateOrPanic_ValidConfigDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { validConfig().ValidateOrPanic() })
}

func TestValidateOrPanic_InvalidConfigPanics(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecretKey = ""

	assert.Panics(t, func() { cfg.ValidateOrPanic() })
}

func TestEmailEnabled(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *Config
		expect bool
	}{
		{name: "neither channel", cfg: &Config{UseResend: false, UseSMTP: false}, expect: false},
		{name: "resend only", cfg: &Config{UseResend: true, UseSMTP: false}, expect: true},
		{name: "smtp only", cfg: &Config{UseResend: false, UseSMTP: true}, expect: true},
		{name: "both channels", cfg: &Config{UseResend: true, UseSMTP: true}, expect: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.cfg.EmailEnabled())
		})
	}
}

func TestGetReminderLocation(t *testing.T) {
	tests := []struct {
		name       string
		timezone   string
		expectName string
	}{
		{name: "valid IANA timezone", timezone: "Europe/Berlin", expectName: "Europe/Berlin"},
		{name: "UTC", timezone: "UTC", expectName: "UTC"},
		{name: "invalid timezone falls back to UTC", timezone: "NotAReal/Zone", expectName: "UTC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{ReminderTimezone: tt.timezone}
			assert.Equal(t, tt.expectName, cfg.GetReminderLocation().String())
		})
	}
}

func TestGetReminderLocation_ReturnsLoadableLocation(t *testing.T) {
	cfg := &Config{ReminderTimezone: "America/New_York"}
	loc := cfg.GetReminderLocation()
	// The returned *time.Location must be usable for real conversions, not a
	// stale/unresolved placeholder.
	_, offset := time.Date(2026, 1, 15, 12, 0, 0, 0, loc).Zone()
	assert.Equal(t, -18000, offset, "America/New_York must resolve to UTC-5 in winter")
}

func TestGetBoolEnv(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback bool
		expect   bool
	}{
		{name: "true parses", value: "true", fallback: false, expect: true},
		{name: "1 parses", value: "1", fallback: false, expect: true},
		{name: "false parses", value: "false", fallback: true, expect: false},
		{name: "0 parses", value: "0", fallback: true, expect: false},
		{name: "invalid value uses fallback", value: "banana", fallback: true, expect: true},
		{name: "empty value uses fallback", value: "", fallback: false, expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL_ENV", tt.value)
			assert.Equal(t, tt.expect, getBoolEnv("TEST_BOOL_ENV", tt.fallback))
		})
	}
}

func TestGetBoolEnv_UnsetUsesFallback(t *testing.T) {
	t.Setenv("TEST_BOOL_ENV_UNSET", "")
	// An unset variable must take the fallback even when the fallback is true.
	assert.Equal(t, true, getBoolEnv("SURELY_NOT_SET_ANYWHERE_12345", true))
}

func TestGetProxies(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty returns nil", input: "", expected: nil},
		{name: "single proxy", input: "10.0.0.1", expected: []string{"10.0.0.1"}},
		{name: "comma-separated with whitespace trimmed", input: " 10.0.0.1 , 10.0.0.2 ", expected: []string{"10.0.0.1", "10.0.0.2"}},
		{name: "CIDR entries", input: "10.0.0.0/8,192.168.0.0/16", expected: []string{"10.0.0.0/8", "192.168.0.0/16"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getProxies(tt.input))
		})
	}
}

func TestValidate_TrustedProxies(t *testing.T) {
	cfg := validConfig()
	cfg.TrustedProxies = []string{"10.0.0.1", "192.168.0.0/16", "not-an-ip-or-cidr"}
	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "TRUSTED_PROXIES"), "an invalid proxy string must be rejected, got: %v", errs)
}

// TestValidate_TrustedProxyCatchAllRejected pins issue #954's refusal of a
// trusted proxy that trusts every source: 0.0.0.0/0 (and its IPv6 twin ::/0)
// lets any client forge X-Forwarded-For, escaping the IP rate limiter and
// poisoning logged IPs. A normal CIDR must still be accepted.
func TestValidate_TrustedProxyCatchAllRejected(t *testing.T) {
	for _, proxy := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(proxy, func(t *testing.T) {
			cfg := validConfig()
			cfg.TrustedProxies = []string{proxy}
			errs := cfg.Validate()
			assert.True(t, hasFieldError(errs, "TRUSTED_PROXIES"), "catch-all proxy %q must be refused, got: %v", proxy, errs)
		})
	}

	cfg := validConfig()
	cfg.TrustedProxies = []string{"10.0.0.0/8", "172.16.0.0/12", "127.0.0.1/32"}
	assert.Empty(t, cfg.Validate(), "a bounded trusted-proxy CIDR must be accepted")
}

// TestEffectiveTrustedProxies pins the default-loopback fallback the shipped
// all-in-one image relies on (issue #954): with no TRUSTED_PROXIES the bundled
// nginx on 127.0.0.1 must still be trusted, or every client collapses into one
// bucket. An explicit list is passed through unchanged.
func TestEffectiveTrustedProxies(t *testing.T) {
	empty := &Config{}
	assert.Equal(t, []string{"127.0.0.1/32", "::1/128"}, empty.EffectiveTrustedProxies())

	configured := &Config{TrustedProxies: []string{"10.1.2.3"}}
	assert.Equal(t, []string{"10.1.2.3"}, configured.EffectiveTrustedProxies())
}

// TestTrustedProxyWarnings pins the advisory (non-fatal) boot warning: a release
// deployment with no configured proxies is told the fallback is loopback and
// that an external proxy must be listed. It must not fire in dev, and must not
// fire when the operator configured proxies.
func TestTrustedProxyWarnings(t *testing.T) {
	t.Setenv("GIN_MODE", "release")
	assert.Len(t, (&Config{}).TrustedProxyWarnings(), 1, "release + empty must warn")
	assert.Empty(t, (&Config{TrustedProxies: []string{"10.1.2.3"}}).TrustedProxyWarnings(), "release + configured must not warn")

	t.Setenv("GIN_MODE", "debug")
	assert.Empty(t, (&Config{}).TrustedProxyWarnings(), "dev must not warn")
}

func TestValidate_AttachmentsDirRelativeRejected(t *testing.T) {
	cfg := validConfig()
	cfg.AttachmentsDir = "relative/path"
	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "ATTACHMENTS_DIR"), "a relative ATTACHMENTS_DIR must be rejected, got: %v", errs)
}

func TestValidate_FCMServiceAccountFileMissing(t *testing.T) {
	cfg := validConfig()
	cfg.FCMServiceAccountFile = "/nonexistent/fcm-sa.json"
	errs := cfg.Validate()

	assert.True(t, hasFieldError(errs, "FCM_SERVICE_ACCOUNT_FILE"), "a missing FCM service-account file must be rejected, got: %v", errs)
}

func TestValidate_UseSMTPEnforcesRequiredFields(t *testing.T) {
	cfg := validConfig()
	cfg.UseSMTP = true
	cfg.SMTPHost = ""
	cfg.SMTPFromEmail = ""

	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "SMTP_HOST"))
	assert.True(t, hasFieldError(errs, "SMTP_FROM_EMAIL"))
}

func TestLoadConfig_InvalidJWTExpiryFallsBackToDefault(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("JWT_EXPIRY_HOURS", "not-a-number")

	cfg := LoadConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, 96, cfg.JWTExpiryHours, "an unparsable JWT_EXPIRY_HOURS must fall back to the default")
}

func TestLoadConfig_InvalidIntEnvFallsBack(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("HTTP_READ_TIMEOUT", "bogus")

	cfg := LoadConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, 15, cfg.ReadTimeout, "an unparsable HTTP_READ_TIMEOUT must fall back to the default")
}

// At-rest encryption master key validation (issue #380).

func TestLoadConfig_DataEncryptionKeyEnv(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	cfg := LoadConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, base64.StdEncoding.EncodeToString(make([]byte, 32)), cfg.DataEncryptionKey)
	assert.Empty(t, cfg.DataEncryptionKeyFile)
}

func TestValidate_DataEncryptionKey_ValidBase64Passes(t *testing.T) {
	cfg := validConfig()
	cfg.DataEncryptionKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	assert.Empty(t, cfg.Validate(), "a well-formed 32-byte base64 key must validate")
}

func TestValidate_DataEncryptionKey_InvalidBase64Fails(t *testing.T) {
	cfg := validConfig()
	cfg.DataEncryptionKey = "this-is-not-base64!!!"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "DATA_ENCRYPTION_KEY"), "a non-base64 key must error")
}

func TestValidate_DataEncryptionKey_WrongLengthFails(t *testing.T) {
	cfg := validConfig()
	cfg.DataEncryptionKey = base64.StdEncoding.EncodeToString(make([]byte, 16))
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "DATA_ENCRYPTION_KEY"), "a key that is not 32 bytes must error")
}

func TestValidate_DataEncryptionKeyFile_MissingFails(t *testing.T) {
	cfg := validConfig()
	cfg.DataEncryptionKeyFile = "/nonexistent/at-rest-key"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "DATA_ENCRYPTION_KEY_FILE"), "a nonexistent key file must error")
}

func TestValidate_DataEncryptionKey_AndFileConflict(t *testing.T) {
	cfg := validConfig()
	cfg.DataEncryptionKey = base64.StdEncoding.EncodeToString(make([]byte, 32))
	cfg.DataEncryptionKeyFile = "/tmp/somewhere"
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "DATA_ENCRYPTION_KEY"), "setting both key and key file must error")
}

// TestValidate_DataEncryptionKey_AgreesWithAtrest pins the issue #938 fix:
// config.Validate must accept/reject DATA_ENCRYPTION_KEY exactly where
// atrest.DecodeMasterKey (the function atrest.ResolveMasterKey actually
// calls to arm encryption) would, because Validate calls that same function
// rather than an independent, potentially-drifting base64/length check.
func TestValidate_DataEncryptionKey_AgreesWithAtrest(t *testing.T) {
	cases := []string{
		base64.StdEncoding.EncodeToString(make([]byte, 32)), // valid
		base64.StdEncoding.EncodeToString(make([]byte, 16)), // wrong length
		"this-is-not-base64!!!",                             // not base64
		"",
	}

	for _, raw := range cases {
		_, atrestErr := atrest.DecodeMasterKey(raw)
		atrestAccepts := raw == "" || atrestErr == nil // empty is "unset", handled separately by Validate

		cfg := validConfig()
		cfg.DataEncryptionKey = raw
		validateAccepts := !hasFieldError(cfg.Validate(), "DATA_ENCRYPTION_KEY")

		assert.Equal(t, atrestAccepts, validateAccepts,
			"Validate and atrest.DecodeMasterKey disagreed on %q", raw)
	}
}

func TestLoadConfig_DataEncryptionKeyFileEnv(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "/etc/mycorrhizal/at-rest.key")
	cfg := LoadConfig()
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.DataEncryptionKey)
	assert.Equal(t, "/etc/mycorrhizal/at-rest.key", cfg.DataEncryptionKeyFile)
}

// --- MIN_CLIENT_VERSION (issue #528 / docs/client-compatibility-policy.md) ---

// The compatibility floor is the policy's load-bearing surface: an empty
// value means "no floor has ever been declared" and must validate, while a
// set-but-garbled value must fail boot (an operator who thinks they raised
// the floor and didn't would ship an unadvertised breaking change).
func TestValidate_MinClientVersion(t *testing.T) {
	validFloors := []string{
		"0.6.0",
		"0.6",
		"1",
		"0.6.0-rc.1",
		"0.7.0+build.7",
		"v0.6.0",
	}
	for _, floor := range validFloors {
		t.Run("valid floor "+floor, func(t *testing.T) {
			cfg := validConfig()
			cfg.MinClientVersion = floor
			assert.False(t, hasFieldError(cfg.Validate(), "MIN_CLIENT_VERSION"),
				"%q must be accepted as a floor", floor)
		})
	}

	invalidFloors := []string{
		"",
		"banana",
		"0.6.0/../../etc",
		"latest",
		"0,6",
		"0.6.0.1", // four numeric segments: not a versionName shape
		"-rc.1",
		"0.6.",
	}
	for _, floor := range invalidFloors {
		if floor == "" {
			// Empty is the "no floor declared" default and IS valid.
			continue
		}
		t.Run("invalid floor "+floor, func(t *testing.T) {
			cfg := validConfig()
			cfg.MinClientVersion = floor
			assert.True(t, hasFieldError(cfg.Validate(), "MIN_CLIENT_VERSION"),
				"%q must be rejected as a floor", floor)
		})
	}

	t.Run("empty (the default) is valid — no floor declared", func(t *testing.T) {
		cfg := validConfig()
		cfg.MinClientVersion = ""
		assert.False(t, hasFieldError(cfg.Validate(), "MIN_CLIENT_VERSION"))
	})
}

func TestLoadConfig_MinClientVersionEnv(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	t.Run("unset defaults to empty (no floor)", func(t *testing.T) {
		t.Setenv("MIN_CLIENT_VERSION", "")
		assert.Equal(t, "", LoadConfig().MinClientVersion)
	})

	t.Run("set is read through", func(t *testing.T) {
		t.Setenv("MIN_CLIENT_VERSION", "0.6.0")
		assert.Equal(t, "0.6.0", LoadConfig().MinClientVersion)
	})
}

// AuthSpray defaults and clamping (issue #940). The velocity signal is a
// security control, so a missing/zero threshold resolves to the safe default
// rather than tripping on the first failure or silently disabling detection.
func TestLoadConfig_AuthSprayDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	cfg := LoadConfig()
	assert.True(t, cfg.AuthSprayEnabled)
	assert.Equal(t, 60, cfg.AuthSprayWindowSeconds)
	assert.Equal(t, 60, cfg.AuthSprayFailureThreshold)
	assert.Equal(t, 15, cfg.AuthSprayIdentifierThreshold)
	assert.Equal(t, 300, cfg.AuthSprayThrottleSeconds)
	assert.True(t, cfg.AlertAuthSprayEnabled)
}

func TestLoadConfig_AuthSprayEnvOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	t.Setenv("AUTH_SPRAY_ENABLED", "false")
	t.Setenv("AUTH_SPRAY_WINDOW_SECONDS", "120")
	t.Setenv("AUTH_SPRAY_FAILURE_THRESHOLD", "200")
	t.Setenv("AUTH_SPRAY_IDENTIFIER_THRESHOLD", "25")
	t.Setenv("AUTH_SPRAY_THROTTLE_SECONDS", "600")
	t.Setenv("ALERT_AUTH_SPRAY_ENABLED", "false")

	cfg := LoadConfig()
	assert.False(t, cfg.AuthSprayEnabled)
	assert.Equal(t, 120, cfg.AuthSprayWindowSeconds)
	assert.Equal(t, 200, cfg.AuthSprayFailureThreshold)
	assert.Equal(t, 25, cfg.AuthSprayIdentifierThreshold)
	assert.Equal(t, 600, cfg.AuthSprayThrottleSeconds)
	assert.False(t, cfg.AlertAuthSprayEnabled)
}

func TestLoadConfig_AuthSprayThresholdsClamped(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")

	t.Setenv("AUTH_SPRAY_WINDOW_SECONDS", "0")
	t.Setenv("AUTH_SPRAY_FAILURE_THRESHOLD", "-1")
	t.Setenv("AUTH_SPRAY_IDENTIFIER_THRESHOLD", "0")
	t.Setenv("AUTH_SPRAY_THROTTLE_SECONDS", "-5")

	cfg := LoadConfig()
	assert.Equal(t, 60, cfg.AuthSprayWindowSeconds)
	assert.Equal(t, 60, cfg.AuthSprayFailureThreshold)
	assert.Equal(t, 15, cfg.AuthSprayIdentifierThreshold)
	assert.Equal(t, 300, cfg.AuthSprayThrottleSeconds)
}

// --- OIDC configuration validation (issue #934) ---
//
// #501 claimed "no required value can be omitted silently"; OIDC being
// treated as enabled only when all three of OIDC_PROVIDER_URL/CLIENT_ID/
// CLIENT_SECRET are set, with a bare log line otherwise, was exactly that.
// These pin the replacement: any var set at all means SSO was intended, so
// an incomplete set is now a boot-failing ValidationError naming what's
// missing, and a complete-but-malformed set (bad URL, empty scope entry)
// fails too instead of only breaking at the first login attempt.

func TestValidate_OIDCUnsetIsFine(t *testing.T) {
	cfg := validConfig()
	cfg.OIDC = OIDCConfig{}
	errs := cfg.Validate()
	assert.False(t, hasFieldError(errs, "OIDC"), "no OIDC vars set must not error, got: %v", errs)
}

func TestValidate_OIDCPartialConfig(t *testing.T) {
	full := OIDCConfig{
		ProviderURL:  "https://idp.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       []string{"openid"},
	}

	tests := []struct {
		name string
		oidc OIDCConfig
	}{
		{"only provider URL set", OIDCConfig{ProviderURL: full.ProviderURL, Scopes: full.Scopes}},
		{"only client ID set", OIDCConfig{ClientID: full.ClientID, Scopes: full.Scopes}},
		{"only client secret set", OIDCConfig{ClientSecret: full.ClientSecret, Scopes: full.Scopes}},
		{"provider URL and client ID set, secret missing", OIDCConfig{ProviderURL: full.ProviderURL, ClientID: full.ClientID, Scopes: full.Scopes}},
		{"client ID and secret set, provider URL missing", OIDCConfig{ClientID: full.ClientID, ClientSecret: full.ClientSecret, Scopes: full.Scopes}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.OIDC = tt.oidc
			errs := cfg.Validate()
			assert.True(t, hasFieldError(errs, "OIDC"), "a partial OIDC config must fail boot naming what's missing, got: %v", errs)
		})
	}
}

func TestValidate_OIDCPartialConfig_NamesMissingVars(t *testing.T) {
	cfg := validConfig()
	cfg.OIDC = OIDCConfig{ProviderURL: "https://idp.example.com"}
	errs := cfg.Validate()

	require.True(t, hasFieldError(errs, "OIDC"))
	var msg string
	for _, e := range errs {
		if e.Field == "OIDC" {
			msg = e.Message
		}
	}
	assert.Contains(t, msg, "OIDC_CLIENT_ID")
	assert.Contains(t, msg, "OIDC_CLIENT_SECRET")
	assert.Contains(t, msg, "Set OIDC_CLIENT_ID, OIDC_CLIENT_SECRET too", "the missing-vars clause must list exactly the vars that are missing")
}

func TestValidate_OIDCCompleteConfigPasses(t *testing.T) {
	cfg := validConfig()
	cfg.OIDC = OIDCConfig{
		ProviderURL:  "https://idp.example.com/realms/main",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       []string{"openid", "email", "profile"},
	}
	errs := cfg.Validate()
	assert.False(t, hasFieldError(errs, "OIDC"), "a fully configured OIDC must not error, got: %v", errs)
	assert.False(t, hasFieldError(errs, "OIDC_PROVIDER_URL"), "got: %v", errs)
	assert.False(t, hasFieldError(errs, "OIDC_SCOPES"), "got: %v", errs)
}

func TestValidate_OIDCProviderURLFormat(t *testing.T) {
	tests := []struct {
		name        string
		providerURL string
		expectError bool
	}{
		{"https URL with path", "https://idp.example.com/realms/main", false},
		{"http URL (self-hosted LAN IdP)", "http://keycloak.internal:8080/realms/main", false},
		{"no scheme", "idp.example.com", true},
		{"unsupported scheme", "ftp://idp.example.com", true},
		{"scheme only, no host", "https://", true},
		{"garbage", "not a url at all", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.OIDC = OIDCConfig{
				ProviderURL:  tt.providerURL,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				Scopes:       []string{"openid"},
			}
			errs := cfg.Validate()
			if tt.expectError {
				assert.True(t, hasFieldError(errs, "OIDC_PROVIDER_URL"), "expected OIDC_PROVIDER_URL error for %q, got: %v", tt.providerURL, errs)
			} else {
				assert.False(t, hasFieldError(errs, "OIDC_PROVIDER_URL"), "did not expect OIDC_PROVIDER_URL error for %q, got: %v", tt.providerURL, errs)
			}
		})
	}
}

func TestValidate_OIDCScopesRejectsEmptyEntries(t *testing.T) {
	cfg := validConfig()
	cfg.OIDC = OIDCConfig{
		ProviderURL:  "https://idp.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		// getScopesEnv would produce exactly this shape from "openid,,email"
		// (a stray comma) or "openid, ,email" (a whitespace-only entry).
		Scopes: []string{"openid", "", "email"},
	}
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "OIDC_SCOPES"), "an empty scope entry must be rejected, got: %v", errs)
}

func TestValidate_OIDCNotEnabledSkipsFormatChecks(t *testing.T) {
	// A malformed provider URL sitting behind a partial config must surface
	// as the OIDC "incomplete" error, not also (or instead) as an
	// OIDC_PROVIDER_URL format error — Enabled gates the format checks so
	// they only run once the set is actually complete.
	cfg := validConfig()
	cfg.OIDC = OIDCConfig{ProviderURL: "not a url at all"}
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "OIDC"))
	assert.False(t, hasFieldError(errs, "OIDC_PROVIDER_URL"), "format check must not run until OIDC is fully configured, got: %v", errs)
}

func TestLoadConfig_OIDCPartialEnvFailsValidation(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-that-is-long-enough-32")
	t.Setenv("PROFILE_PHOTO_DIR", "/tmp/photos")
	t.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
	t.Setenv("OIDC_CLIENT_ID", "some-client-id")
	t.Setenv("OIDC_CLIENT_SECRET", "some-client-secret")
	// OIDC_PROVIDER_URL deliberately left unset.

	cfg := LoadConfig()
	require.False(t, cfg.OIDC.Enabled, "OIDC must not be treated as enabled with the provider URL missing")
	errs := cfg.Validate()
	assert.True(t, hasFieldError(errs, "OIDC"), "a partial OIDC env must fail Validate(), got: %v", errs)
}

// --- Public-exposure SSRF-guard boot warning (issue #951) ---
//
// Every *_BLOCK_PRIVATE_URLS flag defaults to off so a trusted-LAN
// self-host keeps working with zero config (see docs/int-01-integration-
// classification-matrix.md). PublicExposureWarnings is the advisory signal
// for the case that default doesn't cover: a deployment that looks
// reachable from outside a trusted LAN while a guard is still off. It must
// never appear in Validate()'s errors — it's advisory, like
// TrustedProxyWarnings, not a boot failure.

func TestPublicExposureWarnings_NotPublicFacingIsQuiet(t *testing.T) {
	cfg := &Config{FrontendURL: "http://localhost:7300", CookieSecure: false}
	assert.Empty(t, cfg.PublicExposureWarnings())
}

func TestPublicExposureWarnings_HTTPSFrontendWithGuardsOffWarns(t *testing.T) {
	cfg := &Config{FrontendURL: "https://crm.example.com", CookieSecure: true}
	warnings := cfg.PublicExposureWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "WEBHOOK_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "OIDC_BLOCK_PRIVATE_URLS")
}

func TestPublicExposureWarnings_CookieSecureAloneImpliesPublic(t *testing.T) {
	// COOKIE_SECURE=true with a non-https FrontendURL is an unusual but
	// legal combination (see TestValidate_CookieSecureTrueAlwaysAllowed) —
	// it alone must still be read as "this deployment intends HTTPS".
	cfg := &Config{FrontendURL: "http://localhost:7300", CookieSecure: true}
	assert.NotEmpty(t, cfg.PublicExposureWarnings())
}

func TestPublicExposureWarnings_AllGuardsOnIsQuiet(t *testing.T) {
	cfg := &Config{
		FrontendURL:               "https://crm.example.com",
		CookieSecure:              true,
		WebhookBlockPrivateURLs:   true,
		CalDAVBlockPrivateURLs:    true,
		ImmichBlockPrivateURLs:    true,
		PaperlessBlockPrivateURLs: true,
		SeafileBlockPrivateURLs:   true,
		WebDAVBlockPrivateURLs:    true,
		MonicaBlockPrivateURLs:    true,
		OIDC:                      OIDCConfig{BlockPrivateURLs: true},
	}
	assert.Empty(t, cfg.PublicExposureWarnings(), "every guard on must not warn")
}

func TestPublicExposureWarnings_NamesOnlyTheFlagsThatAreOff(t *testing.T) {
	cfg := &Config{
		FrontendURL:             "https://crm.example.com",
		CookieSecure:            true,
		WebhookBlockPrivateURLs: true, // this one is on and must not be named
	}
	warnings := cfg.PublicExposureWarnings()
	require.Len(t, warnings, 1)
	assert.NotContains(t, warnings[0], "WEBHOOK_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "CALDAV_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "IMMICH_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "PAPERLESS_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "SEAFILE_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "WEBDAV_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "MONICA_BLOCK_PRIVATE_URLS")
	assert.Contains(t, warnings[0], "OIDC_BLOCK_PRIVATE_URLS")
}

func TestPublicExposureWarnings_NeverSurfacesAsValidationError(t *testing.T) {
	// Advisory only — must never block boot the way TestValidate_ValidConfigHasNoErrors
	// pins for the rest of Validate().
	cfg := validConfig() // https FrontendURL + CookieSecure=true, every *_BLOCK_PRIVATE_URLS off
	assert.NotEmpty(t, cfg.PublicExposureWarnings(), "sanity: this config should actually warn")
	assert.Empty(t, cfg.Validate(), "PublicExposureWarnings must never leak into Validate()'s errors")
}
