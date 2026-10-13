package soak

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mycorrhizal/middleware"

	"gorm.io/gorm"
)

// Fault names a deliberate, harness-injected leak. It exists for one purpose:
// proving the soak's assertions bite. A soak that has never failed has proven
// nothing (CLAUDE.md "Hand-verify your tests"), and the real leaks it hunts
// are by definition not on hand. Each fault leaks one class of signal in the
// server's own process, once per sampling tick, through the real code where
// one exists (the real rate limiter, the real database connection pool).
// Faults are only available to an in-process run; they are never reachable
// from the server binary.
type Fault string

const (
	// FaultNone injects nothing (the default, and the only value CI uses
	// outside the harness's own self-test).
	FaultNone Fault = ""
	// FaultGoroutines leaks parked goroutines (an unbounded fire-and-forget).
	FaultGoroutines Fault = "goroutines"
	// FaultHeap retains heap (an unbounded map/slice keyed by request).
	FaultHeap Fault = "heap"
	// FaultFDs leaks open file descriptors (an unclosed file / response body).
	FaultFDs Fault = "fds"
	// FaultLimiter grows the real account rate limiter's maps with fresh keys
	// (state keyed by something unbounded that never evicts).
	FaultLimiter Fault = "limiter"
	// FaultWAL pins a read snapshot on the real connection pool for the whole
	// run, so SQLite can never checkpoint past it: the WAL grows without bound
	// (checkpoint starvation by a never-ending reader).
	FaultWAL Fault = "wal"
	// FaultDBConns pins pooled database connections and never returns them
	// (a leaked *sql.Rows / *sql.Tx holding its connection). The pool is
	// uncapped, so the open-connection count climbs.
	FaultDBConns Fault = "dbconns"
)

// Faults lists every injectable fault.
var Faults = []Fault{FaultGoroutines, FaultHeap, FaultFDs, FaultLimiter, FaultWAL, FaultDBConns}

// ParseFaults validates a comma-separated fault list from a flag ("" is none).
func ParseFaults(s string) ([]Fault, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []Fault
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, f := range Faults {
			if string(f) == name {
				out = append(out, f)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown fault %q (valid: goroutines, heap, fds, limiter, wal, dbconns)", name)
		}
	}
	return out, nil
}

// injector applies a set of Faults each tick and releases what they hold on
// Close.
type injector struct {
	faults []Fault

	stop    chan struct{}
	retain  [][]byte
	files   []*os.File
	pinConn *sql.Conn
	conns   []*sql.Conn // FaultDBConns: connections checked out and never returned
	ctx     context.Context
	db      *gorm.DB
	tick    int
	seq     int64 // distinguishes this injector's limiter keys from earlier runs'
	closeMu sync.Once

	// every is the nominal Tick cadence (the sampling interval); last is when
	// the previous Tick (or construction) happened. Tick scales what it leaks
	// by how many nominal intervals actually elapsed, so the injected leak is
	// proportional to wall-clock time, not to how many ticks a starved runner
	// managed to deliver (see Tick).
	every time.Duration
	last  time.Time
}

// Per-nominal-interval leak sizes. They are sized against the committed
// growth limits and the transient noise a contended -race runner puts on each
// signal (a start-up burst of ~100+ goroutines/fds and ~25 pooled
// connections that can sit inside the tail window), not against a healthy
// run's steady state: the injected ramp over the >= 9 s tail must clear both
// the limit and that transient by a wide margin, whatever the sample count.
const (
	leakGoroutines = 100
	leakHeapBytes  = 8 << 20
	leakFDs        = 48
	leakLimiter    = 40
	leakDBConns    = 8
)

// injectorSeq numbers injectors within a process. The account rate limiter is
// process-wide, so a second run in the same test binary (go test -count=2, the
// nightly order-dependence pass) that reused the first run's key names would
// only re-touch existing keys and the limiter leak would go undetected.
var injectorSeq atomic.Int64

// has reports whether f is in the injected set.
func (in *injector) has(f Fault) bool {
	for _, x := range in.faults {
		if x == f {
			return true
		}
	}
	return false
}

