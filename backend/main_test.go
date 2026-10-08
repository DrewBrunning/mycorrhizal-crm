package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mycorrhizal/embedded"
)

// Issue #1339: the argv selection main() does (`--embedded-host`) is the one
// thing the Android host depends on that no in-process test can see —
// embedded.RunHosted is tested directly, and LocalOnlyModeE2eTest skips on the
// x86_64 CI emulator. So these tests build and exec the real binary for the
// host OS/arch and drive it exactly as android/.../LocalServerHost.kt does.

var (
	binOnce sync.Once
	binDir  string // set as soon as the build directory exists, so a failed build is cleaned up too
	binPath string
	binErr  string
)

// TestMain removes the binary hostBinary built: it lives in a MkdirTemp dir
// created inside a sync.Once, so no t.Cleanup can own it (issue #1555).
func TestMain(m *testing.M) {
	code := m.Run()
	removeHostBinary()
	os.Exit(code)
}

// removeHostBinary deletes hostBinary's build directory, if one was created —
// including when the build itself failed and binPath was never set.
func removeHostBinary() {
	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}
}

// hostBinary builds the backend binary once per test run.
func hostBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the embedded host serves a Unix socket; not built for windows")
	}
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "embhost-bin")
		if err != nil {
			binErr = err.Error()
			return
		}
		binDir = dir
		out := filepath.Join(dir, "mycorrhizal")
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, ".")
		if b, err := cmd.CombinedOutput(); err != nil {
			binErr = err.Error() + "\n" + string(b)
			return
		}
		binPath = out
	})
	require.Empty(t, binErr, "building the backend binary failed")
	return binPath
}

// shortTempDir avoids t.TempDir(): a Unix socket path is limited to ~104 bytes
// and t.TempDir embeds the (long) test name.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "emb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestRemoveHostBinaryDeletesBuildDir(t *testing.T) {
	dir := shortTempDir(t)
	f := filepath.Join(dir, "mycorrhizal")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	old := binDir
	defer func() { binDir = old }()

	binDir = ""
	removeHostBinary() // no-op: must not remove anything
	_, err := os.Stat(f)
	require.NoError(t, err)

	// A failed build leaves binPath empty but the directory created: it is
	// still removed (issue #1555 review).
	binDir = dir
	removeHostBinary()
	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))
}

func hostConfigJSON(t *testing.T, dir string) (embedded.HostConfig, []byte) {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	hc := embedded.HostConfig{
		JWTSecret:         "argv-test-secret-key-that-is-long-enough",
		DataEncryptionKey: base64.StdEncoding.EncodeToString(key),
		SocketPath:        filepath.Join(dir, "sock"),
		DBPath:            filepath.Join(dir, "data", "mycorrhizal.db"),
		ProfilePhotoDir:   filepath.Join(dir, "data", "photos"),
		AttachmentsDir:    filepath.Join(dir, "data", "attachments"),
	}
	// The Android host creates the app-private data dir before exec.
	require.NoError(t, os.MkdirAll(filepath.Dir(hc.DBPath), 0o700))
	raw, err := json.Marshal(hc)
	require.NoError(t, err)
	return hc, raw
}

