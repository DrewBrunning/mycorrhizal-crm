package perfbench

import "sort"

// GrowthFinding is one operation's cost shape across two dataset scales.
//
// The issue asks to "look for the shape of the cost, not just its value" and
// to flag "any super-linear operation ... as a finding, not just a number".
// Classification uses only the DETERMINISTIC signals — query count and result
// size — so a run is reproducible. Wall-clock ratios are carried for context
// but never drive the class.
type GrowthFinding struct {
	Operation string `json:"operation"`

	SmallProfile string `json:"small_profile"`
	LargeProfile string `json:"large_profile"`

	RowScaleRatio float64 `json:"row_scale_ratio"`
	QueryRatio    float64 `json:"query_ratio"`
	ResultRatio   float64 `json:"result_ratio"`
	DurationRatio float64 `json:"duration_ratio"` // context only

	Class      GrowthClass `json:"class"`
	Expected   GrowthClass `json:"expected"`
	Regression bool        `json:"regression"` // Class is worse than Expected

	// DurationConcern is an ADVISORY flag: wall-clock grew faster than the
	// square of the row-count ratio. It never fails a test (wall-clock is
	// hardware-variable, issue #261) but it is the "fast at the typical
	// profile, outage at large" signal the ticket asks to surface, so the
	// report calls it out.
	DurationConcern bool `json:"duration_concern"`
}

// classifyRatio buckets one growth ratio against the row-count ratio.
//
//   - ~flat (<= 1.5x while the data grew several-fold) => constant
//   - grows with the data, within a slack factor of the row-count ratio => linear
//   - grows faster than the data => superlinear
func classifyRatio(ratio, rowRatio float64) GrowthClass {
	const flat = 1.5
	const linearSlack = 1.4
	switch {
	case ratio <= flat:
		return GrowthConstant
	case rowRatio > 1 && ratio <= rowRatio*linearSlack:
		return GrowthLinear
	default:
		return GrowthSuperlinear
	}
}

func ratio(small, large float64) float64 {
	if small <= 0 {
		if large <= 0 {
			return 1
		}
		return large // treat "0 -> N" as an N-fold increase
	}
	return large / small
}

// Classify compares one operation measured at two scales (small must be the
// smaller RowScale).
func Classify(small, large Result) GrowthFinding {
	rowRatio := ratio(float64(small.RowScale), float64(large.RowScale))
	qRatio := ratio(float64(small.Queries), float64(large.Queries))
	rRatio := ratio(float64(small.ResultSize), float64(large.ResultSize))
	dRatio := ratio(float64(small.MedianNanos), float64(large.MedianNanos))

	class := classifyRatio(qRatio, rowRatio)
	// Result-set growth is a real cost shape even when the query count is flat
	// (the pairwise duplicate scan) — but only for operations that opt in.
	// A page-limited list or a density-driven graph traversal returns more
	// rows on a bigger dataset by design, not by inefficiency.
	if large.ClassifyResultGrowth && (small.ResultSize > 0 || large.ResultSize > 0) {
		if rc := classifyRatio(rRatio, rowRatio); rc.rank() > class.rank() {
			class = rc
		}
	}

	expected := large.ExpectedGrowth
	return GrowthFinding{
		Operation:       large.Operation,
		SmallProfile:    small.Profile,
		LargeProfile:    large.Profile,
		RowScaleRatio:   round2(rowRatio),
		QueryRatio:      round2(qRatio),
		ResultRatio:     round2(rRatio),
		DurationRatio:   round2(dRatio),
		Class:           class,
		Expected:        expected,
		Regression:      class.rank() > expected.rank(),
		DurationConcern: rowRatio > 1 && dRatio > rowRatio*rowRatio,
	}
}

// AnalyzeGrowth pairs up every operation's smallest and largest measurement
// and classifies each. results may mix profiles; operations measured at fewer
// than two distinct scales are skipped.
func AnalyzeGrowth(results []Result) []GrowthFinding {
	byOp := map[string][]Result{}
	for _, r := range results {
		byOp[r.Operation] = append(byOp[r.Operation], r)
	}

	names := make([]string, 0, len(byOp))
	for name := range byOp {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []GrowthFinding
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
		out = append(out, Classify(small, large))
	}
	return out
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
