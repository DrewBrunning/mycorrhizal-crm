package logger

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestFromContextAttachesUserID pins the gin context key FromContext reads for
// the acting user. The auth middleware stores it under "userID"
// (middleware/auth.go); FromContext read "user_id" for a long time and so
// silently attached no user_id to any request-scoped log line (issue #425).
func TestFromContextAttachesUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buf := captureLogger(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/contacts", nil)
	c.Set("request_id", "req-1")
	c.Set("userID", uint(42))

	FromContext(c).Info().Msg("request")

	line := lastLine(t, buf)
	require.Equal(t, float64(42), line[FieldUserID])
	require.Equal(t, "req-1", line[FieldRequestID])
	require.Equal(t, "req-1", line[FieldCorrelationID])
}

// TestFromContextSanitizesPath covers the FromContext path field wiring
// (logger.go): c.Request.URL.Path is user-controlled and every controller
// logs through FromContext, so it must pass through SanitizeLogField before
// landing in a log line. The %0a in the URL decodes to a raw newline in
// URL.Path. Note that zerolog also escapes structured field values in both
// JSON and console output (the PR's own finding), so the control-free
// assertion here is defense-in-depth documentation rather than the sole
// protection — this test's job is to keep line 107's SanitizeLogField call
// exercised and reviewed, matching the middleware logging test's coverage.
func TestFromContextSanitizesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buf := &bytes.Buffer{}
	oldLogger := Logger
	oldLevel := zerolog.GlobalLevel()
	Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/%0aFAKE", nil)

	FromContext(c).Info().Msg("request")

	out := buf.String()
	require.NotEmpty(t, out)
	require.Equal(t, "\n", out[len(out)-1:], "output must end with exactly one newline")
	for i := 0; i < len(out)-1; i++ {
		b := out[i]
		require.Truef(t, b >= 0x20 && b != 0x7f,
			"control byte %#02x at position %d in log output %q", b, i, out)
	}
}

// captureStdout redirects the process-wide os.Stdout to a pipe for the
// duration of a test. InitLogger writes directly to os.Stdout (there is no
// injected writer), so this is the only way to observe its startup line. The
// returned closeWrite must be called once the test is done emitting logs so
// the reader sees EOF.
func captureStdout(t *testing.T) (r *os.File, closeWrite func()) {
	t.Helper()
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdout
	os.Stdout = pw
	var once sync.Once
	t.Cleanup(func() {
		os.Stdout = old
		once.Do(func() { _ = pw.Close() })
		_ = pr.Close()
	})
	return pr, func() {
		once.Do(func() { _ = pw.Close() })
	}
}

func saveLoggerGlobals(t *testing.T) {
	t.Helper()
	oldLogger := Logger
	oldLevel := zerolog.GlobalLevel()
	oldTimeFormat := zerolog.TimeFieldFormat
	oldDefaultCtx := zerolog.DefaultContextLogger
	t.Cleanup(func() {
		Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
		zerolog.TimeFieldFormat = oldTimeFormat
		zerolog.DefaultContextLogger = oldDefaultCtx
	})
}

// TestInitLoggerJSONWritesStartupLine covers the non-pretty branch of
// InitLogger: plain JSON to stdout, default (RFC3339) time format, and the
// documented "Logger initialized" line carrying the config that produced it.
func TestInitLoggerJSONWritesStartupLine(t *testing.T) {
	saveLoggerGlobals(t)
	r, closeWrite := captureStdout(t)

	InitLogger(Config{Level: "debug"})

	closeWrite()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(out), "Logger initialized")
	require.Contains(t, string(out), `"level":"debug"`)
	require.NotContains(t, string(out), "\x1b[", "JSON mode must not emit ANSI colors")
}

// TestInitLoggerPrettyAndCustomTimeFormat covers the pretty console branch and
// the explicit TimeFormat branch. Console output is human-oriented (ANSI
// colors, non-JSON), so the assertion is that the startup line is still
// emitted rather than the exact rendering.
func TestInitLoggerPrettyAndCustomTimeFormat(t *testing.T) {
	saveLoggerGlobals(t)
	r, closeWrite := captureStdout(t)

	InitLogger(Config{Level: "info", Pretty: true, TimeFormat: time.RFC3339Nano})

	closeWrite()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NotEmpty(t, out, "pretty console logger must still emit the startup line")
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want zerolog.Level
	}{
		{in: "debug", want: zerolog.DebugLevel},
		{in: "info", want: zerolog.InfoLevel},
		{in: "warn", want: zerolog.WarnLevel},
		{in: "error", want: zerolog.ErrorLevel},
		{in: "fatal", want: zerolog.FatalLevel},
		{in: "panic", want: zerolog.PanicLevel},
		{in: "trace", want: zerolog.InfoLevel}, // unrecognized falls back to info
		{in: "", want: zerolog.InfoLevel},      // empty is unrecognized
		{in: "INFO", want: zerolog.InfoLevel},  // case-sensitive; uppercase is unrecognized
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, parseLevel(tc.in))
		})
	}
}

// TestLevelWrappers exercises the package-level Info/Debug/Warn/Error/Fatal/
// Panic helpers. They only build a *zerolog.Event; the fatal/panic ones only
// terminate the process when .Msg is called, so constructing them is safe.
func TestLevelWrappers(t *testing.T) {
	buf := captureLogger(t)

	// Note: no .Msg() on the Fatal/Panic events — that is what actually
	// calls os.Exit/panic.
	require.NotNil(t, Info())
	require.NotNil(t, Debug())
	require.NotNil(t, Warn())
	require.NotNil(t, Error())
	require.NotNil(t, Fatal())
	require.NotNil(t, Panic())

	// Emit one real line to prove the global logger is wired to the buffer.
	Info().Str("k", "v").Msg("hello")
	line := lastLine(t, buf)
	require.Equal(t, "hello", line["message"])
}
