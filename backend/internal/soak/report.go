package soak

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// sparkRunes are the eight block heights of a text sparkline.
var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders s as a one-line unicode chart of at most width cells
// (resampled by taking the last value in each bucket). A flat series renders
// as a flat low line — the shape of a healthy run.
func Sparkline(s Series, width int) string {
	if len(s) == 0 || width < 1 {
		return ""
	}
	vals := make([]float64, 0, width)
	if len(s) <= width {
		for _, p := range s {
			vals = append(vals, p.V)
		}
	} else {
		for i := 0; i < width; i++ {
			lo := i * len(s) / width
			hi := (i+1)*len(s)/width - 1
			if hi < lo {
				hi = lo
			}
			vals = append(vals, s[hi].V)
		}
	}
	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	out := make([]rune, len(vals))
	for i, v := range vals {
		idx := 0
		if max > min {
			idx = int((v - min) / (max - min) * float64(len(sparkRunes)-1))
		}
		out[i] = sparkRunes[idx]
	}
	return string(out)
}

// seriesOrder is the stable row order of the report's signal table.
var seriesOrder = []string{
	SigGoroutines, SigOpenFDs, SigHeapInuse, SigRSS, SigWALBytes, SigDBBytes,
	SigLimiterEntries, SigDBConnsOpen, SigInFlight, SigP95Read, SigP95Write, SigP95Auth,
}

// WriteJSON writes the full report (including every sampled series, the raw
// data behind the graphs) as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// verdictFor finds the verdict for sig, if it has a budget.
func (r *Report) verdictFor(sig string) (Verdict, bool) {
	for _, v := range r.Verdicts {
		if v.Budget.Signal == sig {
			return v, true
		}
	}
	return Verdict{}, false
}

// Markdown renders the report for a CI step summary / issue comment: run
// parameters, the signal table with sparklines, the verdicts, the end-of-run
// checks and any failures.
func (r *Report) Markdown() string {
	var sb strings.Builder
	status := "PASS"
	if !r.OK() {
		status = "FAIL"
	}
	fmt.Fprintf(&sb, "### Soak run: %s\n\n", status)
	fmt.Fprintf(&sb, "- mode `%s`, %.0fs at %.0f ops/s over %d users, sampled every %.0fs, %s",
		r.Mode, r.Duration, r.Rate, r.Users, r.SampleEvery, r.GoVersion)
	if len(r.Faults) > 0 {
		fmt.Fprintf(&sb, ", **injected faults %v** (harness self-test)", r.Faults)
	}
	fmt.Fprintf(&sb, "\n- workload: %d operations, %d server errors, %d rate-limited, %d dropped (server slower than offered rate)\n\n",
		r.Workload.Total, r.Workload.ServerErrs, r.Workload.RateLimited, r.Workload.Dropped)

	sb.WriteString("| Signal | Start | End | Tail max | Tail growth | Budget | Verdict | Shape |\n|---|---|---|---|---|---|---|---|\n")
	order := append([]string(nil), seriesOrder...)
	known := map[string]bool{}
	for _, s := range order {
		known[s] = true
	}
	var extra []string
	for s := range r.Series {
		if !known[s] {
			extra = append(extra, s)
		}
	}
	sort.Strings(extra)
	for _, sig := range append(order, extra...) {
		s := r.Series[sig]
		if len(s) == 0 {
			continue
		}
		tail := s.Tail()
		fit := LinearFit(tail)
		budget, verdict := "-", "-"
		if v, ok := r.verdictFor(sig); ok {
			budget = fmtLimit(v.Budget)
			switch {
			case v.Insufficient:
				verdict = "no data"
			case v.Breached && v.Budget.Advisory:
				verdict = "over (advisory)"
			case v.Breached:
				verdict = "OVER"
			default:
				verdict = "ok"
			}
		}
		fmt.Fprintf(&sb, "| `%s` | %.4g | %.4g | %.4g | %.4g | %s | %s | `%s` |\n",
			sig, round3(s[0].V), round3(s[len(s)-1].V), round3(tail.Max()), round3(fit.Growth()), budget, verdict, Sparkline(s, 30))
	}

	sb.WriteString("\n| Check | Result | Detail |\n|---|---|---|\n")
	for _, c := range r.Checks {
		res := "ok"
		switch {
		case c.Skipped:
			res = "skipped"
		case !c.OK:
			res = "FAIL"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", c.Name, res, strings.ReplaceAll(c.Detail, "|", "/"))
	}

	if len(r.Failures) > 0 {
		sb.WriteString("\n**Failures**\n\n")
		for _, f := range r.Failures {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
	}
	return sb.String()
}
