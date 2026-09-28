package embedded

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

// unixHTTPClient is an http.Client that dials the server's Unix socket
// regardless of the request URL's host, the way the Android local profile's
// OkHttp SocketFactory routes Local-profile traffic (ADR 0028).
func unixHTTPClient(socket string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
}

// newTestConfig builds a validated Config programmatically (no env vars), the
// way an embedding host does. embedded selects the deployment shape.
func newTestConfig(t *testing.T, deployment string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	cfg, err := config.New(func(c *config.Config) {
		c.Deployment = deployment
		c.DBPath = filepath.Join(dir, "test.db")
		c.JWTSecretKey = "test-secret-key-that-is-definitely-long-enough"
		c.ProfilePhotoDir = filepath.Join(dir, "photos")
		c.AttachmentsDir = filepath.Join(dir, "attachments")
		c.DataEncryptionKey = base64.StdEncoding.EncodeToString(key)
		c.FrontendURL = "http://localhost:7300"
		c.GinMode = "test"
		c.LogLevel = "error"
	})
	require.NoError(t, err)
	return cfg
}

func listenUnix(t *testing.T, name string) (net.Listener, string) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), name)
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	return ln, socket
}

func getHealth(t *testing.T, client *http.Client) (int, map[string]any) {
	t.Helper()
	resp, err := client.Get("http://unix/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return resp.StatusCode, parsed
}

// TestStartStop_Lifecycle is the issue #1257 acceptance test: the server boots
// through the library entry point on a caller-supplied Unix listener against a
// t.TempDir() database, serves /health, and Stop leaves the scheduler stopped
// and the database closed.
//
// Hand-verified (CLAUDE.md): with the s.sched.Stop() line removed from Stop,
// this test fails on srv.sched.IsRunning() — the scheduler is still ticking —
// which is exactly the regression the assertion exists to catch.
func TestStartStop_Lifecycle(t *testing.T) {
	cfg := newTestConfig(t, config.DeploymentServer)
	ln, socket := listenUnix(t, "server.sock")

	srv, err := Start(context.Background(), cfg, Options{
		Listener:               ln,
		disableInitialTriggers: true, // keep the test deterministic
	})
	require.NoError(t, err)

	code, body := getHealth(t, unixHTTPClient(socket))
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, body["status"])

	require.NoError(t, srv.Stop(context.Background()))

	require.False(t, srv.sched.IsRunning(), "Stop must stop the scheduler")

	sqlDB, err := srv.db.DB()
	require.NoError(t, err)
	require.Error(t, sqlDB.Ping(), "Stop must close the database")
}

// TestStart_InvalidConfigReturnsError pins that a library caller gets an error
// rather than a process abort when the Config does not validate.
func TestStart_InvalidConfigReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DBPath:          filepath.Join(dir, "x.db"),
		ProfilePhotoDir: filepath.Join(dir, "photos"),
	}

	ln, _ := listenUnix(t, "invalid.sock")
	_, err := Start(context.Background(), cfg, Options{Listener: ln})
	require.Error(t, err)
	require.Contains(t, err.Error(), "JWT_SECRET_KEY")
}

