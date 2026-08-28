package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"mycorrhizal/logger"
)

// captureLogger swaps the global logger for one writing into buf and forces a
// permissive global level (other packages may have raised it), restoring both
// afterwards.
func captureLogger(t *testing.T, buf *bytes.Buffer, console bool) {
	t.Helper()
	oldLogger := logger.Logger
	oldLevel := zerolog.GlobalLevel()
	if console {
		logger.Logger = zerolog.New(zerolog.ConsoleWriter{Out: buf, NoColor: true})
	} else {
		logger.Logger = zerolog.New(buf)
	}
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		logger.Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})
}

func TestLoggingMiddlewareSanitizesControlCharacters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	craftedTargets := []string{
		"/%0aFAKE",                       // CRLF-style line break in the path
		"/%0dFAKE",                       // bare carriage return in the path
		"/path?q=%0aquery%0dCR",          // control characters in the query
		"/%1b[31mANSI%1b[0m?q=%0aforged", // ANSI escape + newline
	}

	for _, console := range []bool{false, true} {
		mode := "json"
		if console {
			mode = "console"
		}
		t.Run(mode, func(t *testing.T) {
			for _, target := range craftedTargets {
				t.Run(target, func(t *testing.T) {
					buf := &bytes.Buffer{}
					captureLogger(t, buf, console)

					router := gin.New()
					router.Use(LoggingMiddleware())
					router.GET("/*any", func(c *gin.Context) { c.Status(http.StatusOK) })

					req := httptest.NewRequest(http.MethodGet, target, nil)
					req.Header.Set("User-Agent", "evil\x0aAgent\x0d")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					require.Equal(t, http.StatusOK, w.Code)
					requireControlFree(t, buf.String())
				})
			}
		})
	}
}

func TestLoggingMiddlewareSanitizesErrorField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buf := &bytes.Buffer{}
	captureLogger(t, buf, true) // console mode is the strict case: messages/errors print verbatim

	router := gin.New()
	router.Use(LoggingMiddleware())
	router.GET("/x", func(c *gin.Context) {
		// Simulate an error that echoes user-controlled input back into the
		// request error log (e.g. a validation failure on a crafted value).
		c.Error(fmt.Errorf("invalid value: %q", "forged\x0aline\x0d"))
		c.Status(http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requireControlFree(t, buf.String())
}

func TestLoggingMiddlewareSingleLinePerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, console := range []bool{false, true} {
		mode := "json"
		if console {
			mode = "console"
		}
		t.Run(mode, func(t *testing.T) {
			buf := &bytes.Buffer{}
			captureLogger(t, buf, console)

			router := gin.New()
			router.Use(LoggingMiddleware())
			router.GET("/*any", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/%0aFAKE", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, 1, strings.Count(buf.String(), "\n"),
				"crafted path must yield exactly one log line, not two")
			requireControlFree(t, buf.String())
		})
	}
}

func TestLoggingMiddlewareRedactsSensitiveQueryValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, console := range []bool{false, true} {
		mode := "json"
		if console {
			mode = "console"
		}
		t.Run(mode, func(t *testing.T) {
			buf := &bytes.Buffer{}
			captureLogger(t, buf, console)

			router := gin.New()
			router.Use(LoggingMiddleware())
			router.GET("/*any", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/?code=TOP-SECRET&state=CSRF-NONCE&search=Ada+Lovelace&page=2", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			out := buf.String()
			require.NotContains(t, out, "TOP-SECRET", "OIDC authorization code must not reach the log")
			require.NotContains(t, out, "CSRF-NONCE", "OIDC state (CSRF token) must not reach the log")
			require.NotContains(t, out, "Ada", "a free-text search term is personal data and must not reach the log")
			require.Contains(t, out, "code=[REDACTED]", "sensitive value must be redacted")
			require.Contains(t, out, "state=[REDACTED]")
			require.Contains(t, out, "search=[REDACTED]")
			require.Contains(t, out, "page=2", "allow-listed operational params are preserved")
		})
	}
}

// requireControlFree asserts the log output contains no control characters
// other than the single trailing newline that terminates the log record.
func requireControlFree(t *testing.T, out string) {
	t.Helper()
	require.NotEmpty(t, out)
	require.Equal(t, "\n", out[len(out)-1:], "output must end with exactly one newline")
	for i := 0; i < len(out)-1; i++ {
		c := out[i]
		require.Truef(t, c >= 0x20 && c != 0x7f,
			"control character %#02x at byte %d in log output %q", c, i, out)
	}
}
