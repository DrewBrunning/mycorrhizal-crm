package perfbench

import (
	"fmt"
	"os"
	"sort"

	"mycorrhizal/internal/largedata"

	"gorm.io/gorm"
)

// DataMovementResult is one bulk operation measured at one profile (PERF-03,
// issue #470): the resource envelope plus the scale it ran at.
type DataMovementResult struct {
	Operation string `json:"operation"`
	Profile   string `json:"profile"`
	Category  string `json:"category"`

	// RowScale is the dataset size (total contacts across every user) — the
	// figure the "memory vs dataset" growth is measured against.
	RowScale int `json:"row_scale"`

	ExpectedMemoryGrowth GrowthClass `json:"expected_memory_growth"`
	FoldResultGrowth     bool        `json:"fold_result_growth"`

	Sample ResourceSample `json:"sample"`
}

// MemoryGrowthFinding is one operation's PEAK-HEAP growth from the smallest to
// the largest measured profile, against how much its own work (or, for the
// fold operations, the dataset) grew. Class is derived from the deterministic
// row counts, never the wall clock — the wall clock is carried for context.
type MemoryGrowthFinding struct {
	Operation    string `json:"operation"`
	SmallProfile string `json:"small_profile"`
	LargeProfile string `json:"large_profile"`

	RowScaleRatio    float64 `json:"row_scale_ratio"`
	RowsTouchedRatio float64 `json:"rows_touched_ratio"`
	PeakHeapRatio    float64 `json:"peak_heap_ratio"`
	PeakDiskRatio    float64 `json:"peak_disk_ratio"`
	DurationRatio    float64 `json:"duration_ratio"` // context only

	Class      GrowthClass `json:"class"`
	Expected   GrowthClass `json:"expected"`
	Regression bool        `json:"regression"`
}

// minHeapSignalBytes is the floor below which a peak-heap ratio is treated as
// noise. Sampling runtime.MemStats around a fast operation on the `smoke`
// dataset can catch only a few hundred KB above the baseline, and a ratio
// between two tiny noisy numbers is meaningless. Below this on BOTH ends the
// operation is reported "constant" — there is genuinely no memory-growth
// signal to gate on.
const minHeapSignalBytes = 1 << 19 // 512 KiB

// memLinearSlack is how far peak heap may outgrow the work it does before the
// growth is called super-linear. It is looser than growth.go's query-count
// slack: a bytes.Buffer that ends a bulk write at 2x the final data size
// (mid-realloc, plus GC lag and heap fragmentation the sampler catches) is
// still "memory proportional to the data" — the shape the finding is about is
// O(n^2) accumulation (the duplicate-pair list), not a 1.6x buffer overshoot.
const memLinearSlack = 2.5

// classifyMemoryRatio buckets a peak-heap growth ratio against the ratio of
// the work (or dataset) that drove it.
//
//   - ~flat while the work grew several-fold => constant
//   - grew with the work, within memLinearSlack => linear
//   - grew faster than the work even allowing that slack => superlinear
func classifyMemoryRatio(heapRatio, workRatio float64) GrowthClass {
	const flat = 1.5
	switch {
	case heapRatio <= flat:
		return GrowthConstant
	case workRatio > 1 && heapRatio <= workRatio*memLinearSlack:
		return GrowthLinear
	default:
		return GrowthSuperlinear
	}
}

// dataMovementWorkDir names a per-operation scratch dir under root.
func dataMovementWorkDir(root, op string) (string, error) {
	return os.MkdirTemp(root, "dm-"+sanitizeOpName(op)+"-")
}

func sanitizeOpName(op string) string {
	out := make([]rune, 0, len(op))
	for _, r := range op {
		if r == '.' || r == '/' {
			r = '_'
		}
		out = append(out, r)
	}
	return string(out)
}

// MeasureDataMovement runs one operation once against shared (already the
// forked copy when op.Isolate) and returns its measured envelope. workDir is a
// scratch directory unique to this call.
func MeasureDataMovement(e *Env, op DataMovementOperation, workDir string) (DataMovementResult, error) {
	if op.Prepare != nil {
		if err := op.Prepare(e, workDir); err != nil {
			return DataMovementResult{}, fmt.Errorf("perfbench: prepare %s: %w", op.Name, err)
		}
	}

	var probe *writeProbe
	if op.Probe {
		p, err := newWriteProbe(e.dbPath, probeBusyTimeoutMS)
		if err != nil {
			return DataMovementResult{}, fmt.Errorf("perfbench: write probe for %s: %w", op.Name, err)
		}
		probe = p
	}

	watch := []string{e.dbPath, workDir}
	sample, runErr := sampleResources(watch, probe, func() (int, int64, error) {
		return op.Run(e, workDir)
	})
	if runErr != nil {
		return DataMovementResult{}, fmt.Errorf("perfbench: run %s: %w", op.Name, runErr)
	}

	return DataMovementResult{
		Operation:            op.Name,
		Profile:              e.Profile.Name,
		Category:             op.Category,
		RowScale:             e.ContactCount(),
		ExpectedMemoryGrowth: op.ExpectedMemoryGrowth,
		FoldResultGrowth:     op.FoldResultGrowth,
		Sample:               sample,
	}, nil
}