// TestStart_EmbeddedRequiresListener pins that embedded mode never falls back
// to TCP loopback (ADR 0028: any app on the device can reach 127.0.0.1).
func TestStart_EmbeddedRequiresListener(t *testing.T) {
	cfg := newTestConfig(t, config.DeploymentEmbedded)
	_, err := Start(context.Background(), cfg, Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Listener")
}

// TestEmbedded_SingleUserRoutesJobsHealth is the issue #1258 acceptance test:
// boot embedded mode, assert exactly one user is provisioned (and only one
// after a second boot), every disabled route 404s, every disabled job is
// absent from the scheduler, and /health reports the embedded deployment with
// the reduced capability list.
func TestEmbedded_SingleUserRoutesJobsHealth(t *testing.T) {
	cfg := newTestConfig(t, config.DeploymentEmbedded)
	// Enabled in config but must still not be served in embedded mode.
	cfg.CardDAVEnabled = true
	cfg.CalDAVEnabled = true

	ln, socket := listenUnix(t, "embedded1.sock")
	srv, err := Start(context.Background(), cfg, Options{Listener: ln, CatchUpDelay: time.Hour})
	require.NoError(t, err)

	var count int64
	require.NoError(t, srv.DB().Model(&models.User{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "embedded must provision exactly one user")
	require.NotEmpty(t, srv.LocalSessionToken(), "the host must get a local session token")
	require.NotZero(t, srv.LocalUserID())

	client := unixHTTPClient(socket)

	// Every disabled surface is absent (404, not 403).
	disabled := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/register"},
		{http.MethodPost, "/api/v1/login"},
		{http.MethodPost, "/api/v1/login/2fa"},
		{http.MethodPost, "/api/v1/auth/device/session"},
		{http.MethodPost, "/api/v1/password-reset/request"},
		{http.MethodPost, "/api/v1/password-reset/confirm"},
		{http.MethodGet, "/api/v1/auth/oidc/config"},
		{http.MethodGet, "/api/v1/users/2fa/status"},
		{http.MethodGet, "/api/v1/api-tokens"},
		{http.MethodGet, "/api/v1/contact-shares/incoming"},
		{http.MethodGet, "/api/v1/webhooks"},
		{http.MethodGet, "/api/v1/auth/device/grants"},
		{http.MethodGet, "/api/v1/notifications/devices"},
		{http.MethodGet, "/api/v1/notifications/push-subscriptions"},
		{http.MethodGet, "/carddav/"},
		{http.MethodGet, "/caldav/"},
	}
	for _, d := range disabled {
		req, err := http.NewRequest(d.method, "http://unix"+d.path, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equalf(t, http.StatusNotFound, resp.StatusCode, "%s %s must be absent in embedded mode", d.method, d.path)
	}

	// An enabled protected route is registered (401 from auth, not 404).
	resp, err := client.Get("http://unix/api/v1/contacts")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "contacts must stay registered in embedded mode")

	// Disabled jobs are not on the scheduler; core jobs are.
	registered := map[string]bool{}
	for _, job := range srv.Jobs() {
		for _, tag := range job.Tags() {
			registered[tag] = true
		}
	}
	for _, job := range []string{
		models.JobNameRestoreDrill,
		models.JobNameAlertEval,
		models.JobNameWebhookRetries,
		models.JobNameWebhookDeliveryPurge,
		models.JobNameStorageSample,
	} {
		require.Falsef(t, registered[job], "embedded must not register the %q job", job)
	}
	for _, job := range []string{
		models.JobNameDailyReminders,
		models.JobNamePurgeDeleted,
		models.JobNameDBIntegrityCheck,
		models.JobNameCalendarSync,
		models.JobNameImmichSync,
	} {
		require.Truef(t, registered[job], "embedded must keep the %q job", job)
	}

	// /health reports the deployment shape and the reduced capability list.
	code, health := getHealth(t, client)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, config.DeploymentEmbedded, health["deployment"])

	caps, ok := health["capabilities"].([]any)
	require.True(t, ok, "capabilities must be an array")
	capSet := map[string]bool{}
	for _, c := range caps {
		capSet[c.(string)] = true
	}
	require.True(t, capSet[config.CapabilityContacts], "capabilities must include contacts")
	require.False(t, capSet[config.CapabilityRegistration], "capabilities must omit registration")
	require.False(t, capSet[config.CapabilityCardDAV], "capabilities must omit carddav")

	require.NoError(t, srv.Stop(context.Background()))

	// Second boot against the same database must not create a second user.
	ln2, _ := listenUnix(t, "embedded2.sock")
	srv2, err := Start(context.Background(), cfg, Options{Listener: ln2, CatchUpDelay: time.Hour})
	require.NoError(t, err)
	defer func() { require.NoError(t, srv2.Stop(context.Background())) }()

	require.NoError(t, srv2.DB().Model(&models.User{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "a second embedded boot must reuse the single user")
	require.NotEmpty(t, srv2.LocalSessionToken())
}

// TestStart_ServerModeDefaultsToTCPWhenNoListener pins that server mode keeps
// the old behavior when no listener is supplied: it listens on :PORT (the
// address is observable through Addr) instead of rejecting the way embedded
// mode does.
func TestStart_ServerModeDefaultsToTCPWhenNoListener(t *testing.T) {
	// Reserve a free port, then release it for Start to bind. A tiny race, but
	// the alternative (PORT=0) is deliberately rejected by config validation.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	cfg := newTestConfig(t, config.DeploymentServer)
	cfg.Port = strconv.Itoa(port)

	srv, err := Start(context.Background(), cfg, Options{disableInitialTriggers: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, srv.Stop(context.Background())) }()

	require.NotNil(t, srv.Addr())
	require.Equal(t, port, srv.Addr().(*net.TCPAddr).Port)
}
