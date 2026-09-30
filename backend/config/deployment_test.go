package config

import (
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// embeddedOverrides returns a minimal set of overrides that make an embedded
// Config pass validation, so the deployment tests can focus on deployment
// behavior rather than every required field.
func embeddedOverrides(t *testing.T, dir string) func(*Config) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return func(c *Config) {
		c.Deployment = DeploymentEmbedded
		c.DBPath = filepath.Join(dir, "embedded.db")
		c.JWTSecretKey = "test-embedded-secret-key-that-is-long-enough"
		c.ProfilePhotoDir = filepath.Join(dir, "photos")
		c.AttachmentsDir = filepath.Join(dir, "attachments")
		c.DataEncryptionKey = base64.StdEncoding.EncodeToString(key)
		c.GinMode = "test"
	}
}

func TestIsEmbedded_ZeroValueIsServer(t *testing.T) {
	t.Parallel()
	var c Config
	if c.IsEmbedded() {
		t.Fatal("a zero-value Config must behave like server mode")
	}
	if c.Deployment == DeploymentEmbedded {
		t.Fatal("zero value must not equal the embedded marker")
	}
}

func TestNew_EmbeddedBuildsValidatedConfig(t *testing.T) {
	t.Parallel()
	cfg, err := New(embeddedOverrides(t, t.TempDir()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !cfg.IsEmbedded() {
		t.Fatalf("Deployment = %q, want %q", cfg.Deployment, DeploymentEmbedded)
	}
	// Defaults are applied (not left empty), so a host only overrides what it
	// must.
	if cfg.Port != "8080" || cfg.ReadTimeout != 15 || cfg.JWTExpiryHours != 96 {
		t.Fatalf("programmatic defaults not applied: %+v", cfg)
	}
}

func TestNew_RunsTheSameValidationRules(t *testing.T) {
	t.Parallel()
	// No JWT secret and no profile photo dir: Validate would reject both, and
	// New must surface that as an error, not a usable Config.
	_, err := New(func(c *Config) {
		c.Deployment = DeploymentEmbedded
		c.DBPath = filepath.Join(t.TempDir(), "x.db")
	})
	if err == nil {
		t.Fatal("New must return an error for an invalid Config")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET_KEY") {
		t.Fatalf("error does not name JWT_SECRET_KEY: %v", err)
	}
}

func TestNew_EmbeddedRequiresMasterKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := New(func(c *Config) {
		c.Deployment = DeploymentEmbedded
		c.DBPath = filepath.Join(dir, "x.db")
		c.JWTSecretKey = "test-embedded-secret-key-that-is-long-enough"
		c.ProfilePhotoDir = filepath.Join(dir, "photos")
		c.AttachmentsDir = filepath.Join(dir, "attachments")
		c.GinMode = "test"
		// DataEncryptionKey deliberately unset: embedded has no HKDF fallback.
	})
	if err == nil {
		t.Fatal("embedded mode without a master key must fail validation")
	}
	if !strings.Contains(err.Error(), "DataEncryptionKey") {
		t.Fatalf("error does not name DataEncryptionKey: %v", err)
	}
}

func TestNew_ServerModeStillDerivesMasterKeyFromJWT(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := New(func(c *Config) {
		c.DBPath = filepath.Join(dir, "x.db")
		c.JWTSecretKey = "test-server-secret-key-that-is-long-enough"
		c.ProfilePhotoDir = filepath.Join(dir, "photos")
		c.AttachmentsDir = filepath.Join(dir, "attachments")
		c.GinMode = "test"
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.DataEncryptionKey != "" || cfg.DataEncryptionKeyFile != "" {
		t.Fatalf("server mode should not require a dedicated master key: %+v", cfg)
	}
}

func TestNew_RejectsUnknownDeployment(t *testing.T) {
	t.Parallel()
	_, err := New(func(c *Config) {
		c.Deployment = "laptop"
	})
	if err == nil || !strings.Contains(err.Error(), "deployment") {
		t.Fatalf("unknown deployment must be rejected, got %v", err)
	}
}

func TestCapabilities_ServerListsEveryTokenEmbeddedDropsTheDisabledOnes(t *testing.T) {
	t.Parallel()

	var server Config
	serverCaps := server.Capabilities()
	if len(serverCaps) != len(serverCapabilityTokens) {
		t.Fatalf("server capabilities = %v, want all %d tokens", serverCaps, len(serverCapabilityTokens))
	}
	if serverCaps == nil {
		t.Fatal("Capabilities must never return nil (the JSON field must be an array, not null)")
	}

	embedded := Config{Deployment: DeploymentEmbedded}
	embeddedCaps := embedded.Capabilities()
	embeddedSet := map[string]bool{}
	for _, token := range embeddedCaps {
		embeddedSet[token] = true
	}

	for _, token := range []string{
		CapabilityRegistration, CapabilityLogin, CapabilityPasswordReset,
		CapabilityOIDC, CapabilityTwoFactor, CapabilityEmail, CapabilityAPITokens,
		CapabilityContactShares, CapabilityWebhooks, CapabilityCardDAV,
		CapabilityCalDAV, CapabilityDeviceGrants, CapabilityPush,
		// Storage-only (ADR 0028 amendment, issue #1367).
		CapabilityCalendar, CapabilityNotifications,
	} {
		if embeddedSet[token] {
			t.Errorf("embedded capabilities must not include %q", token)
		}
	}

	for _, token := range []string{
		CapabilityContacts, CapabilityDashboard, CapabilityNotes, CapabilityReminders,
		CapabilityImport, CapabilityExport, CapabilitySearch,
	} {
		if !embeddedSet[token] {
			t.Errorf("embedded capabilities must include %q", token)
		}
	}

	// Exact set: a new token must be added here deliberately (and to the
	// Android ServerCapability mirror), never by accident.
	wantEmbedded := []string{
		CapabilityContacts, CapabilityDashboard, CapabilityNotes, CapabilityActivities,
		CapabilityReminders, CapabilityLifeEvents, CapabilityGraph, CapabilitySearch,
		CapabilityImport, CapabilityExport,
	}
	if !reflect.DeepEqual(embeddedCaps, wantEmbedded) {
		t.Errorf("embedded capabilities = %v, want exactly %v", embeddedCaps, wantEmbedded)
	}
}

// TestCapabilities_NoTokenCollidesWithAJobName guards an accidental
// information-leak regression: the unauthenticated /health body must not
// contain an internal scheduled-job name, and capability tokens share that
// body (see controllers' health endpoint tests).
func TestCapabilities_NoTokenCollidesWithAJobName(t *testing.T) {
	t.Parallel()
	// The full set of models.JobName* values, kept as literals so this test
	// does not import models (config is imported by models' consumers, so a
	// config -> models import would be a cycle risk).
	jobNames := []string{
		"daily_reminders", "calendar_sync", "webhook_retries", "purge_deleted",
		"cadence_overdue", "immich_sync", "audit_purge", "reach_out_detection",
		"db_integrity_check", "restore_drill", "system_event_purge",
		"webhook_delivery_purge", "idempotency_key_purge", "session_purge",
		"alert_eval", "job_run_purge", "storage_sample", "search_index_rebuild",
		"derived_columns_rebuild",
	}
	var server Config
	for _, token := range server.Capabilities() {
		for _, job := range jobNames {
			if token == job {
				t.Errorf("capability token %q collides with a job name; rename it", token)
			}
		}
	}
}
