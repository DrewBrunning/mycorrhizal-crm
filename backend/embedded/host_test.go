package embedded

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"

	"github.com/stretchr/testify/require"
)

func hostConfigForTest(t *testing.T) HostConfig {
	t.Helper()
	dir := t.TempDir()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return HostConfig{
		JWTSecret:         "embedded-host-secret-key-that-is-long-enough",
		DataEncryptionKey: base64.StdEncoding.EncodeToString(key),
		SocketPath:        filepath.Join(dir, "local-server", "sock"),
		DBPath:            filepath.Join(dir, "local-server", "mycorrhizal.db"),
		ProfilePhotoDir:   filepath.Join(dir, "local-server", "photos"),
		AttachmentsDir:    filepath.Join(dir, "local-server", "attachments"),
	}
}

func TestReadHostConfig_Valid(t *testing.T) {
	t.Parallel()
	hc := hostConfigForTest(t)
	hc.CatchUpDelayMS = 2500

	raw, err := json.Marshal(hc)
	require.NoError(t, err)

	got, err := ReadHostConfig(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, hc, got)
}

func TestReadHostConfig_RejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	hc := hostConfigForTest(t)
	raw, err := json.Marshal(hc)
	require.NoError(t, err)

	cases := map[string]string{
		"jwt_secret":          "jwt_secret",
		"data_encryption_key": "data_encryption_key",
		"socket_path":         "socket_path",
		"db_path":             "db_path",
		"profile_photo_dir":   "profile_photo_dir",
		"attachments_dir":     "attachments_dir",
	}
	for field, named := range cases {
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		delete(m, field)
		blob, err := json.Marshal(m)
		require.NoError(t, err)

		_, err = ReadHostConfig(bytes.NewReader(blob))
		require.Error(t, err, "missing %s must be rejected", field)
		require.Contains(t, err.Error(), named)
	}
}

// TestReadHostConfig_RejectsUnknownFields guards the host/child contract
// against silent drift: a field the app starts sending that the binary does not
// understand would otherwise be ignored, leaving the server half-configured.
func TestReadHostConfig_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	hc := hostConfigForTest(t)
	raw, err := json.Marshal(hc)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	m["something_new"] = "surprise"
	blob, err := json.Marshal(m)
	require.NoError(t, err)

	_, err = ReadHostConfig(bytes.NewReader(blob))
	require.Error(t, err)
	require.Contains(t, err.Error(), "something_new")
}

func TestReadHostConfig_RejectsTrailingData(t *testing.T) {
	t.Parallel()
	hc := hostConfigForTest(t)
	raw, err := json.Marshal(hc)
	require.NoError(t, err)

	_, err = ReadHostConfig(bytes.NewReader(append(raw, []byte(`{"extra":true}`)...)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing data")
}

// TestHostConfig_SecretsNeverComeFromEnvironment pins ADR 0028 Decision 2: the
// JWT secret and master key are taken from the payload, never the process
// environment. A stray server-mode env var must not override the host's values,
// or a device's local sessions could be signed with a key the app never chose.
func TestHostConfig_SecretsNeverComeFromEnvironment(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "env-secret-that-must-be-ignored")
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))

	hc := hostConfigForTest(t)
	cfg, err := hc.Config()
	require.NoError(t, err)

	require.Equal(t, hc.JWTSecret, cfg.JWTSecretKey)
	require.Equal(t, hc.DataEncryptionKey, cfg.DataEncryptionKey)
	require.True(t, cfg.IsEmbedded())
	require.NotEqual(t, "env-secret-that-must-be-ignored", cfg.JWTSecretKey)
}

func TestHostConfig_ConfigAppliesDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := hostConfigForTest(t).Config()
	require.NoError(t, err)
	require.Equal(t, config.DeploymentEmbedded, cfg.Deployment)
	require.Equal(t, "8080", cfg.Port)
	require.Equal(t, 96, cfg.JWTExpiryHours)
}

// TestRunHosted_ServesEmbeddedHealthOverUnixSocket is the acceptance path for
// the packaged binary: a HostConfig goes in, the embedded deployment comes up
// on the Unix socket, and cancelling the context stops it. The Android host's
// own liveness probe is GET /health over that socket.
func TestRunHosted_ServesEmbeddedHealthOverUnixSocket(t *testing.T) {
	dir := t.TempDir()
	hc := hostConfigForTest(t)
	hc.SocketPath = filepath.Join(dir, "sock")
	hc.DBPath = filepath.Join(dir, "myco.db")
	hc.ProfilePhotoDir = filepath.Join(dir, "photos")
	hc.AttachmentsDir = filepath.Join(dir, "attachments")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan string, 1)
	go func() {
		done <- RunHosted(ctx, hc, func(srv *Server) { ready <- srv.LocalSessionToken() })
	}()

	var sessionToken string
	select {
	case sessionToken = <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("server never became ready")
	}
	// The host reads this token from the ready handshake and stores it as the
	// Local profile's credential; an empty one would strand the local session.
	require.NotEmpty(t, sessionToken)

	client := unixHTTPClient(hc.SocketPath)
	code, body := getHealth(t, client)
	require.Equal(t, 200, code)
	require.Equal(t, config.DeploymentEmbedded, body["deployment"])
	caps, ok := body["capabilities"].([]any)
	require.True(t, ok, "capabilities must be a JSON array")
	require.NotEmpty(t, caps)
	// Embedded drops the network-only surfaces.
	for _, token := range caps {
		require.NotEqual(t, config.CapabilityLogin, token)
		require.NotEqual(t, config.CapabilityCardDAV, token)
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("RunHosted did not stop after context cancellation")
	}
}

// TestRunHosted_RemovesStaleSocket covers the killed-process case: a leftover
// socket file must not make the next boot fail to bind.
func TestRunHosted_RemovesStaleSocket(t *testing.T) {
	dir := t.TempDir()
	hc := hostConfigForTest(t)
	hc.SocketPath = filepath.Join(dir, "sock")
	hc.DBPath = filepath.Join(dir, "myco.db")
	hc.ProfilePhotoDir = filepath.Join(dir, "photos")
	hc.AttachmentsDir = filepath.Join(dir, "attachments")

	// Pretend a crashed process left a socket file behind.
	require.NoError(t, os.WriteFile(hc.SocketPath, []byte("not really a socket"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- RunHosted(ctx, hc, func(*Server) { close(ready) })
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("server never became ready over a stale socket path")
	}
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("RunHosted did not stop")
	}
}

func TestReadHostConfig_OversizedPayloadRejected(t *testing.T) {
	t.Parallel()
	// A payload over the cap is truncated by the LimitReader and fails decode;
	// either way it must not allocate without bound.
	big := `{"jwt_secret":"` + strings.Repeat("a", maxHostConfigBytes+1) + `"}`
	_, err := ReadHostConfig(strings.NewReader(big))
	require.Error(t, err)
}
