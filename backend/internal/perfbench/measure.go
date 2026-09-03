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
// measured iteration (they are deterministic; the clock is not).
const readIterations = 7

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
	if op.Destructive {
		iters = 1
		warmUp = false // the single run IS the measurement; there is no second chance.
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

// RunProfile builds an Env for opts.Profile and measures every registered
// operation against it. Destructive operations each get their own fresh Env
// (a sub-directory copy) so they never perturb another operation's numbers.
func RunProfile(opts EnvOptions) ([]Result, error) {
	shared, err := NewEnv(opts)
	if err != nil { // # pragma: no cover — NewEnv failure modes are all pragma'd there
		return nil, err
	}
	defer shared.Close()

	var out []Result
	for _, op := range Registry() {
		env := shared
		if op.Destructive {
			sub, err := os.MkdirTemp(opts.WorkDir, "op-"+op.Name+"-")
			if err != nil { // # pragma: no cover — MkdirTemp under an existing writable dir
				return nil, fmt.Errorf("perfbench: sub-dir for %s: %w", op.Name, err)
			}
			env, err = NewEnv(EnvOptions{Profile: opts.Profile, WorkDir: sub, OpenMigrated: opts.OpenMigrated})
			if err != nil { // # pragma: no cover — same fresh-schema path NewEnv already succeeded on above
				return nil, err
			}
		}
		r, err := Measure(env, op)
		if op.Destructive {
			env.Close()
		}
		if err != nil { // # pragma: no cover — every registered operation succeeds against a freshly populated fixture
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
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
