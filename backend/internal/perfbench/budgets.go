package perfbench

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// BudgetsFile is the committed budgets file's path, relative to this package.
const BudgetsFile = "testdata/budgets.json"

// BudgetsDoc is the human-facing governance doc, relative to the repo root.
const BudgetsDoc = "docs/development/perf-budgets.md"

//go:embed testdata/budgets.json
var embeddedBudgets []byte

// Budget sanity bounds. A wall-clock budget outside this band, or a
// peak-heap ceiling outside its own, is almost certainly a typo rather than a
// deliberate decision — Validate rejects it. `max_wall_ms: 0` is the explicit
// waiver (kept for an operation that is not on any interactive path); the
// paired reason must still say why.
const (
	minWallMS    = 50
	maxWallMS    = 10_000
	minHeapMiB   = 16
	maxHeapMiB   = 4_096
	mib          = 1 << 20
	wallMSToNano = int64(time.Millisecond)
)

// Budgets is the HAND-AUTHORED performance-budget record (PERF-04, issue
// #471). Unlike baseline.json / datamovement-baseline.json — which are
// generated measurements — this file is a set of governance decisions:
//
//   - The deterministic budget (query count, rows touched) IS the committed
//     PERF-02/PERF-03 baseline: an operation must not issue more queries or
//     touch more rows than it was last measured to. CheckCore / CheckDataMovement
//     re-assert that from the "budget" vocabulary, and it is the per-PR gate.
//   - The absolute wall-clock budget (max_wall_ms) is a hard ceiling on the
//     median at the anchor profile for the short list of operations that sit on
//     an interactive path and must stay under a human-perceptible threshold.
//     It is evaluated only at the anchor profile, i.e. only in release
//     validation (the gated at-scale tests), never on a noisy per-PR runner.
//   - The peak-heap ceiling (max_peak_heap_mib) is a hard cap that must hold
//     because exceeding it means an OOM kill on a small self-hosted box, not a
//     slow response. Also anchor-profile / release-validation only.
//
// Every operation from #469 and #470 has an entry with a non-empty reason —
// either a real ceiling or an explicit waiver (`max_wall_ms: 0`). Changing a
// number here is a deliberate, reviewed edit that must update the sibling
// reason; see docs/development/perf-budgets.md.
type Budgets struct {
	Note string `json:"note"`
	// AnchorProfile is the dataset profile the absolute (wall-clock, memory)
	// budgets are evaluated at — the "intended MVP scale" the milestone talks
	// about. Deterministic budgets are checked at every measured profile.
	AnchorProfile string `json:"anchor_profile"`

	CoreOperations         map[string]CoreBudget         `json:"core_operations"`
	DataMovementOperations map[string]DataMovementBudget `json:"data_movement_operations"`
}

// CoreBudget is one PERF-02 operation's budget.
type CoreBudget struct {
	// MaxWallMS is the absolute median-wall-clock ceiling at the anchor
	// profile, in milliseconds. 0 waives the wall-clock budget (the operation
	// is not on an interactive path); Reason must then say so.
	MaxWallMS int    `json:"max_wall_ms"`
	Reason    string `json:"reason"`
}

// DataMovementBudget is one PERF-03 bulk operation's budget.
type DataMovementBudget struct {
	// MaxPeakHeapMiB is the absolute peak-heap ceiling at the anchor profile,
	// in MiB. Required (> 0) for every bulk operation — memory is the axis
	// that OOM-kills a bulk job.
	MaxPeakHeapMiB int    `json:"max_peak_heap_mib"`
	Reason         string `json:"reason"`
}

// BreachKind names which budget an operation blew.
type BreachKind string

const (
	BreachQueryCount  BreachKind = "query_count"
	BreachRowsTouched BreachKind = "rows_touched"
	BreachWallClock   BreachKind = "wall_clock"
	BreachPeakHeap    BreachKind = "peak_heap"
	// BreachUnbudgeted is an operation with no entry at all — a structural
	// failure the coverage test (TestBudgetsCoverEveryOperation) is the real
	// guard for; CheckCore/CheckDataMovement surface it defensively.
	BreachUnbudgeted BreachKind = "unbudgeted"
)

// Breach is one operation exceeding one budget at one profile.
type Breach struct {
	Operation string     `json:"operation"`
	Profile   string     `json:"profile"`
	Kind      BreachKind `json:"kind"`
	Measured  int64      `json:"measured"`
	Budget    int64      `json:"budget"`
	Detail    string     `json:"detail"`
}

func (b Breach) String() string {
	if b.Kind == BreachUnbudgeted {
		return fmt.Sprintf("%s: %s", b.Operation, b.Detail)
	}
	measured, budget := fmt.Sprintf("%d", b.Measured), fmt.Sprintf("%d", b.Budget)
	if b.Kind == BreachWallClock || b.Kind == BreachPeakHeap {
		measured, budget = humanBudget(b.Kind, b.Measured), humanBudget(b.Kind, b.Budget)
	}
	return fmt.Sprintf("%s @ %s: %s %s over budget %s (%s)", b.Operation, b.Profile, b.Kind, measured, budget, b.Detail)
}

func humanBudget(kind BreachKind, v int64) string {
	if kind == BreachPeakHeap {
		return humanBytes(v)
	}
	return humanDuration(v)
}

