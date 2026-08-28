package metrics

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, r *Registry) string {
	t.Helper()
	var sb strings.Builder
	require.NoError(t, r.WritePrometheus(&sb))
	return sb.String()
}

func TestCounterVec_IncAndAdd(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("things_total", "Things.", "kind")
	c.With("a").Inc()
	c.With("a").Inc()
	c.With("b").Add(5)

	out := render(t, r)
	assert.Contains(t, out, "# HELP things_total Things.\n")
	assert.Contains(t, out, "# TYPE things_total counter\n")
	assert.Contains(t, out, `things_total{kind="a"} 2`+"\n")
	assert.Contains(t, out, `things_total{kind="b"} 5`+"\n")
}

func TestGaugeVec_SetAddIncDec(t *testing.T) {
	r := NewRegistry()
	g := r.NewGaugeVec("temp", "Temp.")
	g.With().Set(21.5)
	g.With().Inc()
	g.With().Dec()
	g.With().Dec()
	// 21.5 + 1 - 1 - 1 = 20.5
	assert.Contains(t, render(t, r), "temp 20.5\n")

	g.With().Set(-3)
	assert.Contains(t, render(t, r), "temp -3\n")
}

func TestGaugeVec_NoLabelsHasNoBraces(t *testing.T) {
	r := NewRegistry()
	r.NewGaugeVec("bare", "Bare.").With().Set(1)
	assert.Contains(t, render(t, r), "\nbare 1\n")
}

func TestHistogramVec_BucketMath(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogramVec("lat_seconds", "Latency.", []float64{0.1, 0.5, 1}, "route")

	for _, v := range []float64{0.05, 0.2, 0.2, 0.7, 3} {
		h.With("/x").Observe(v)
	}

	out := render(t, r)
	assert.Contains(t, out, "# TYPE lat_seconds histogram\n")
	// cumulative: <=0.1 -> 1 ; <=0.5 -> 3 ; <=1 -> 4 ; +Inf -> 5
	assert.Contains(t, out, `lat_seconds_bucket{route="/x",le="0.1"} 1`+"\n")
	assert.Contains(t, out, `lat_seconds_bucket{route="/x",le="0.5"} 3`+"\n")
	assert.Contains(t, out, `lat_seconds_bucket{route="/x",le="1"} 4`+"\n")
	assert.Contains(t, out, `lat_seconds_bucket{route="/x",le="+Inf"} 5`+"\n")
	assert.Contains(t, out, `lat_seconds_sum{route="/x"} 4.15`+"\n")
	assert.Contains(t, out, `lat_seconds_count{route="/x"} 5`+"\n")
}

func TestHistogramVec_BucketsSortedOnRegister(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogramVec("h_seconds", "H.", []float64{1, 0.1, 0.5})
	h.With().Observe(0.2)

	out := render(t, r)
	i1 := strings.Index(out, `le="0.1"`)
	i2 := strings.Index(out, `le="0.5"`)
	i3 := strings.Index(out, `le="1"`)
	require.True(t, i1 >= 0 && i2 >= 0 && i3 >= 0)
	assert.Less(t, i1, i2)
	assert.Less(t, i2, i3)
}

func TestWritePrometheus_FamiliesAndSeriesSorted(t *testing.T) {
	r := NewRegistry()
	r.NewCounterVec("zzz_total", "Z.").With().Inc()
	c := r.NewCounterVec("aaa_total", "A.", "k")
	c.With("y").Inc()
	c.With("x").Inc()

	out := render(t, r)
	assert.Less(t, strings.Index(out, "aaa_total"), strings.Index(out, "zzz_total"))
	assert.Less(t, strings.Index(out, `aaa_total{k="x"}`), strings.Index(out, `aaa_total{k="y"}`))

	// exactly one HELP and one TYPE line per family
	assert.Equal(t, 1, strings.Count(out, "# HELP aaa_total "))
	assert.Equal(t, 1, strings.Count(out, "# TYPE aaa_total "))
}

func TestWritePrometheus_EscapesLabelValues(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("weird_total", "W.", "path")
	c.With(`a"b\c` + "\n" + "d").Inc()

	out := render(t, r)
	assert.Contains(t, out, `weird_total{path="a\"b\\c\nd"} 1`+"\n")
}

func TestWritePrometheus_EscapesHelpButNotQuotes(t *testing.T) {
	r := NewRegistry()
	r.NewCounterVec("h_total", "line one\nline \\ two").With().Inc()
	assert.Contains(t, render(t, r), `# HELP h_total line one\nline \\ two`+"\n")
}

func TestWith_PanicsOnLabelCountMismatch(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("m_total", "M.", "a", "b")
	assert.PanicsWithValue(t,
		`metrics: family "m_total" expects 2 label value(s), got 1`,
		func() { c.With("only-one").Inc() })
}

func TestRegister_PanicsOnConflictingRedeclaration(t *testing.T) {
	r := NewRegistry()
	r.NewCounterVec("x_total", "X.", "a")
	assert.Panics(t, func() { r.NewGaugeVec("x_total", "X.") })
	assert.Panics(t, func() { r.NewCounterVec("x_total", "X.", "a", "b") })
}

func TestRegister_SameDeclarationReturnsSameFamily(t *testing.T) {
	r := NewRegistry()
	a := r.NewCounterVec("y_total", "Y.", "k")
	b := r.NewCounterVec("y_total", "Y.", "k")
	a.With("v").Inc()
	b.With("v").Inc()
	assert.Contains(t, render(t, r), `y_total{k="v"} 2`+"\n")
}

func TestConcurrentIncrementsAreExact(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("race_total", "R.", "k")
	h := r.NewHistogramVec("race_seconds", "R.", []float64{1, 10}, "k")

	const goroutines, perG = 50, 200
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				c.With("shared").Inc()
				c.With("uniq").Add(2)
				h.With("shared").Observe(0.5)
			}
		}()
	}
	wg.Wait()

	out := render(t, r)
	assert.Contains(t, out, `race_total{k="shared"} 10000`+"\n")
	assert.Contains(t, out, `race_total{k="uniq"} 20000`+"\n")
	assert.Contains(t, out, `race_seconds_count{k="shared"} 10000`+"\n")
	assert.Contains(t, out, `race_seconds_bucket{k="shared",le="1"} 10000`+"\n")
}
