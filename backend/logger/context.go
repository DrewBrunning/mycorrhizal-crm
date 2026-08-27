package logger

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Correlation-ID propagation (issue #425).
//
// A single unit of user-visible work — an HTTP request, or one scheduled job
// run — gets one correlation ID. Every background step and outbound call it
// spawns carries the same ID, so an operator can follow a UI action through
// to its eventual result or error across otherwise-disconnected log threads.
//
// The ID lives in two places on the context:
//   - bound as the FieldCorrelationID field on a zerolog.Logger stored via
//     Logger.WithContext, so Ctx(ctx) log lines carry it with no extra work;
//   - as a bare string under correlationIDKey, so CorrelationID(ctx) can read
//     it back for an outbound X-Correlation-ID header without parsing a logger.

type ctxKey int

const correlationIDKey ctxKey = iota

// Ctx returns the logger bound to ctx (via WithContext / WithCorrelationID /
// WithComponent), falling back to the global Logger when ctx carries none.
// InitLogger points zerolog.DefaultContextLogger at the global Logger, so
// zerolog.Ctx already falls back correctly; this wrapper adds nil-ctx safety
// and a stable name for call sites.
func Ctx(ctx context.Context) *zerolog.Logger {
	if ctx == nil {
		return &Logger
	}
	l := zerolog.Ctx(ctx)
	if l == nil || l.GetLevel() == zerolog.Disabled {
		return &Logger
	}
	return l
}

// WithCorrelationID binds id as the correlation ID on ctx: both as a bound
// logger field and as a retrievable string. An empty id is a no-op.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if id == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, correlationIDKey, id)
	l := Ctx(ctx).With().Str(FieldCorrelationID, id).Logger()
	return l.WithContext(ctx)
}

// WithComponent binds component as the FieldComponent field on ctx's logger.
func WithComponent(ctx context.Context, component string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	l := Ctx(ctx).With().Str(FieldComponent, component).Logger()
	return l.WithContext(ctx)
}

// CorrelationID returns the correlation ID bound to ctx, or "" if none.
func CorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(correlationIDKey).(string); ok {
		return v
	}
	return ""
}

// JobContext builds the context for one scheduled-job run: a fresh
// correlation ID of the form "job:<jobName>:<uuid>" and the scheduler
// component, both bound for logging. Used by main.go's recoverJob / safeGo
// wrappers so every line a job emits — and every outbound call it makes —
// shares one ID distinct from any HTTP request.
func JobContext(jobName string) context.Context {
	id := "job:" + jobName + ":" + uuid.NewString()
	return WithComponent(WithCorrelationID(context.Background(), id), ComponentScheduler)
}
