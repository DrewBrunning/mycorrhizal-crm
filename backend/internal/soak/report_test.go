package soak

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSparkline(t *testing.T) {
	assert.Empty(t, Sparkline(nil, 10))
	assert.Empty(t, Sparkline(line(5, 0, 1), 0))

	flat := Sparkline(line(8, 7, 0), 8)
	assert.Equal(t, "▁▁▁▁▁▁▁▁", flat, "a flat series renders flat")

	rising := []rune(Sparkline(line(8, 0, 1), 8))
	assert.Equal(t, '▁', rising[0])
	assert.Equal(t, '█', rising[len(rising)-1])

	// Longer than the width: resampled down to exactly width cells.
	assert.Len(t, []rune(Sparkline(line(500, 0, 1), 30)), 30)
}

func sampleReport() *Report {
	return &Report{
		Mode: "in-process", Duration: 60, SampleEvery: 2, Users: 10, Rate: 20, GoVersion: "go-test",
		Series: map[string]Series{
			SigGoroutines: line(30, 12, 0),
			SigRSS:        line(30, 5e7, 1e4),
			"custom_sig":  line(30, 1, 0),
		},
		Verdicts: []Verdict{
			Evaluate(Budget{Signal: SigGoroutines, Kind: KindGrowth, Limit: 12, Unit: "goroutines", Reason: "r"}, line(30, 12, 0)),
			Evaluate(Budget{Signal: SigRSS, Kind: KindGrowth, Limit: 1, Unit: "bytes", Reason: "r"}, line(30, 5e7, 1e4)),
			Evaluate(Budget{Signal: "adv", Kind: KindGrowth, Limit: 1, Unit: "u", Reason: "r", Advisory: true}, line(30, 0, 9)),
			Evaluate(Budget{Signal: "nodata", Kind: KindGrowth, Limit: 1, Unit: "u", Reason: "r"}, nil),
		},
		Workload: Result{Total: 1200, Ops: map[string]int64{"x": 1}},
		Checks: []Check{
			{Name: "ok-check", OK: true, Detail: "fine"},
			{Name: "skipped-check", OK: true, Skipped: true, Detail: "n/a"},
			{Name: "bad-check", Detail: "has | pipe"},
		},
	}
}

func TestReport_MarkdownPassAndFail(t *testing.T) {
	r := sampleReport()
	r.Series["adv"] = line(30, 0, 9)
	md := r.Markdown()
	assert.Contains(t, md, "### Soak run: PASS")
	assert.Contains(t, md, "`goroutines`")
	assert.Contains(t, md, "`custom_sig`", "signals without a budget still appear")
	assert.Contains(t, md, "over (advisory)")
	assert.Contains(t, md, "| bad-check | FAIL | has / pipe |", "pipes in details must not break the table")
	assert.Contains(t, md, "skipped")

	r.fail("budget: %s", "boom")
	r.Faults = []Fault{FaultHeap}
	md = r.Markdown()
	assert.Contains(t, md, "### Soak run: FAIL")
	assert.Contains(t, md, "**Failures**")
	assert.Contains(t, md, "injected faults")
	assert.False(t, r.OK())
}

func TestReport_VerdictLabels(t *testing.T) {
	r := sampleReport()
	r.Series["nodata"] = line(2, 0, 0)
	md := r.Markdown()
	assert.Contains(t, md, "no data")
	assert.Contains(t, md, "OVER")
	_, ok := r.verdictFor("nope")
	assert.False(t, ok)
}

func TestReport_WriteJSONRoundTrips(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	require.NoError(t, r.WriteJSON(&buf))
	var back Report
	require.NoError(t, json.Unmarshal(buf.Bytes(), &back))
	assert.Equal(t, r.Mode, back.Mode)
	assert.Len(t, back.Series[SigGoroutines], 30)
	assert.True(t, strings.Contains(buf.String(), `"series"`))
}
