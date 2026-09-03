package perfbench

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ResourceSample is one data-movement operation measured once (PERF-03, issue
// #470). Where PERF-02 records the deterministic query count of a fast
// interactive call, PERF-03 records the *resource envelope* of a bulk
// operation the user is not waiting on: how long it ran, how much memory it
// held at its peak, how much extra disk it needed, how many rows it touched,
// and — the outage-vs-background-task distinction — whether it blocked other
// writers while it ran.
type ResourceSample struct {
	// DurationNanos is wall-clock time for the operation. Indicative only:
	// hardware-variable, never a gate (issue #261's stance, inherited from
	// PERF-02).
	DurationNanos int64 `json:"duration_nanos"`

	// PeakHeapBytes is the highest runtime.MemStats.HeapAlloc observed while
	// the operation ran, minus the reading taken (after a GC) immediately
	// before it started. It is the number the "does peak memory scale with
	// the dataset?" finding is built from — an operation whose PeakHeapBytes
	// grows with the row count will eventually be OOM-killed on a small
	// self-hosted box.
	PeakHeapBytes int64 `json:"peak_heap_bytes"`
	// HeapGrowthBytes is HeapAlloc(after) - HeapAlloc(before), sampled at the
	// same two points. It can be negative (a GC mid-operation), so it is
	// context, not the signal; PeakHeapBytes is the signal.
	HeapGrowthBytes int64 `json:"heap_growth_bytes"`

	// PeakExtraDiskBytes is the largest total size of the watched paths
	// (the SQLite file, its -wal/-shm sidecars, and any output/temp files)
	// observed during the operation, minus their total when it started. This
	// is the free space an operator must have before running it — VACUUM INTO
	// needs room for a whole second copy, a table rebuild during migration
	// needs room for a copy of that table.
	PeakExtraDiskBytes int64 `json:"peak_extra_disk_bytes"`
	// FinalExtraDiskBytes is the same delta measured once the operation
	// returned — the persistent cost (a backup file that stays) as opposed to
	// the transient peak (a -wal that is checkpointed away).
	FinalExtraDiskBytes int64 `json:"final_extra_disk_bytes"`

	// RowsTouched is what the operation itself reports it processed
	// (contacts exported, FTS rows rewritten, dependent rows deleted). 0 when
	// the operation has no natural row count.
	RowsTouched int `json:"rows_touched"`

	// OutputBytes is the size of the artifact the operation produced — the
	// serialized export stream, the VACUUM INTO backup file — when it has one.
	// 0 otherwise. It is the other half of the free-space story: a backup
	// needs room for OutputBytes, an export streamed to a client needs none
	// on the server but the client does.
	OutputBytes int64 `json:"output_bytes"`

	// HeapSamples / DiskSamples are how many times each sampler fired — a
	// sanity check that a fast operation was still observed at least once.
	HeapSamples int `json:"heap_samples"`
	DiskSamples int `json:"disk_samples"`

	// WriteProbed is true when a concurrent write probe ran alongside the
	// operation. When false the three fields below are zero and mean nothing.
	WriteProbed bool `json:"write_probed"`
	// ProbeWrites is how many one-row writes the probe completed on a second
	// connection while the operation ran.
	ProbeWrites int `json:"probe_writes"`
	// ProbeMaxStallNanos is the longest a single probe write took. A value
	// near the 5s busy_timeout (with ProbeStalledOut set) means the operation
	// held SQLite's single write lock for its whole duration — an outage for
	// any concurrent writer, not a background task.
	ProbeMaxStallNanos int64 `json:"probe_max_stall_nanos"`
	// ProbeStalledOut is set when at least one probe write hit SQLITE_BUSY
	// after the full busy_timeout — i.e. the operation blocked writes for
	// longer than 5 seconds.
	ProbeStalledOut bool `json:"probe_stalled_out"`
}

// heapSampleInterval / diskSampleInterval pace the two background samplers.
// runtime.ReadMemStats briefly stops the world, so the heap poll is kept at
// 5ms — frequent enough to catch the peak of any operation here (they all run
// for tens of ms or more) without its own overhead dominating the wall-clock
// (which is indicative-only anyway). The filesystem walk behind the disk poll
// is heavier, so it runs less often.
const (
	heapSampleInterval = 5 * time.Millisecond
	diskSampleInterval = 20 * time.Millisecond
	probeInterval      = 5 * time.Millisecond
)

// sampleResources runs fn once with a memory sampler, a disk sampler, and
// (when probe is non-nil) a concurrent write probe attached, and returns the
// resource envelope. watchPaths are the files/directories whose combined size
// is tracked for the disk delta — pass the SQLite file and the directory any
// output lands in.
func sampleResources(watchPaths []string, probe *writeProbe, fn func() (rowsTouched int, outputBytes int64, err error)) (ResourceSample, error) {
	// A GC before the baseline reading means PeakHeapBytes is measured against
	// a compacted floor, not whatever garbage the populate left behind.
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	baseHeap := ms.HeapAlloc
	baseDisk := totalSize(watchPaths)

	var (
		mu        sync.Mutex
		peakHeap  = baseHeap
		heapCount int
		peakDisk  = baseDisk
		diskCount int
	)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(heapSampleInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				mu.Lock()
				if m.HeapAlloc > peakHeap {
					peakHeap = m.HeapAlloc
				}
				heapCount++
				mu.Unlock()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(diskSampleInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s := totalSize(watchPaths)
				mu.Lock()
				if s > peakDisk {
					peakDisk = s
				}
				diskCount++
				mu.Unlock()
			}
		}
	}()

	if probe != nil {
		probe.start()
	}

	start := time.Now()
	rows, outputBytes, runErr := fn()
	elapsed := time.Since(start)

	close(stop)
	wg.Wait()
	if probe != nil {
		probe.stop()
	}

	runtime.ReadMemStats(&ms)
	endHeap := ms.HeapAlloc
	endDisk := totalSize(watchPaths)

	mu.Lock()
	if endHeap > peakHeap {
		peakHeap = endHeap
	}
	if endDisk > peakDisk {
		peakDisk = endDisk
	}
	sample := ResourceSample{
		DurationNanos:       elapsed.Nanoseconds(),
		PeakHeapBytes:       int64(peakHeap) - int64(baseHeap),
		HeapGrowthBytes:     int64(endHeap) - int64(baseHeap),
		PeakExtraDiskBytes:  peakDisk - baseDisk,
		FinalExtraDiskBytes: endDisk - baseDisk,
		RowsTouched:         rows,
		OutputBytes:         outputBytes,
		// + 1 for the mandatory post-run reading above, so a sub-tick-interval
		// operation still reports at least one sample of each.
		HeapSamples: heapCount + 1,
		DiskSamples: diskCount + 1,
	}
	mu.Unlock()

	if sample.PeakHeapBytes < 0 { // # pragma: no cover — a GC can briefly drop HeapAlloc below the baseline
		sample.PeakHeapBytes = 0
	}

	if probe != nil {
		sample.WriteProbed = true
		sample.ProbeWrites = probe.writes
		sample.ProbeMaxStallNanos = probe.maxStall.Nanoseconds()
		sample.ProbeStalledOut = probe.stalledOut
	}

	return sample, runErr
}