// CheckCore evaluates every core Result against the budgets, using base for
// the deterministic (query-count) ceiling. Wall-clock budgets apply only at
// the anchor profile.
func (bg Budgets) CheckCore(results []Result, base Baseline) []Breach {
	var out []Breach
	for _, r := range results {
		cb, ok := bg.CoreOperations[r.Operation]
		if !ok {
			out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachUnbudgeted,
				Detail: "no budget entry in budgets.json — add one with a reason"})
			continue
		}
		if pm, ok := baselineMetric(base, r.Operation, r.Profile); ok && r.Queries > pm.Queries {
			out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachQueryCount,
				Measured: r.Queries, Budget: pm.Queries,
				Detail: "query count over the committed baseline — an N+1 regression, or regenerate the baseline if deliberate"})
		}
		if r.Profile == bg.AnchorProfile && cb.MaxWallMS > 0 {
			budget := int64(cb.MaxWallMS) * wallMSToNano
			if r.MedianNanos > budget {
				out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachWallClock,
					Measured: r.MedianNanos, Budget: budget,
					Detail: "median wall-clock over the absolute interactive budget"})
			}
		}
	}
	return out
}

// CheckDataMovement evaluates every data-movement Result against the budgets,
// using base for the deterministic (rows-touched) ceiling. Peak-heap budgets
// apply only at the anchor profile.
func (bg Budgets) CheckDataMovement(results []DataMovementResult, base DataMovementBaseline) []Breach {
	var out []Breach
	for _, r := range results {
		db, ok := bg.DataMovementOperations[r.Operation]
		if !ok {
			out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachUnbudgeted,
				Detail: "no budget entry in budgets.json — add one with a reason"})
			continue
		}
		if pm, ok := dmBaselineMetric(base, r.Operation, r.Profile); ok && r.Sample.RowsTouched > pm.RowsTouched {
			out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachRowsTouched,
				Measured: int64(r.Sample.RowsTouched), Budget: int64(pm.RowsTouched),
				Detail: "rows touched over the committed baseline — regenerate the baseline if deliberate"})
		}
		if r.Profile == bg.AnchorProfile && db.MaxPeakHeapMiB > 0 {
			budget := int64(db.MaxPeakHeapMiB) * mib
			if r.Sample.PeakHeapBytes > budget {
				out = append(out, Breach{Operation: r.Operation, Profile: r.Profile, Kind: BreachPeakHeap,
					Measured: r.Sample.PeakHeapBytes, Budget: budget,
					Detail: "peak heap over the absolute ceiling — an accumulation bug that will OOM a small box"})
			}
		}
	}
	return out
}

// Validate checks the budgets file is well-formed against the live operation
// registries: every registered operation has an entry with a non-empty
// reason, every ceiling is within its sanity band, and no entry names an
// operation that no longer exists. It returns a sorted list of problems (empty
// == valid).
func (bg Budgets) Validate(core []Operation, dm []DataMovementOperation) []string {
	var problems []string
	if strings.TrimSpace(bg.AnchorProfile) == "" {
		problems = append(problems, "anchor_profile is empty")
	}

	knownCore := map[string]bool{}
	for _, op := range core {
		knownCore[op.Name] = true
		cb, ok := bg.CoreOperations[op.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("core operation %q has no budget entry", op.Name))
			continue
		}
		if strings.TrimSpace(cb.Reason) == "" {
			problems = append(problems, fmt.Sprintf("core budget %q has an empty reason", op.Name))
		}
		if cb.MaxWallMS != 0 && (cb.MaxWallMS < minWallMS || cb.MaxWallMS > maxWallMS) {
			problems = append(problems, fmt.Sprintf("core budget %q: max_wall_ms %d outside [%d,%d] (use 0 to waive)",
				op.Name, cb.MaxWallMS, minWallMS, maxWallMS))
		}
	}
	for name := range bg.CoreOperations {
		if !knownCore[name] {
			problems = append(problems, fmt.Sprintf("core budget %q is not a registered operation (stale entry?)", name))
		}
	}

	knownDM := map[string]bool{}
	for _, op := range dm {
		knownDM[op.Name] = true
		db, ok := bg.DataMovementOperations[op.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("data-movement operation %q has no budget entry", op.Name))
			continue
		}
		if strings.TrimSpace(db.Reason) == "" {
			problems = append(problems, fmt.Sprintf("data-movement budget %q has an empty reason", op.Name))
		}
		if db.MaxPeakHeapMiB < minHeapMiB || db.MaxPeakHeapMiB > maxHeapMiB {
			problems = append(problems, fmt.Sprintf("data-movement budget %q: max_peak_heap_mib %d outside [%d,%d]",
				op.Name, db.MaxPeakHeapMiB, minHeapMiB, maxHeapMiB))
		}
	}
	for name := range bg.DataMovementOperations {
		if !knownDM[name] {
			problems = append(problems, fmt.Sprintf("data-movement budget %q is not a registered operation (stale entry?)", name))
		}
	}

	sort.Strings(problems)
	return problems
}

func baselineMetric(b Baseline, op, profile string) (ProfileMetric, bool) {
	ob, ok := b.Operations[op]
	if !ok {
		return ProfileMetric{}, false
	}
	pm, ok := ob.Profiles[profile]
	return pm, ok
}

func dmBaselineMetric(b DataMovementBaseline, op, profile string) (DataMovementProfileMetric, bool) {
	ob, ok := b.Operations[op]
	if !ok {
		return DataMovementProfileMetric{}, false
	}
	pm, ok := ob.Profiles[profile]
	return pm, ok
}

// EmbeddedBudgets returns the budgets file compiled into the binary.
func EmbeddedBudgets() (Budgets, error) { return parseBudgets(embeddedBudgets) }

// LoadBudgets parses a budgets file from disk.
func LoadBudgets(path string) (Budgets, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- build-tool argument, not request input
	if err != nil {
		return Budgets{}, err
	}
	return parseBudgets(data)
}

func parseBudgets(data []byte) (Budgets, error) {
	var b Budgets
	if err := json.Unmarshal(data, &b); err != nil {
		return Budgets{}, fmt.Errorf("perfbench: parsing budgets: %w", err)
	}
	return b, nil
}
