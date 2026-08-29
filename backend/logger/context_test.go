package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// captureLogger swaps the global Logger for one writing JSON to buf, and
// restores it afterwards.
func captureLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := Logger
	oldDefault := zerolog.DefaultContextLogger
	oldLevel := zerolog.GlobalLevel()
	Logger = zerolog.New(buf)
	zerolog.DefaultContextLogger = &Logger
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		Logger = old
		zerolog.DefaultContextLogger = oldDefault
		zerolog.SetGlobalLevel(oldLevel)
	})
	return buf
}

func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &m))
	return m
}

func TestWithCorrelationID_BindsFieldAndString(t *testing.T) {
	buf := captureLogger(t)

	ctx := WithCorrelationID(context.Background(), "abc-123")

	require.Equal(t, "abc-123", CorrelationID(ctx))

	Ctx(ctx).Info().Msg("hi")
	require.Equal(t, "abc-123", lastLine(t, buf)[FieldCorrelationID])
}

func TestWithCorrelationID_EmptyIsNoop(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "")
	require.Equal(t, "", CorrelationID(ctx))
}

func TestCtx_FallsBackToGlobal(t *testing.T) {
	buf := captureLogger(t)

	// A bare context has no bound logger; Ctx must still log via the global.
	Ctx(context.Background()).Info().Msg("fallback")
	require.Equal(t, "fallback", lastLine(t, buf)["message"])

	// A nil context must not panic.
	require.NotNil(t, Ctx(nil))
}

func TestWithComponent_BindsField(t *testing.T) {
	buf := captureLogger(t)

	ctx := WithComponent(context.Background(), ComponentContactSync)
	Ctx(ctx).Info().Msg("x")

	require.Equal(t, ComponentContactSync, lastLine(t, buf)[FieldComponent])
}

func TestJobContext_HasCorrelationIDAndComponent(t *testing.T) {
	buf := captureLogger(t)

	ctx := JobContext("purge_deleted")

	id := CorrelationID(ctx)
	require.True(t, strings.HasPrefix(id, "job:purge_deleted:"), "got %q", id)

	Ctx(ctx).Info().Msg("run")
	line := lastLine(t, buf)
	require.Equal(t, id, line[FieldCorrelationID])
	require.Equal(t, ComponentScheduler, line[FieldComponent])
}

func TestChildContextInheritsCorrelationID(t *testing.T) {
	buf := captureLogger(t)

	ctx := WithCorrelationID(context.Background(), "chain-1")
	ctx = WithComponent(ctx, ComponentWebhook)

	Ctx(ctx).Info().Msg("step")
	line := lastLine(t, buf)
	require.Equal(t, "chain-1", line[FieldCorrelationID])
	require.Equal(t, ComponentWebhook, line[FieldComponent])
}

// The nil-context branches of the correlation/component helpers must be
// no-ops that never panic (callers in services/ and jobs/ pass contexts that
// can be nil on error paths).
func TestCorrelationHelpersNilContext(t *testing.T) {
	ctx := WithCorrelationID(nil, "abc")
	require.Equal(t, "abc", CorrelationID(ctx), "WithCorrelationID must lift a nil ctx to Background")

	ctx = WithComponent(nil, ComponentContactSync)
	require.NotNil(t, ctx)

	require.Equal(t, "", CorrelationID(nil))
}

func TestCorrelationID_NonStringValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), correlationIDKey, 42)
	require.Equal(t, "", CorrelationID(ctx), "a non-string correlation value must read as empty")
}