// TestEmbeddedHostBinary_BootsServesAndStops execs the real binary with
// --embedded-host, feeds it a HostConfig on stdin, waits for the readiness
// handshake, probes /health over the Unix socket, makes an authenticated call
// with the minted token, then SIGTERMs it (as Process.destroy() does) and
// expects a clean exit.
func TestEmbeddedHostBinary_BootsServesAndStops(t *testing.T) {
	bin := hostBinary(t)
	dir := shortTempDir(t)
	hc, raw := hostConfigJSON(t, dir)

	cmd := exec.Command(bin, embeddedHostArg)
	cmd.Stdin = bytes.NewReader(raw)
	// Deliberately a minimal environment: the secrets must come from stdin.
	cmd.Env = []string{"HOME=" + dir}
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // no-op if already exited
	})

	// Readiness handshake: one stdout line {"host_ready":true,"session_token":…}
	// among the server's own log lines. Bounded by a timeout.
	tokenCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var h struct {
				HostReady    bool   `json:"host_ready"`
				SessionToken string `json:"session_token"`
			}
			if json.Unmarshal(sc.Bytes(), &h) == nil && h.HostReady {
				tokenCh <- h.SessionToken
				break
			}
		}
		// Keep draining so the child never blocks on a full pipe.
		for sc.Scan() {
		}
	}()

	var token string
	select {
	case token = <-tokenCh:
	case err := <-exited:
		t.Fatalf("binary exited before the readiness handshake: %v\nstderr: %s", err, stderr.String())
	case <-time.After(60 * time.Second):
		t.Fatalf("no readiness handshake within 60s\nstderr: %s", stderr.String())
	}
	require.NotEmpty(t, token, "the handshake must carry a session token")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", hc.SocketPath)
		}},
	}

	resp, err := client.Get("http://unix/health")
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req, err := http.NewRequest(http.MethodGet, "http://unix/api/v1/contacts", nil)
	require.NoError(t, err)
	unauth, err := client.Do(req)
	require.NoError(t, err)
	_ = unauth.Body.Close()
	require.Equal(t, http.StatusUnauthorized, unauth.StatusCode, "the API must require the token")

	req.Header.Set("Authorization", "Bearer "+token)
	auth, err := client.Do(req)
	require.NoError(t, err)
	_ = auth.Body.Close()
	require.Equal(t, http.StatusOK, auth.StatusCode, "the minted session token must authenticate")

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	select {
	case err := <-exited:
		require.NoError(t, err, "SIGTERM must produce a clean exit\nstderr: %s", stderr.String())
	case <-time.After(45 * time.Second):
		t.Fatal("binary did not exit within 45s of SIGTERM")
	}
}

// TestEmbeddedHostBinary_WithoutArgFallsBackToEnvConfig is the #1336
// regression: exec'd with no argv the binary takes the env-configured server
// path and dies on the missing JWT_SECRET_KEY — which is what the Android host
// did before it passed --embedded-host.
func TestEmbeddedHostBinary_WithoutArgFallsBackToEnvConfig(t *testing.T) {
	bin := hostBinary(t)
	dir := shortTempDir(t)
	_, raw := hostConfigJSON(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = []string{"HOME=" + dir}
	out, err := cmd.CombinedOutput()

	var ee *exec.ExitError
	require.ErrorAs(t, err, &ee, "without the arg the binary must exit non-zero")
	require.NotZero(t, ee.ExitCode())
	require.Contains(t, string(out), "JWT_SECRET_KEY")
	require.NoFileExists(t, filepath.Join(dir, "sock"), "no embedded server may have started")
}

var kotlinHostArgPattern = regexp.MustCompile(`(?m)^\s*internal\s+const\s+val\s+EMBEDDED_HOST_ARG\s*=\s*"([^"]*)"`)

// TestEmbeddedHostArg_MatchesAndroidHost pins the Kotlin EMBEDDED_HOST_ARG to
// this file's embeddedHostArg; they were tied only by a "Must match" comment.
func TestEmbeddedHostArg_MatchesAndroidHost(t *testing.T) {
	src, err := os.ReadFile("../android/core/data/src/main/kotlin/com/mycorrhizal/crm/data/local/LocalServerHost.kt")
	require.NoError(t, err)
	m := kotlinHostArgPattern.FindSubmatch(src)
	require.NotNil(t, m, "EMBEDDED_HOST_ARG constant not found in LocalServerHost.kt (format changed?)")
	require.Equal(t, embeddedHostArg, strings.TrimSpace(string(m[1])),
		"LocalServerHost.kt EMBEDDED_HOST_ARG must equal backend/main.go embeddedHostArg")
}