// RunDataMovementProfile builds an Env for the profile, measures every
// registered data-movement operation against it (mutating ones on their own
// byte copy), and releases the Env.
func RunDataMovementProfile(opts EnvOptions) ([]DataMovementResult, error) {
	shared, err := NewEnv(opts)
	if err != nil { // # pragma: no cover — NewEnv failure modes are pragma'd in env.go
		return nil, err
	}
	defer shared.Close()

	var out []DataMovementResult
	for _, op := range DataMovementRegistry() {
		workDir, err := dataMovementWorkDir(opts.WorkDir, op.Name)
		if err != nil { // # pragma: no cover — MkdirTemp under an existing writable dir
			return nil, fmt.Errorf("perfbench: scratch dir for %s: %w", op.Name, err)
		}

		env := shared
		if op.Isolate {
			forkDir, err := os.MkdirTemp(opts.WorkDir, "dmfork-"+sanitizeOpName(op.Name)+"-")
			if err != nil { // # pragma: no cover — MkdirTemp under a writable dir
				return nil, err
			}
			env, err = shared.forkForDestructive(forkDir)
			if err != nil { // # pragma: no cover — copy + reopen of a healthy local db
				return nil, err
			}
		}

		r, err := MeasureDataMovement(env, op, workDir)
		if op.Isolate {
			env.Close()
		}
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// DataMovementSuite is the full output of a PERF-03 run.
type DataMovementSuite struct {
	ResultsByProfile map[string][]DataMovementResult
	MemoryFindings   []MemoryGrowthFinding
	ProfileOrder     []string
}

// RunAllDataMovement measures every registered operation against each profile
// in order. workRoot mirrors RunAll: "" uses the system temp dir and cleans
// each profile's dir; a non-empty root (a test's t.TempDir()) is left alone.
func RunAllDataMovement(profiles []largedata.Profile, workRoot string, openMigrated func(string) (*gorm.DB, error)) (DataMovementSuite, error) {
	s := DataMovementSuite{ResultsByProfile: map[string][]DataMovementResult{}}
	var all []DataMovementResult
	for _, p := range profiles {
		dir, err := os.MkdirTemp(workRoot, "dmprof-"+p.Name+"-")
		if err != nil { // # pragma: no cover — MkdirTemp under a writable root
			return DataMovementSuite{}, err
		}
		res, err := RunDataMovementProfile(EnvOptions{Profile: p, WorkDir: dir, OpenMigrated: openMigrated})
		if workRoot == "" { // # pragma: no cover — only the cmd passes an empty root
			_ = os.RemoveAll(dir)
		}
		if err != nil {
			return DataMovementSuite{}, fmt.Errorf("perfbench: data-movement profile %q: %w", p.Name, err)
		}
		s.ResultsByProfile[p.Name] = res
		s.ProfileOrder = append(s.ProfileOrder, p.Name)
		all = append(all, res...)
	}
	s.MemoryFindings = AnalyzeMemoryGrowth(all)
	return s, nil
}

// AnalyzeMemoryGrowth pairs each operation's smallest and largest measurement
// and classifies its peak-heap growth. Operations measured at fewer than two
// distinct dataset scales are skipped.
func AnalyzeMemoryGrowth(results []DataMovementResult) []MemoryGrowthFinding {
	byOp := map[string][]DataMovementResult{}
	for _, r := range results {
		byOp[r.Operation] = append(byOp[r.Operation], r)
	}
	names := make([]string, 0, len(byOp))
	for n := range byOp {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []MemoryGrowthFinding
	for _, name := range names {
		rs := byOp[name]
		if len(rs) < 2 {
			continue
		}
		sort.Slice(rs, func(i, j int) bool { return rs[i].RowScale < rs[j].RowScale })
		small, large := rs[0], rs[len(rs)-1]
		if small.RowScale == large.RowScale {
			continue
		}
		out = append(out, classifyMemoryGrowth(small, large))
	}
	return out
}

func classifyMemoryGrowth(small, large DataMovementResult) MemoryGrowthFinding {
	rowScaleRatio := ratio(float64(small.RowScale), float64(large.RowScale))
	rowsTouchedRatio := ratio(float64(small.Sample.RowsTouched), float64(large.Sample.RowsTouched))
	heapRatio := ratio(float64(small.Sample.PeakHeapBytes), float64(large.Sample.PeakHeapBytes))
	diskRatio := ratio(float64(small.Sample.PeakExtraDiskBytes), float64(large.Sample.PeakExtraDiskBytes))
	durRatio := ratio(float64(small.Sample.DurationNanos), float64(large.Sample.DurationNanos))

	// The denominator: for the fold operations (duplicate detection) memory is
	// judged against the DATASET, because their own work count is itself
	// superlinear in the dataset and that is precisely the finding. For every
	// other operation memory is judged against its own work — an exporter that
	// holds one buffer proportional to what it exports is "linear", and that
	// is expected, not a regression.
	denom := rowsTouchedRatio
	if large.FoldResultGrowth {
		denom = rowScaleRatio
	}

	class := GrowthConstant
	if small.Sample.PeakHeapBytes >= minHeapSignalBytes || large.Sample.PeakHeapBytes >= minHeapSignalBytes {
		class = classifyMemoryRatio(heapRatio, denom)
	}

	expected := large.ExpectedMemoryGrowth
	return MemoryGrowthFinding{
		Operation:        large.Operation,
		SmallProfile:     small.Profile,
		LargeProfile:     large.Profile,
		RowScaleRatio:    round2(rowScaleRatio),
		RowsTouchedRatio: round2(rowsTouchedRatio),
		PeakHeapRatio:    round2(heapRatio),
		PeakDiskRatio:    round2(diskRatio),
		DurationRatio:    round2(durRatio),
		Class:            class,
		Expected:         expected,
		Regression:       class.rank() > expected.rank(),
	}
}

// Baseline builds the committed PERF-03 baseline for this suite.
func (s DataMovementSuite) Baseline() DataMovementBaseline {
	return BuildDataMovementBaseline(s.ResultsByProfile, s.ProfileOrder)
}

// Report renders the human-facing PERF-03 markdown report.
func (s DataMovementSuite) Report() string {
	return RenderDataMovementReport(s.ResultsByProfile, s.ProfileOrder, s.MemoryFindings)
}
