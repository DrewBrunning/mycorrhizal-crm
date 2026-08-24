package logger

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

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
