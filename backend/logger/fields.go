package logger

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Standardized structured-logging vocabulary (issue #425).
//
// Operational producers — scheduler jobs, sync services, notification
// dispatch, webhook delivery, the migration runner, backup/restore — pick
// their field keys from this list instead of inventing one per call site, so
// a log stream can be filtered and correlated the same way everywhere. This
// is a consistency convention, not a hard requirement on every log line in
// the codebase: request-path logging already has its own shape in
// middleware/logging.go, and low-value debug lines are left alone.
//
// The matching persisted shape is models.SystemEvent; keep the value
// vocabularies below (results, severities) in sync with its CHECK constraint.
const (
	FieldEvent         = "event"          // what happened, e.g. "job_completed"
	FieldComponent     = "component"      // subsystem, e.g. "contact_sync"
	FieldOperation     = "operation"      // the specific unit of work, e.g. a job name
	FieldDurationMS    = "duration_ms"    // wall-clock duration of the operation
	FieldResult        = "result"         // success | failure | skipped
	FieldError         = "error"          // sanitized error string on the failure path
	FieldCorrelationID = "correlation_id" // shared ID threading one chain of work
	FieldRequestID     = "request_id"     // the originating HTTP request's ID
	FieldUserID        = "user_id"        // the acting user, when attributable
	FieldReason        = "reason"         // why an operation was skipped
)

// Result values for FieldResult. A "skipped" run is neither success nor
// failure — the work did not run (job lock held, feature disabled, nothing
// to do).
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultSkipped = "skipped"
)

// Severity values, mirroring zerolog levels for the persisted SystemEvent
// where a raw zerolog.Level is not stored.
const (
	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityError = "error"
)

// Component values for the operational producers. Free-form strings are still
// allowed, but a producer that has a name here should use it.
const (
	ComponentApp          = "app"
	ComponentScheduler    = "scheduler"
	ComponentMigration    = "migration"
	ComponentContactSync  = "contact_sync"
	ComponentCalendarSync = "calendar_sync"
	ComponentNotify       = "notification"
	ComponentWebhook      = "webhook"
	ComponentBackup       = "backup"
	ComponentStorage      = "storage"
)

// OpLog times a single operation and emits one standardized completion line
// when Done or Skip is called. It is the helper behind the "important
// long-running operations have measurable duration" requirement (#425):
//
//	op := logger.Op(ctx, "job_completed").Component(logger.ComponentScheduler).Operation(name)
//	defer func() { op.Done(err) }()
//
// The logger is resolved from ctx (Ctx), so a correlation ID bound upstream
// rides along automatically.
type OpLog struct {
	logger    *zerolog.Logger
	event     string
	component string
	operation string
	start     time.Time
	strFields map[string]string
	intFields map[string]int64
}

// Op starts timing an operation. event is the value for FieldEvent on the
// completion line (e.g. "sync_completed", "job_completed").
func Op(ctx context.Context, event string) *OpLog {
	return &OpLog{
		logger: Ctx(ctx),
		event:  event,
		start:  time.Now(),
	}
}

// Component sets FieldComponent on the completion line.
func (o *OpLog) Component(c string) *OpLog { o.component = c; return o }

// Operation sets FieldOperation on the completion line.
func (o *OpLog) Operation(op string) *OpLog { o.operation = op; return o }

// Str attaches an extra low-cardinality string field to the completion line.
func (o *OpLog) Str(key, value string) *OpLog {
	if o.strFields == nil {
		o.strFields = map[string]string{}
	}
	o.strFields[key] = value
	return o
}

// Int attaches an extra numeric field to the completion line (counts, sizes).
func (o *OpLog) Int(key string, value int) *OpLog {
	if o.intFields == nil {
		o.intFields = map[string]int64{}
	}
	o.intFields[key] = int64(value)
	return o
}

// Duration returns the elapsed time since Op was called.
func (o *OpLog) Duration() time.Duration { return time.Since(o.start) }

// Done emits the completion line. A nil err is result=success at info level;
// a non-nil err is result=failure at error level with a sanitized error
// string.
func (o *OpLog) Done(err error) {
	var ev *zerolog.Event
	if err != nil {
		ev = o.logger.Error().Str(FieldResult, ResultFailure).Str(FieldError, SanitizeLogField(err.Error()))
	} else {
		ev = o.logger.Info().Str(FieldResult, ResultSuccess)
	}
	o.emit(ev)
}

// Skip emits the completion line for an operation that did not run.
func (o *OpLog) Skip(reason string) {
	o.emit(o.logger.Info().Str(FieldResult, ResultSkipped).Str(FieldReason, SanitizeLogField(reason)))
}

func (o *OpLog) emit(ev *zerolog.Event) {
	ev = ev.Str(FieldEvent, o.event).Int64(FieldDurationMS, o.Duration().Milliseconds())
	if o.component != "" {
		ev = ev.Str(FieldComponent, o.component)
	}
	if o.operation != "" {
		ev = ev.Str(FieldOperation, o.operation)
	}
	for k, v := range o.strFields {
		ev = ev.Str(k, v)
	}
	for k, v := range o.intFields {
		ev = ev.Int64(k, v)
	}
	ev.Msg(o.event)
}
