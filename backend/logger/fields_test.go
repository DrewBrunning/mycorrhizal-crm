package logger

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOp_Done_Success(t *testing.T) {
	buf := captureLogger(t)

	op := Op(context.Background(), "job_completed").
		Component(ComponentScheduler).
		Operation("purge_deleted").
		Str("version", "v0.6.3").
		Int("rows", 4)
	op.Done(nil)

	line := lastLine(t, buf)
	require.Equal(t, "job_completed", line[FieldEvent])
	require.Equal(t, ResultSuccess, line[FieldResult])
	require.Equal(t, ComponentScheduler, line[FieldComponent])
	require.Equal(t, "purge_deleted", line[FieldOperation])
	require.Equal(t, "v0.6.3", line["version"])
	require.Equal(t, float64(4), line["rows"])
	require.Contains(t, line, FieldDurationMS)
	require.Equal(t, "info", line["level"])
	require.NotContains(t, line, FieldError)
}

func TestOp_Done_Failure(t *testing.T) {
	buf := captureLogger(t)

	Op(context.Background(), "sync_failed").
		Component(ComponentContactSync).
		Done(errors.New("boom\nwith newline"))

	line := lastLine(t, buf)
	require.Equal(t, "sync_failed", line[FieldEvent])
	require.Equal(t, ResultFailure, line[FieldResult])
	require.Equal(t, "error", line["level"])
	// Error string is sanitized (no raw control chars).
	require.Equal(t, `boom\nwith newline`, line[FieldError])
}

func TestOp_Skip(t *testing.T) {
	buf := captureLogger(t)

	Op(context.Background(), "job_completed").
		Component(ComponentScheduler).
		Operation("immich_sync").
		Skip("job lock held")

	line := lastLine(t, buf)
	require.Equal(t, ResultSkipped, line[FieldResult])
	require.Equal(t, "job lock held", line[FieldReason])
	require.Contains(t, line, FieldDurationMS)
}

func TestOp_CarriesCorrelationIDFromContext(t *testing.T) {
	buf := captureLogger(t)

	ctx := WithCorrelationID(context.Background(), "corr-9")
	Op(ctx, "job_completed").Done(nil)

	require.Equal(t, "corr-9", lastLine(t, buf)[FieldCorrelationID])
}
