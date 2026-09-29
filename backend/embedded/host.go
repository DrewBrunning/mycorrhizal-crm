package embedded

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
)

// HostConfig is what an embedding host (the Android app, ADR 0028 Decision 2)
// hands to the packaged server process. Every field is a value the host already
// owns: the app-private paths it chose, and — critically — the two secrets it
// generated and wrapped with Android Keystore (the Kotlin `LocalServerSecrets`).
// It is read from the process's private stdin, never the environment: env vars
// are world-readable on some platforms and are inherited by every child, and
// ADR 0028 forbids the secrets there.
//
// The non-secret paths could have come from the environment, but keeping the
// whole payload in one framed document makes the host/child contract a single
// typed struct (and a single thing to validate) rather than two half-channels.
type HostConfig struct {
	// JWTSecret signs the single local user's sessions. Required.
	JWTSecret string `json:"jwt_secret"`
	// DataEncryptionKey is the base64-encoded 32-byte at-rest master key.
	// Required: embedded mode has no HKDF-from-JWT fallback (config.Validate).
	DataEncryptionKey string `json:"data_encryption_key"`
	// SocketPath is the Unix domain socket the server serves on, inside
	// app-private storage. Required.
	SocketPath string `json:"socket_path"`
	// DBPath is the SQLite database file. Required.
	DBPath string `json:"db_path"`
	// ProfilePhotoDir is where profile photos are written. Required.
	ProfilePhotoDir string `json:"profile_photo_dir"`
	// AttachmentsDir is where contact attachments are written. Required.
	AttachmentsDir string `json:"attachments_dir"`
	// CatchUpDelayMS defers the boot-time catch-up burst by this many
	// milliseconds so the first request is not queued behind it (spike #1256).
	// Zero means run immediately (tests).
	CatchUpDelayMS int64 `json:"catch_up_delay_ms,omitempty"`
}

// maxHostConfigBytes bounds what a misbehaving or hostile stdin can make the
// child read. The payload is a handful of paths and two short secrets.
const maxHostConfigBytes = 64 << 10

// embeddedJWTExpiryHours is the maximum JWT_EXPIRY_HOURS the config validator
// accepts (one year); see HostConfig.Config.
const embeddedJWTExpiryHours = 8760

// hostStopTimeout bounds the graceful shutdown after the host signals stop.
const hostStopTimeout = 30 * time.Second

// ReadHostConfig decodes the host payload from r. It is strict: unknown fields
// and trailing data are rejected so the Android host and this parser cannot
// silently drift, and a payload with missing required fields is refused with an
// error naming them rather than a half-configured server boot.
func ReadHostConfig(r io.Reader) (HostConfig, error) {
	var hc HostConfig
	dec := json.NewDecoder(io.LimitReader(r, maxHostConfigBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&hc); err != nil {
		return HostConfig{}, fmt.Errorf("embedded: reading host config: %w", err)
	}
	// Reject trailing bytes: one exact JSON document, nothing more.
	if dec.More() {
		return HostConfig{}, errors.New("embedded: host config has trailing data after the JSON document")
	}
	if err := hc.Validate(); err != nil {
		return HostConfig{}, err
	}
	return hc, nil
}

// Validate checks the payload is complete enough to boot an embedded server.
// It deliberately does not check the secrets' strength beyond non-empty: the
// host generated them from a CSPRNG, and config.New enforces the master key's
// 32-byte shape.
func (h HostConfig) Validate() error {
	switch {
	case h.JWTSecret == "":
		return errors.New("embedded: host config is missing jwt_secret")
	case h.DataEncryptionKey == "":
		return errors.New("embedded: host config is missing data_encryption_key")
	case h.SocketPath == "":
		return errors.New("embedded: host config is missing socket_path")
	case h.DBPath == "":
		return errors.New("embedded: host config is missing db_path")
	case h.ProfilePhotoDir == "":
		return errors.New("embedded: host config is missing profile_photo_dir")
	case h.AttachmentsDir == "":
		return errors.New("embedded: host config is missing attachments_dir")
	}
	return nil
}

// Config builds the programmatic *config.Config this HostConfig describes. It
// never reads the environment, so the Android host's secrets cannot leak into
// (or be overridden by) the child's process environment.
func (h HostConfig) Config() (*config.Config, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	cfg, err := config.New(func(c *config.Config) {
		c.Deployment = config.DeploymentEmbedded
		c.JWTSecretKey = h.JWTSecret
		c.DataEncryptionKey = h.DataEncryptionKey
		c.DBPath = h.DBPath
		c.ProfilePhotoDir = h.ProfilePhotoDir
		c.AttachmentsDir = h.AttachmentsDir
		// Embedded mode serves only the Unix socket. A localhost FrontendURL
		// and the default port satisfy validation; no TCP listener is opened.
		c.FrontendURL = "http://localhost"
		c.GinMode = "release"
		c.LogLevel = "info"
		// Issue #1312: the single local user's session is minted once per Start and
		// travels only over the parent/child pipe to an app-private Unix socket, so
		// the server-mode idle timeout (12h) and 96h expiry protect nothing here,
		// and a Local profile has no login surface to recover from a 401. Disable
		// idle enforcement (0 is the documented "disabled" value) and use the
		// range maximum for the absolute expiry. Server-mode defaults are untouched.
		c.SessionIdleTimeoutHours = 0
		c.JWTExpiryHours = embeddedJWTExpiryHours
	})
	if err != nil {
		return nil, fmt.Errorf("embedded: building host config: %w", err)
	}
	return cfg, nil
}

// RunHosted is the packaged binary's embedded entry point: it builds the
// embedded Config, binds the Unix socket at SocketPath (removing a stale file
// first — the host guarantees the path is inside its own data directory), and
// runs the server until ctx is cancelled, then stops it gracefully. ready, when
// non-nil, is called with the running server so a caller can record the socket
// path; the packaged binary's ready callback (main.go runEmbeddedHost) also writes the
// {"host_ready":true,"session_token":…} line to stdout, and the host polls GET /health.
//
// Process death is also a stop path: the Android host signals SIGTERM
// (Process.destroy()) and cancels the same context, so Stop runs on both.
func RunHosted(ctx context.Context, hc HostConfig, ready func(*Server)) error {
	cfg, err := hc.Config()
	if err != nil {
		return err
	}

	// A leftover socket from a killed process makes bind fail with
	// EADDRINUSE; the host owns the directory, so removing it is safe.
	if hc.SocketPath != "" {
		_ = os.Remove(hc.SocketPath)
	}

	ln, err := net.Listen("unix", hc.SocketPath)
	if err != nil {
		return fmt.Errorf("embedded: listening on %s: %w", hc.SocketPath, err)
	}

	opts := Options{Listener: ln}
	if hc.CatchUpDelayMS > 0 {
		opts.CatchUpDelay = time.Duration(hc.CatchUpDelayMS) * time.Millisecond
	}

	srv, err := Start(ctx, cfg, opts)
	if err != nil {
		_ = ln.Close()
		return err
	}
	if ready != nil {
		ready(srv)
	}

	logger.Info().Str("socket", hc.SocketPath).Msg("Embedded server ready")

	<-ctx.Done()
	logger.Info().Msg("Embedded host context cancelled; stopping server")

	stopCtx, cancel := context.WithTimeout(context.Background(), hostStopTimeout)
	defer cancel()
	return srv.Stop(stopCtx)
}