// newInjector prepares faults against db. WAL pinning needs the live pool;
// every other fault is self-contained.
func newInjector(ctx context.Context, faults []Fault, db *gorm.DB) (*injector, error) {
	in := &injector{faults: faults, stop: make(chan struct{}), db: db, seq: injectorSeq.Add(1), ctx: ctx, every: time.Second, last: time.Now()}
	if in.has(FaultDBConns) && db == nil {
		return nil, fmt.Errorf("fault %q needs an in-process database handle", FaultDBConns)
	}
	if in.has(FaultWAL) {
		if db == nil {
			return nil, fmt.Errorf("fault %q needs an in-process database handle", FaultWAL)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("fault wal: sql handle: %w", err)
		}
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("fault wal: pin connection: %w", err)
		}
		// A raw BEGIN is deferred (the immediate-txlock DSN flag only applies
		// to database/sql's own Begin), and the read after it takes the
		// snapshot every later checkpoint must respect.
		if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("fault wal: begin: %w", err)
		}
		var n int
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts").Scan(&n); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("fault wal: pin snapshot: %w", err)
		}
		in.pinConn = conn
	}
	return in, nil
}

// multiplier is how many nominal intervals elapsed since the previous Tick
// (at least 1), and advances the clock.
func (in *injector) multiplier() int {
	now := time.Now()
	m := 1
	if in.every > 0 {
		if r := int((now.Sub(in.last) + in.every/2) / in.every); r > m {
			m = r
		}
	}
	in.last = now
	return m
}

// Tick leaks one nominal increment of each fault's resource, times the number
// of nominal intervals that elapsed since the last Tick.
//
// Why time-scaled: the sampler's ticker drops ticks when the process is
// starved (a -race runner shared with the rest of the package shard), so a
// "14 s" run can deliver ~6 samples instead of 14. A fixed amount per tick
// then shrank the total leak with the runner's load, right when the start-up
// burst (goroutines, fds and pooled connections spike for the first seconds,
// and with few samples that spike sits inside the 2/3 tail window) is
// largest. The fitted slope went flat or negative and a genuine leak went
// undetected (issue #1675). Scaling by elapsed intervals makes the leak a
// function of wall-clock time, which the fit's time axis already is.
func (in *injector) Tick() {
	in.tick++
	m := in.multiplier()
	if in.has(FaultGoroutines) {
		for i := 0; i < leakGoroutines*m; i++ {
			go func() { <-in.stop }()
		}
	}
	if in.has(FaultHeap) {
		for k := 0; k < m; k++ {
			b := make([]byte, leakHeapBytes)
			for i := 0; i < len(b); i += 4096 {
				b[i] = 1 // touch every page so it is resident, not just reserved
			}
			in.retain = append(in.retain, b)
		}
	}
	if in.has(FaultFDs) {
		for i := 0; i < leakFDs*m; i++ {
			if f, err := os.Open(os.DevNull); err == nil {
				in.files = append(in.files, f)
			}
		}
	}
	if in.has(FaultLimiter) {
		rl := middleware.GetAccountRateLimiter()
		for i := 0; i < leakLimiter*m; i++ {
			rl.RecordFailedAttempt(fmt.Sprintf("soak-fault-%d-%d-%d", in.seq, in.tick, i))
		}
	}
	if in.has(FaultDBConns) && in.db != nil {
		if sqlDB, err := in.db.DB(); err == nil {
			for i := 0; i < leakDBConns*m; i++ {
				if c, err := sqlDB.Conn(in.ctx); err == nil {
					in.conns = append(in.conns, c)
				}
			}
		}
	}
	// FaultWAL pins its snapshot at construction. It also writes ballast every
	// tick: the pinned reader only starves the checkpoint, so the WAL still
	// has to be fed, and feeding it from the workload made detection depend
	// on how many writes a contended runner managed (7.98 MiB observed
	// against an 8 MiB ceiling after 197 ops). 1 MiB per tick reaches the
	// ceiling in a few seconds whatever the throughput.
	if in.has(FaultWAL) && in.db != nil {
		// Best effort: a failed write just means less ballast this tick.
		if err := in.db.Exec("CREATE TABLE IF NOT EXISTS soak_fault_ballast (b BLOB)").Error; err == nil {
			_ = in.db.Exec("INSERT INTO soak_fault_ballast (b) VALUES (zeroblob(1048576))").Error
		}
	}
}

// Close releases everything the fault holds.
func (in *injector) Close() {
	in.closeMu.Do(func() {
		close(in.stop)
		for _, f := range in.files {
			_ = f.Close()
		}
		for _, c := range in.conns {
			_ = c.Close()
		}
		in.conns = nil
		if in.pinConn != nil {
			_, _ = in.pinConn.ExecContext(context.Background(), "ROLLBACK")
			_ = in.pinConn.Close()
		}
		in.retain = nil
		runtime.GC()
	})
}
