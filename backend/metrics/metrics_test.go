package metrics

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dump(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	require.NoError(t, Default().WritePrometheus(&sb))
	return sb.String()
}

// delta returns a func reporting how much the exposition sample that starts
// with series (name plus label set) has grown since delta was called; an
// absent series reads as 0.
//
// Default() is process-global, so under -shuffle/-count=2 (issue #1492) an
// earlier test, or this test's own first run, has already incremented the same
// series. Tests assert the change across their own calls, never an absolute
// value.
func delta(t *testing.T, series string) func() float64 {
	t.Helper()
	read := func() float64 {
		for _, line := range strings.Split(dump(t), "\n") {
			if rest, ok := strings.CutPrefix(line, series+" "); ok {
				v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
				require.NoError(t, err, "unparseable sample %q", line)
				return v
			}
		}
		return 0
	}
	before := read()
	return func() float64 { return read() - before }
}

func TestJobRun_RecordsCountAndDuration(t *testing.T) {
	okRuns := delta(t, `job_runs_total{job="test-job-ok",result="success"}`)
	badRuns := delta(t, `job_runs_total{job="test-job-bad",result="failure"}`)
	okCount := delta(t, `job_duration_seconds_count{job="test-job-ok"}`)
	okSum := delta(t, `job_duration_seconds_sum{job="test-job-ok"}`)

	JobRun("test-job-ok", "success", 2.5)
	JobRun("test-job-ok", "success", 7.0)
	JobRun("test-job-bad", "failure", 0.2)

	assert.Equal(t, 2.0, okRuns())
	assert.Equal(t, 1.0, badRuns())
	assert.Equal(t, 2.0, okCount())
	assert.Equal(t, 9.5, okSum())
}

func TestHTTPHelpers_RecordOnExpectedFamilies(t *testing.T) {
	reqs := delta(t, `http_requests_total{method="GET",route="/api/v1/thing/:id",status="200"}`)
	durs := delta(t, `http_request_duration_seconds_count{method="GET",route="/api/v1/thing/:id"}`)
	inflight := delta(t, `http_requests_in_flight`)

	HTTPRequest("GET", "/api/v1/thing/:id", "200")
	HTTPObserve("GET", "/api/v1/thing/:id", 0.02)
	HTTPInFlightInc()
	HTTPInFlightInc()
	HTTPInFlightDec()
	// Put the process-global gauge back so no later test sees a leaked request.
	t.Cleanup(HTTPInFlightDec)

	assert.Equal(t, 1.0, reqs())
	assert.Equal(t, 1.0, durs())
	assert.Equal(t, 1.0, inflight())
}

func TestSystemEvent_FoldsUnknownLabelsToBoundedSet(t *testing.T) {
	sync := delta(t, `system_events_total{event_type="sync_failed",component="contact_sync",result="failure"}`)
	other := delta(t, `system_events_total{event_type="other",component="other",result="none"}`)
	sent := delta(t, `system_events_total{event_type="notification_sent",component="notification",result="success"}`)

	SystemEvent("sync_failed", "contact_sync", "failure")
	SystemEvent("totally-made-up", "some-plugin-name", "")
	SystemEvent("notification_sent", "notification", "success")

	assert.Equal(t, 1.0, sync())
	assert.Equal(t, 1.0, other())
	assert.Equal(t, 1.0, sent())
}

func TestFoldHelpers(t *testing.T) {
	assert.Equal(t, "none", foldResult(""))
	assert.Equal(t, "skipped", foldResult("skipped"))
	assert.Equal(t, "other", foldResult("weird"))

	assert.Equal(t, "none", foldComponent(""))
	assert.Equal(t, "backup", foldComponent("backup"))
	assert.Equal(t, "other", foldComponent("nope"))

	assert.Equal(t, "none", foldEventType(""))
	assert.Equal(t, "job_failed", foldEventType("job_failed"))
	assert.Equal(t, "other", foldEventType("nope"))
}

func TestSetRuntimeGauges_PopulatesFamilies(t *testing.T) {
	SetRuntimeGauges()

	out := dump(t)
	for _, name := range []string{
		"go_goroutines ", "go_memstats_alloc_bytes ", "go_memstats_sys_bytes ",
		"process_uptime_seconds ", "process_start_time_seconds ",
	} {
		assert.Contains(t, out, "\n"+name, "expected a %q sample", name)
	}
	assert.Contains(t, out, `go_info{version=`)
	assert.Contains(t, out, `mycorrhizal_build_info{version=`)
}

func TestSetDBGauges_FromStats(t *testing.T) {
	SetDBGauges(sql.DBStats{
		OpenConnections:    3,
		InUse:              1,
		Idle:               2,
		MaxOpenConnections: 10,
		WaitCount:          4,
		WaitDuration:       1500 * time.Millisecond,
	})

	out := dump(t)
	assert.Contains(t, out, "\ndb_connections_open 3\n")
	assert.Contains(t, out, "\ndb_connections_in_use 1\n")
	assert.Contains(t, out, "\ndb_connections_idle 2\n")
	assert.Contains(t, out, "\ndb_connections_max_open 10\n")
	assert.Contains(t, out, "\ndb_connections_wait_count 4\n")
	assert.Contains(t, out, "\ndb_connections_wait_seconds 1.5\n")
}

