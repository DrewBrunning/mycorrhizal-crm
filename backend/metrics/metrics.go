package metrics

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"mycorrhizal/buildinfo"
)

// processStart is fixed at first import so process_uptime_seconds is meaningful
// from the very first scrape.
var processStart = time.Now()

// ProcessStart returns the instant this process began, fixed at first import
// of this package. Exported so the admin system-status endpoint (issue #388)
// can report uptime from the same source the /metrics gauges use, without a
// second time origin that could disagree.
func ProcessStart() time.Time { return processStart }

var defaultRegistry = NewRegistry()

// Default is the process-wide registry the /metrics endpoint renders.
func Default() *Registry { return defaultRegistry }

// Metric families. Names follow Prometheus conventions: base unit in the name,
// `_total` only on counters, low-cardinality labels only (route templates and
// bounded enum tokens — never contact IDs or raw paths).
var (
	httpRequests = defaultRegistry.NewCounterVec(
		"http_requests_total",
		"Total HTTP requests by method, matched route template and response status code.",
		"method", "route", "status")

	httpDuration = defaultRegistry.NewHistogramVec(
		"http_request_duration_seconds",
		"HTTP request latency in seconds by method and matched route template.",
		[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		"method", "route")

	httpInFlight = defaultRegistry.NewGaugeVec(
		"http_requests_in_flight",
		"HTTP requests currently being served.")

	jobRuns = defaultRegistry.NewCounterVec(
		"job_runs_total",
		"Background job executions by job name and result.",
		"job", "result")

	jobDuration = defaultRegistry.NewHistogramVec(
		"job_duration_seconds",
		"Background job wall-clock duration in seconds by job name.",
		[]float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 900},
		"job")

	systemEvents = defaultRegistry.NewCounterVec(
		"system_events_total",
		"Operational events recorded to the system_events timeline, by event type, component and result. "+
			"Covers sync, notification, backup/restore, webhook, migration and job outcomes.",
		"event_type", "component", "result")

	goGoroutines  = defaultRegistry.NewGaugeVec("go_goroutines", "Number of goroutines that currently exist.")
	goAllocBytes  = defaultRegistry.NewGaugeVec("go_memstats_alloc_bytes", "Bytes of allocated heap objects.")
	goHeapInuse   = defaultRegistry.NewGaugeVec("go_memstats_heap_inuse_bytes", "Bytes in in-use heap spans.")
	goSysBytes    = defaultRegistry.NewGaugeVec("go_memstats_sys_bytes", "Bytes of memory obtained from the OS.")
	goInfo        = defaultRegistry.NewGaugeVec("go_info", "Information about the Go runtime; value is always 1.", "version")
	processUptime = defaultRegistry.NewGaugeVec("process_uptime_seconds", "Seconds elapsed since the process started.")
	processStartT = defaultRegistry.NewGaugeVec("process_start_time_seconds", "Start time of the process since the Unix epoch, in seconds.")
	buildInfo     = defaultRegistry.NewGaugeVec("mycorrhizal_build_info", "Build identity of the running binary; value is always 1.", "version", "commit")

	dbOpen    = defaultRegistry.NewGaugeVec("db_connections_open", "Established database connections (in use plus idle).")
	dbInUse   = defaultRegistry.NewGaugeVec("db_connections_in_use", "Database connections currently in use.")
	dbIdle    = defaultRegistry.NewGaugeVec("db_connections_idle", "Idle database connections in the pool.")
	dbMaxOpen = defaultRegistry.NewGaugeVec("db_connections_max_open", "Configured max open connections (0 means unlimited).")
	dbWaitN   = defaultRegistry.NewGaugeVec("db_connections_wait_count", "Connection waits since start (cumulative; a rising value indicates pool pressure).")
	dbWaitSec = defaultRegistry.NewGaugeVec("db_connections_wait_seconds", "Time blocked waiting for a connection since start, in seconds (cumulative).")

	storageBytes = defaultRegistry.NewGaugeVec("mycorrhizal_storage_bytes", "On-disk size of a storage area in bytes.", "kind")
	fsFreeBytes  = defaultRegistry.NewGaugeVec("filesystem_free_bytes", "Free bytes on the filesystem holding the database.")
	fsSizeBytes  = defaultRegistry.NewGaugeVec("filesystem_size_bytes", "Total bytes on the filesystem holding the database.")
)

// HTTPRequest counts one completed HTTP request.
func HTTPRequest(method, route, status string) {
	httpRequests.With(method, route, status).Inc()
}

// HTTPObserve records one HTTP request's latency.
func HTTPObserve(method, route string, seconds float64) {
	httpDuration.With(method, route).Observe(seconds)
}

// HTTPInFlightInc / HTTPInFlightDec bracket a request currently being served.
func HTTPInFlightInc() { httpInFlight.With().Inc() }
func HTTPInFlightDec() { httpInFlight.With().Dec() }

// JobRun records one background-job execution and its duration.
func JobRun(job, result string, seconds float64) {
	jobRuns.With(job, foldResult(result)).Inc()
	jobDuration.With(job).Observe(seconds)
}

