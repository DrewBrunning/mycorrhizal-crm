package perfbench

import (
	"fmt"
	"os"
	"sort"
	"time"

	"mycorrhizal/internal/fireandforget"
)

// readIterations is how many times a non-destructive operation is run for the
// duration median. The query count and result size are read from the first
// measured iteration (they are deterministic; the clock is not), so this only
// governs the median's stability — kept modest to bound CI time under -race.
const readIterations = 3

// Result is one operation measured at one profile.
type Result struct {
	Operation      string      `json:"operation"`
	Profile        string      `json:"profile"`
	Category       string      `json:"category"`
	Queries        int64       `json:"queries"`
	ResultSize     int         `json:"result_size"`
	RowScale       int         `json:"row_scale"`
	Iterations     int         `json:"iterations"`
	MedianNanos    int64       `json:"median_nanos"`
	ExpectedGrowth GrowthClass `json:"expected_growth"`
	// ClassifyResultGrowth mirrors Operation.ClassifyResultGrowth so growth.go
	// can fold result-size growth into the class for the pairwise operations.
	ClassifyResultGrowth bool `json:"classify_result_growth"`
}

// Measure runs op against e and records its query count, result size and
// median wall-clock time.
//
// fireandforget.Wait bookends every iteration: the write handlers fan a
// webhook-lookup query out through internal/fireandforget (context.WithoutCancel,
// so it always runs), and it must be drained before the counter is read or its
// query lands in the next iteration's window. That one query IS part of a
// write's real cost, so it is deliberately counted — the baseline records it.
func Measure(e *Env, op Operation) (Result, error) {
	iters := readIterations
	warmUp := true
	switch {
	case op.Destructive:
		iters = 1
		warmUp = false // the single run IS the measurement; there is no second chance.
	case op.SlowRead:
		iters = 1 // query count (iter 0) is still deterministic; the median is advisory
	}

	prepare := func() error {
		if op.Prepare == nil {
			return nil
		}
		return op.Prepare(e)
	}

	if warmUp {
		fireandforget.Wait()
		if err := prepare(); err != nil { // # pragma: no cover — the registry's Prepare hooks issue only always-valid SQL
			return Result{}, fmt.Errorf("perfbench: prepare %s: %w", op.Name, err)
		}
		if _, err := op.Run(e); err != nil {
			return Result{}, fmt.Errorf("perfbench: warm-up %s: %w", op.Name, err)
		}
	}

	var (
		queries int64
		size    int
		durs    = make([]time.Duration, 0, iters)
	)
	for i := 0; i < iters; i++ {
		fireandforget.Wait()
		if err := prepare(); err != nil { // # pragma: no cover — see the warm-up prepare note
			return Result{}, fmt.Errorf("perfbench: prepare %s: %w", op.Name, err)
		}
		e.Counter.Reset()
		start := time.Now()
		s, err := op.Run(e)
		elapsed := time.Since(start)
		if err != nil {
			return Result{}, fmt.Errorf("perfbench: run %s: %w", op.Name, err)
		}
		fireandforget.Wait()
		if i == 0 {
			queries = e.Counter.Count()
			size = s
		}
		durs = append(durs, elapsed)
	}

	return Result{
		Operation:            op.Name,
		Profile:              e.Profile.Name,
		Category:             op.Category,
		Queries:              queries,
		ResultSize:           size,
		RowScale:             e.ContactCount(),
		Iterations:           iters,
		MedianNanos:          medianNanos(durs),
		ExpectedGrowth:       op.ExpectedGrowth,
		ClassifyResultGrowth: op.ClassifyResultGrowth,
	}, nil
}

// RunProfile builds an Env for opts.Profile, measures every registered
// operation against it, and releases the Env.
func RunProfile(opts EnvOptions) ([]Result, error) {
	env, out, err := RunProfileKeepEnv(opts)
	if env != nil {
		env.Close()
	}
	return out, err
}

// RunProfileKeepEnv is RunProfile but hands the shared Env back to the caller
// (who must Close it). Tests reuse it to run extra assertions against the same
// populated fixture instead of paying for a second migrate + populate.
// Destructive operations still run against their own byte copy of the fixture.
func RunProfileKeepEnv(opts EnvOptions) (*Env, []Result, error) {
	shared, err := NewEnv(opts)
	if err != nil { // # pragma: no cover — NewEnv failure modes are all pragma'd there
		return nil, nil, err
	}

	var out []Result
	for _, op := range operationsForProfile(opts.Profile) {
		env := shared
		if op.Destructive {
			sub, err := os.MkdirTemp(opts.WorkDir, "op-"+op.Name+"-")
			if err != nil { // # pragma: no cover — MkdirTemp under an existing writable dir
				shared.Close()
				return nil, nil, fmt.Errorf("perfbench: sub-dir for %s: %w", op.Name, err)
			}
			// A byte copy of the already-populated fixture — no second
			// migrate + populate (the dominant cost under CI -race).
			env, err = shared.forkForDestructive(sub)
			if err != nil { // # pragma: no cover — copy + reopen of a healthy local db
				shared.Close()
				return nil, nil, err
			}
		}
		r, err := Measure(env, op)
		if op.Destructive {
			env.Close()
		}
		if err != nil { // # pragma: no cover — every registered operation succeeds against a freshly populated fixture
			shared.Close()
			return nil, nil, err
		}
		out = append(out, r)
	}
	return shared, out, nil
}

func medianNanos(durs []time.Duration) int64 {
	if len(durs) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid].Nanoseconds()
	}
	return (sorted[mid-1].Nanoseconds() + sorted[mid].Nanoseconds()) / 2
}
