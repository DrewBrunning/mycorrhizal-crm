package config

import (
	"testing"
	"time"

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

	// Default reminder schedule. Pinned because this value is also stated in
	// three places outside the code — .env.example, backend/.env.example and
	// docs/getting-started.md — and they had already drifted apart once
	// (12:00 in code and docs, 06:00 in both env samples).
	assert.Equal(t, "06:00", cfg.ReminderTime)
	assert.Equal(t, "UTC", cfg.ReminderTimezone)
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
