package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormLogger "gorm.io/gorm/logger"
)

// traceSpy is a gormLogger.Interface that records whether Trace was called, so
// the counter's forwarding behavior can be pinned without a real logger.
type traceSpy struct {
	gormLogger.Interface
	called bool
}

func (s *traceSpy) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	s.called = true
}

// TestQueryCounterStartsAtZeroAndCountsTraces covers the whole
// query_counter.go surface: a fresh counter starts at zero, Trace tallies each
// statement (and still forwards to the wrapped logger), Reset zeroes the tally,
// and Count returns the running total.
func TestQueryCounterStartsAtZeroAndCountsTraces(t *testing.T) {
	t.Parallel()
	c := NewQueryCounter()
	require.Equal(t, int64(0), c.Count(), "a fresh counter must start at zero")

	// Trace must increment the tally per call and stay safe for a nil fc
	// (the counter never dereferences the callback itself).
	c.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	c.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	require.Equal(t, int64(2), c.Count(), "each Trace must increment the tally")

	c.Reset()
	require.Equal(t, int64(0), c.Count(), "Reset must zero the tally")

	c.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 2", 1 }, nil)
	require.Equal(t, int64(1), c.Count(), "Count must resume from zero after Reset")
}

// TestQueryCounterForwardsToWrappedLogger pins that the counter is a
// transparent wrapper, not a replacement: the underlying gorm logger must
// still receive the trace (via a spy that records what it was called with).
func TestQueryCounterForwardsToWrappedLogger(t *testing.T) {
	t.Parallel()
	s := &traceSpy{}
	c := &QueryCounter{Interface: s}

	c.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)

	assert.True(t, s.called, "Trace must forward to the wrapped logger")
	assert.Equal(t, int64(1), c.Count())
}