// SystemEvent counts one operational event as it is recorded to the
// system_events timeline. event_type / component are folded to a bounded set
// so a stray free-form component string cannot blow up label cardinality.
func SystemEvent(eventType, component, result string) {
	systemEvents.With(foldEventType(eventType), foldComponent(component), foldResult(result)).Inc()
}

// SetRuntimeGauges refreshes the Go-runtime and process gauges. Called once
// per scrape from the /metrics handler.
func SetRuntimeGauges() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	goGoroutines.With().Set(float64(runtime.NumGoroutine()))
	goAllocBytes.With().Set(float64(ms.Alloc))
	goHeapInuse.With().Set(float64(ms.HeapInuse))
	goSysBytes.With().Set(float64(ms.Sys))
	goInfo.With(runtime.Version()).Set(1)

	processUptime.With().Set(time.Since(processStart).Seconds())
	processStartT.With().Set(float64(processStart.Unix()))

	bi := buildinfo.Get()
	buildInfo.With(bi.Version, bi.Commit).Set(1)
}

// SetDBGauges refreshes the connection-pool gauges from a sql.DBStats snapshot.
func SetDBGauges(s sql.DBStats) {
	dbOpen.With().Set(float64(s.OpenConnections))
	dbInUse.With().Set(float64(s.InUse))
	dbIdle.With().Set(float64(s.Idle))
	dbMaxOpen.With().Set(float64(s.MaxOpenConnections))
	dbWaitN.With().Set(float64(s.WaitCount))
	dbWaitSec.With().Set(s.WaitDuration.Seconds())
}

// SetStorageGauges refreshes the storage gauges: the SQLite database footprint
// (main file plus its -wal / -shm siblings) and the free/total bytes of the
// filesystem holding it. Per-directory attachment and profile-photo totals are
// deliberately not walked here — a recursive stat on every scrape is the line
// this endpoint does not cross for a self-hosted single-process app.
func SetStorageGauges(dbPath string) {
	storageBytes.With("database").Set(float64(DatabaseBytes(dbPath)))

	if free, size, ok := FilesystemBytes(filepath.Dir(dbPath)); ok {
		fsFreeBytes.With().Set(free)
		fsSizeBytes.With().Set(size)
	}
}

// DatabaseBytes is the on-disk footprint of the SQLite database at dbPath: the
// main file plus its -wal / -shm siblings, each counted only when it exists.
// Shared by the /metrics storage gauge and the admin system-status endpoint
// (issue #388) so the two never disagree on how the total is computed.
func DatabaseBytes(dbPath string) int64 {
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, err := os.Stat(dbPath + suffix); err == nil {
			total += fi.Size()
		}
	}
	return total
}

// FilesystemBytes returns (free, total) bytes for the filesystem holding path.
// Same syscall.Statfs primitive as services/alerting_conditions.go; arithmetic
// in float64 to keep gosec G115 satisfied without a suppression. Exported for
// the admin system-status endpoint (issue #388).
func FilesystemBytes(path string) (free, total float64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil || st.Bsize <= 0 || st.Blocks == 0 {
		return 0, 0, false
	}
	bsize := float64(st.Bsize)
	total = float64(st.Blocks) * bsize
	free = float64(st.Bavail) * bsize
	if free > total {
		free = total
	}
	return free, total, true
}

// --- label folding ---------------------------------------------------------

func foldResult(result string) string {
	switch result {
	case "":
		return "none"
	case "success", "failure", "skipped":
		return result
	default:
		return "other"
	}
}

// knownComponents mirrors logger.Component* (backend/logger/fields.go). Kept as
// a local copy on purpose (this package is a dependency-free leaf and must not
// import logger); an unlisted value folds to "other", so a missed addition
// degrades gracefully rather than breaking. Keep in sync.
var knownComponents = map[string]struct{}{
	"app": {}, "scheduler": {}, "migration": {}, "contact_sync": {},
	"calendar_sync": {}, "notification": {}, "webhook": {}, "backup": {},
}

func foldComponent(component string) string {
	if component == "" {
		return "none"
	}
	if _, ok := knownComponents[component]; ok {
		return component
	}
	return "other"
}

// knownEventTypes mirrors models.SystemEventTypes (backend/models/system_event.go)
// and migration 000038's CHECK constraint. Local copy for the same
// leaf-package reason as knownComponents. Keep in sync.
var knownEventTypes = map[string]struct{}{
	"application_started": {}, "application_stopped": {},
	"migration_started": {}, "migration_completed": {}, "migration_failed": {},
	"job_started": {}, "job_completed": {}, "job_failed": {},
	"sync_started": {}, "sync_completed": {}, "sync_failed": {},
	"notification_sent": {}, "notification_failed": {},
	"backup_completed": {}, "backup_failed": {}, "restore_test_completed": {},
	"integration_failed": {},
}

func foldEventType(eventType string) string {
	if eventType == "" {
		return "none"
	}
	if _, ok := knownEventTypes[eventType]; ok {
		return eventType
	}
	return "other"
}