// totalSize sums the byte size of every regular file at or under each path.
// Missing files (a -wal that has been checkpointed away between ticks, a temp
// file already renamed) contribute zero rather than an error — the sampler
// must never fail the measurement it is only observing.
func totalSize(paths []string) int64 {
	var total int64
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			total += info.Size()
			// SQLite keeps committed-but-uncheckpointed pages in a -wal
			// sidecar and an index in -shm; both are real disk the operator
			// needs and neither is under a directory walk of the .db path.
			for _, suffix := range []string{"-wal", "-shm"} {
				if si, err := os.Stat(p + suffix); err == nil {
					total += si.Size()
				}
			}
			continue
		}
		_ = filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a vanished entry is not a measurement failure
			}
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
			return nil
		})
	}
	return total
}

// writeProbe hammers a second connection to the operation's database with
// tiny one-row writes and records the longest any of them was made to wait.
// It is how PERF-03 tells an operation that holds the write lock for its whole
// duration (an outage for concurrent writers) from one that does not (a
// background task). It writes to its own scratch table so it never perturbs
// the fixture or fires a maintenance trigger.
type writeProbe struct {
	db *sql.DB

	stopCh chan struct{}
	doneCh chan struct{}

	writes     int
	maxStall   time.Duration
	stalledOut bool
}

// probeBusyTimeoutMS is the wait a probe write gives up after — the same 5s
// the app's own connections use (database.openDSN). A probe write that still
// fails after this long means the operation under test held the write lock
// for the whole 5s, which is the ProbeStalledOut signal.
const probeBusyTimeoutMS = 5000

// newWriteProbe opens a second raw connection to dbPath (the app's own DSN,
// so it inherits busy_timeout and _txlock=immediate) and creates the scratch
// table. busyTimeoutMS is the per-write give-up wait — production passes
// probeBusyTimeoutMS; a test passes something short so the stall-out branch
// is reachable without a real five-second lock hold. It returns nil, nil when
// dbPath is empty — an operation with no single database file to contend on
// (restore, which is building a new one) simply runs unprobed.
func newWriteProbe(dbPath string, busyTimeoutMS int) (*writeProbe, error) {
	if dbPath == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite", probeDSN(dbPath, busyTimeoutMS))
	if err != nil { // # pragma: no cover — sql.Open only validates the DSN string
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _perfbench_write_probe (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &writeProbe{db: db, stopCh: make(chan struct{}), doneCh: make(chan struct{})}, nil
}

func (p *writeProbe) start() {
	go func() {
		defer close(p.doneCh)
		t := time.NewTicker(probeInterval)
		defer t.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-t.C:
				began := time.Now()
				_, err := p.db.Exec("INSERT INTO _perfbench_write_probe (ts) VALUES (?)", began.UnixNano())
				stall := time.Since(began)
				if stall > p.maxStall {
					p.maxStall = stall
				}
				if err != nil {
					// SQLITE_BUSY after the full busy_timeout: the operation
					// under test has held the write lock for >5s.
					p.stalledOut = true
					continue
				}
				p.writes++
			}
		}
	}()
}

func (p *writeProbe) stop() {
	close(p.stopCh)
	<-p.doneCh
	if _, err := p.db.Exec("DROP TABLE IF EXISTS _perfbench_write_probe"); err != nil { // # pragma: no cover — DROP on a table this probe created
		_ = err
	}
	_ = p.db.Close()
}

// probeDSN mirrors database.openDSN closely enough for the probe: WAL, a
// busy_timeout, and BEGIN IMMEDIATE so a blocked write actually waits on the
// busy handler instead of failing instantly (CLAUDE.md backend trap #9).
// database.openDSN is unexported, so this is a deliberate small copy.
func probeDSN(path string, busyTimeoutMS int) string {
	return fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", path, busyTimeoutMS)
}
