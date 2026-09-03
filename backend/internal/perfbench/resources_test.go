package perfbench

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTotalSize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.db"), make([]byte, 1000), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.db-wal"), make([]byte, 200), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.db-shm"), make([]byte, 32), 0o600))
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 500), 0o600))

	t.Run("file counts its wal/shm sidecars", func(t *testing.T) {
		assert.Equal(t, int64(1232), totalSize([]string{filepath.Join(dir, "a.db")}))
	})
	t.Run("directory is walked recursively", func(t *testing.T) {
		// a.db + a.db-wal + a.db-shm + sub/b.bin
		assert.Equal(t, int64(1732), totalSize([]string{dir}))
	})
	t.Run("missing path contributes zero", func(t *testing.T) {
		assert.Equal(t, int64(0), totalSize([]string{filepath.Join(dir, "nope")}))
	})
}

func TestSampleResources_RecordsEnvelope(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "artifact.bin")

	var held []byte
	sample, err := sampleResources([]string{dir}, nil, func() (int, int64, error) {
		held = make([]byte, 4<<20) // 4 MiB live across the sampling window
		for i := 0; i < len(held); i += 4096 {
			held[i] = 1
		}
		require.NoError(t, os.WriteFile(out, make([]byte, 3<<20), 0o600))
		time.Sleep(80 * time.Millisecond) // let both samplers tick (heap 5ms, disk 20ms)
		runtime.KeepAlive(held)
		return 42, 7, nil
	})
	require.NoError(t, err)

	assert.Equal(t, 42, sample.RowsTouched)
	assert.Equal(t, int64(7), sample.OutputBytes)
	assert.Positive(t, sample.DurationNanos)
	assert.GreaterOrEqual(t, sample.HeapSamples, 1)
	assert.GreaterOrEqual(t, sample.DiskSamples, 1)
	assert.Greater(t, sample.PeakHeapBytes, int64(2<<20), "the 4MiB allocation should show in the peak")
	assert.GreaterOrEqual(t, sample.PeakExtraDiskBytes, int64(3<<20), "the 3MiB artifact should show in the disk peak")
	assert.False(t, sample.WriteProbed)
}

func TestSampleResources_PropagatesRunError(t *testing.T) {
	_, err := sampleResources(nil, nil, func() (int, int64, error) {
		return 0, 0, assert.AnError
	})
	assert.ErrorIs(t, err, assert.AnError)
}

func TestWriteProbe_EmptyPathIsNoProbe(t *testing.T) {
	p, err := newWriteProbe("", probeBusyTimeoutMS)
	require.NoError(t, err)
	assert.Nil(t, p)
}

func TestWriteProbe_CountsWritesWhenUncontended(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "probe.db")
	// Touch the file into existence as a valid empty SQLite db.
	seed, err := sql.Open("sqlite", probeDSN(dbPath, probeBusyTimeoutMS))
	require.NoError(t, err)
	_, err = seed.Exec("CREATE TABLE t (x)")
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	p, err := newWriteProbe(dbPath, probeBusyTimeoutMS)
	require.NoError(t, err)
	p.start()
	time.Sleep(60 * time.Millisecond)
	p.stop()

	assert.Positive(t, p.writes, "an uncontended probe completes writes")
	assert.False(t, p.stalledOut)
}

func TestWriteProbe_StallsAndTimesOutUnderALockHold(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "probe.db")
	holder, err := sql.Open("sqlite", probeDSN(dbPath, probeBusyTimeoutMS))
	require.NoError(t, err)
	defer holder.Close()
	_, err = holder.Exec("CREATE TABLE t (x)")
	require.NoError(t, err)

	// A short busy_timeout on the probe so the stall-out branch is reachable
	// without a real multi-second lock hold.
	p, err := newWriteProbe(dbPath, 120)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx, err := holder.Begin() // BEGIN IMMEDIATE via _txlock=immediate — takes the write lock
		require.NoError(t, err)
		_, err = tx.Exec("INSERT INTO t VALUES (1)")
		require.NoError(t, err)
		time.Sleep(400 * time.Millisecond) // hold it past the probe's 120ms give-up
		require.NoError(t, tx.Commit())
	}()

	p.start()
	time.Sleep(500 * time.Millisecond)
	p.stop()
	wg.Wait()

	assert.True(t, p.stalledOut, "a probe write blocked past its busy_timeout is a stall-out")
	assert.Greater(t, p.maxStall, 100*time.Millisecond)
}

func TestSanitizeOpName(t *testing.T) {
	assert.Equal(t, "import_vcf", sanitizeOpName("import.vcf"))
	assert.Equal(t, "a_b_c", sanitizeOpName("a/b.c"))
	assert.Equal(t, "plain", sanitizeOpName("plain"))
}

func TestHumanBytes(t *testing.T) {
	assert.Equal(t, "0B", humanBytes(-5))
	assert.Equal(t, "512B", humanBytes(512))
	assert.Equal(t, "1.0KiB", humanBytes(1024))
	assert.Equal(t, "1.5KiB", humanBytes(1536))
	assert.Equal(t, "2.0MiB", humanBytes(2<<20))
	assert.Equal(t, "3.0GiB", humanBytes(3<<30))
}

func TestProbeDSN(t *testing.T) {
	assert.Equal(t,
		"/x/y.db?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate",
		probeDSN("/x/y.db", 5000))
}
