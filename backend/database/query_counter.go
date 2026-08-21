package database

import (
	"context"
	"sync/atomic"
	"time"

	gormLogger "gorm.io/gorm/logger"
)

// QueryCounter is a gorm logger that tallies every traced SQL statement, for
// performance tests that pin a handler's query count (issue #261). GORM logs
// every real DML/SELECT statement through logger.Trace, so wrapping the
// default logger and counting Trace calls is a dependency-free way to catch an
// N+1 regression deterministically: a loop that runs one query per row shows
// up as a count that grows with the seeded data size.
//
// BEGIN/COMMIT from gorm.DB.Transaction bypass the callback logger (they go
// straight to the connection pool), so the tally reflects real statements only
// — the only thing a query-shape assertion cares about.
//
// Use it by opening a second GORM connection to the same SQLite file with
// Logger: counter, seeding on a separate connection, then asserting
// counter.Count() after the measured call. Seeding must not share the counted
// connection, or its writes pollute the tally.
type QueryCounter struct {
	gormLogger.Interface
	n atomic.Int64
}

// NewQueryCounter returns a counter wrapping the default gorm logger in Silent
// mode, so a first-run job (no job-lock/cursor rows yet) does not spam
// "record not found" into the test log — the tally is what the tests read.
func NewQueryCounter() *QueryCounter {
	return &QueryCounter{Interface: gormLogger.Default.LogMode(gormLogger.Silent)}
}

// Trace implements gormLogger.Interface.
func (c *QueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.n.Add(1)
	c.Interface.Trace(ctx, begin, fc, err)
}

// Count returns the number of traced statements since construction or the last
// Reset.
func (c *QueryCounter) Count() int64 { return c.n.Load() }

// Reset zeroes the tally, e.g. before the measured request when seeding shared
// the counted connection.
func (c *QueryCounter) Reset() { c.n.Store(0) }