func TestSetStorageGauges_SumsSqliteSiblings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycorrhizal.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 1000), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", make([]byte, 200), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", make([]byte, 24), 0o600))

	SetStorageGauges(dbPath)

	out := dump(t)
	assert.Contains(t, out, `mycorrhizal_storage_bytes{kind="database"} 1224`+"\n")
	assert.Contains(t, out, "\nfilesystem_free_bytes ")
	assert.Contains(t, out, "\nfilesystem_size_bytes ")
}

func TestSetStorageGauges_MissingDBFileIsZero(t *testing.T) {
	SetStorageGauges(filepath.Join(t.TempDir(), "does-not-exist.db"))
	assert.Contains(t, dump(t), `mycorrhizal_storage_bytes{kind="database"} 0`+"\n")
}

// The three helpers below are exported so the admin system-status endpoint
// (issue #388) reads uptime and storage from the exact same source the
// /metrics gauges use. They were previously covered only transitively through
// SetStorageGauges / SetRuntimeGauges; these pin them directly.

func TestProcessStart_IsAStablePastInstant(t *testing.T) {
	a := ProcessStart()
	b := ProcessStart()
	assert.Equal(t, a, b, "ProcessStart must be fixed for the process lifetime")
	assert.False(t, a.IsZero(), "ProcessStart must be set")
	assert.False(t, a.After(time.Now()), "ProcessStart must be in the past")
}

func TestDatabaseBytes_SumsSqliteSiblings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycorrhizal.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 1000), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", make([]byte, 200), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", make([]byte, 24), 0o600))

	assert.Equal(t, int64(1224), DatabaseBytes(dbPath))
}

func TestDatabaseBytes_CountsOnlyExistingSiblings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycorrhizal.db")
	require.NoError(t, os.WriteFile(dbPath, make([]byte, 512), 0o600))

	assert.Equal(t, int64(512), DatabaseBytes(dbPath), "no -wal/-shm siblings present")
	assert.Equal(t, int64(0), DatabaseBytes(filepath.Join(dir, "absent.db")))
}

func TestFilesystemBytes_RealDirAndBogusPath(t *testing.T) {
	free, total, ok := FilesystemBytes(t.TempDir())
	require.True(t, ok)
	assert.Greater(t, total, 0.0)
	assert.GreaterOrEqual(t, free, 0.0)
	assert.LessOrEqual(t, free, total, "free is clamped to total")

	_, _, ok = FilesystemBytes(filepath.Join(t.TempDir(), "no", "such", "path"))
	assert.False(t, ok, "Statfs on a missing path reports not-ok")
}

func TestProcessRSSAndFDs_ParseProcFixture(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "statm"), []byte("100 25 3 1 0 5 0\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "fd"), 0o750))
	for _, n := range []string{"0", "1", "2"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "fd", n), nil, 0o600))
	}

	rss, ok := ProcessRSSBytes(root)
	require.True(t, ok)
	assert.Equal(t, 25*float64(os.Getpagesize()), rss)
	n, ok := ProcessOpenFDs(root)
	require.True(t, ok)
	assert.Equal(t, 3, n)

	// Missing, short and garbled inputs leave the gauge unset rather than zero.
	_, ok = ProcessRSSBytes(t.TempDir())
	assert.False(t, ok)
	_, ok = ProcessOpenFDs(t.TempDir())
	assert.False(t, ok)
	for _, bad := range []string{"7\n", "1 x\n", "1 -4\n"} {
		d := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(d, "statm"), []byte(bad), 0o600))
		_, ok = ProcessRSSBytes(d)
		assert.False(t, ok, bad)
	}
}

func TestSetLimiterGauges_AndWALStorageGauge(t *testing.T) {
	SetLimiterGauges(map[string]int{"api": 3, "account": 9})
	db := filepath.Join(t.TempDir(), "x.db")
	require.NoError(t, os.WriteFile(db, make([]byte, 10), 0o600))
	require.NoError(t, os.WriteFile(db+"-wal", make([]byte, 7), 0o600))
	SetStorageGauges(db)
	out := dump(t)
	assert.Contains(t, out, `mycorrhizal_ratelimiter_entries{limiter="api"} 3`)
	assert.Contains(t, out, `mycorrhizal_ratelimiter_entries{limiter="account"} 9`)
	assert.Contains(t, out, `mycorrhizal_storage_bytes{kind="wal"} 7`)

	require.NoError(t, os.Remove(db+"-wal"))
	SetStorageGauges(db)
	assert.Contains(t, dump(t), `mycorrhizal_storage_bytes{kind="wal"} 0`)
}
