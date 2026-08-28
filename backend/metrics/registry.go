// Package metrics is a tiny hand-rolled Prometheus metrics registry: just the
// three metric shapes this app needs (counter, gauge, histogram), keyed by a
// fixed set of label names, plus a writer that emits the 0.0.4 text exposition
// format. It intentionally does not depend on prometheus/client_golang — the
// project pins its Go toolchain and keeps its dependency tree small, and a
// single-process self-hosted app needs an endpoint and some counters, not a
// full instrumentation SDK (issue #389).
//
// Concurrency: metric values are updated with atomics on the hot path; the
// registry mutex is taken only when a new family or a new label-set series is
// created, and once per scrape to snapshot the family list.
package metrics

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type metricType int

const (
	typeCounter metricType = iota
	typeGauge
	typeHistogram
)

func (t metricType) String() string {
	switch t {
	case typeCounter:
		return "counter"
	case typeGauge:
		return "gauge"
	case typeHistogram:
		return "histogram"
	default:
		return "untyped"
	}
}

// Registry holds a set of metric families and renders them.
type Registry struct {
	mu       sync.Mutex
	families []*family
	byName   map[string]*family
}

// NewRegistry returns an empty registry. Production code uses the package
// Default(); tests use their own so assertions never see cross-test bleed.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]*family)}
}

type family struct {
	name    string
	help    string
	typ     metricType
	labels  []string
	buckets []float64 // histogram only, ascending, no +Inf entry

	mu     sync.Mutex
	series map[string]*seriesData
}

type seriesData struct {
	labelValues []string

	// counter / gauge: a float64 stored as its bit pattern for atomic access.
	bits uint64

	// histogram only.
	bucketCounts []uint64 // cumulative count of observations <= buckets[i]
	sum          uint64   // float64 bits
	count        uint64
}

func (r *Registry) register(name, help string, typ metricType, labels []string, buckets []float64) *family {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byName[name]; ok {
		if existing.typ != typ || !equalStrings(existing.labels, labels) {
			panic(fmt.Sprintf("metrics: family %q re-registered with a different type or label set", name))
		}
		return existing
	}

	b := append([]float64(nil), buckets...)
	sort.Float64s(b)
	f := &family{
		name:    name,
		help:    help,
		typ:     typ,
		labels:  append([]string(nil), labels...),
		buckets: b,
		series:  make(map[string]*seriesData),
	}
	r.byName[name] = f
	r.families = append(r.families, f)
	return f
}

func (f *family) get(labelValues []string) *seriesData {
	if len(labelValues) != len(f.labels) {
		panic(fmt.Sprintf("metrics: family %q expects %d label value(s), got %d", f.name, len(f.labels), len(labelValues)))
	}
	key := strings.Join(labelValues, "\x00")

	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.series[key]; ok {
		return s
	}
	s := &seriesData{labelValues: append([]string(nil), labelValues...)}
	if f.typ == typeHistogram {
		s.bucketCounts = make([]uint64, len(f.buckets))
	}
	f.series[key] = s
	return s
}

// --- typed vectors -------------------------------------------------------------

// CounterVec is a family of monotonically increasing counters.
type CounterVec struct{ f *family }

// GaugeVec is a family of gauges (values that can go up or down).
type GaugeVec struct{ f *family }

// HistogramVec is a family of fixed-bucket histograms.
type HistogramVec struct{ f *family }

// NewCounterVec registers (or returns the already-registered) counter family.
func (r *Registry) NewCounterVec(name, help string, labels ...string) *CounterVec {
	return &CounterVec{r.register(name, help, typeCounter, labels, nil)}
}

// NewGaugeVec registers (or returns the already-registered) gauge family.
func (r *Registry) NewGaugeVec(name, help string, labels ...string) *GaugeVec {
	return &GaugeVec{r.register(name, help, typeGauge, labels, nil)}
}

// NewHistogramVec registers (or returns the already-registered) histogram
// family. buckets are upper bounds in seconds/whatever unit; +Inf is implicit.
func (r *Registry) NewHistogramVec(name, help string, buckets []float64, labels ...string) *HistogramVec {
	return &HistogramVec{r.register(name, help, typeHistogram, labels, buckets)}
}

// Counter is a handle to one counter series.
type Counter struct{ s *seriesData }

// Gauge is a handle to one gauge series.
type Gauge struct{ s *seriesData }

// Histogram is a handle to one histogram series.
type Histogram struct {
	s *seriesData
	f *family
}

// With binds the vector to a concrete label-value tuple (positional, matching
// the label names given at registration).
func (v *CounterVec) With(labelValues ...string) Counter {
	return Counter{v.f.get(labelValues)}
}

// With binds the vector to a concrete label-value tuple.
func (v *GaugeVec) With(labelValues ...string) Gauge {
	return Gauge{v.f.get(labelValues)}
}

// With binds the vector to a concrete label-value tuple.
func (v *HistogramVec) With(labelValues ...string) Histogram {
	return Histogram{v.f.get(labelValues), v.f}
}

// Inc adds 1.
func (c Counter) Inc() { addFloat(&c.s.bits, 1) }

// Add adds delta (must be >= 0 for a counter; negative is a caller bug and is
// not guarded here, matching client_golang's own posture in release builds).
func (c Counter) Add(delta float64) { addFloat(&c.s.bits, delta) }

// Set replaces the gauge value.
func (g Gauge) Set(v float64) { atomic.StoreUint64(&g.s.bits, math.Float64bits(v)) }

// Add adds delta (may be negative).
func (g Gauge) Add(delta float64) { addFloat(&g.s.bits, delta) }

// Inc adds 1.
func (g Gauge) Inc() { g.Add(1) }

// Dec subtracts 1.
func (g Gauge) Dec() { g.Add(-1) }

// Observe records one sample.
func (h Histogram) Observe(v float64) {
	addFloat(&h.s.sum, v)
	atomic.AddUint64(&h.s.count, 1)
	for i, ub := range h.f.buckets {
		if v <= ub {
			atomic.AddUint64(&h.s.bucketCounts[i], 1)
		}
	}
}

func addFloat(bits *uint64, delta float64) {
	for {
		old := atomic.LoadUint64(bits)
		nw := math.Float64bits(math.Float64frombits(old) + delta)
		if atomic.CompareAndSwapUint64(bits, old, nw) {
			return
		}
	}
}

// --- rendering ---------------------------------------------------------------

// WritePrometheus renders every family in the 0.0.4 text exposition format,
// families sorted by name and series sorted by label values so the output is
// byte-stable for a given set of values.
func (r *Registry) WritePrometheus(w io.Writer) error {
	r.mu.Lock()
	fams := make([]*family, len(r.families))
	copy(fams, r.families)
	r.mu.Unlock()
	sort.Slice(fams, func(i, j int) bool { return fams[i].name < fams[j].name })

	bw := bufio.NewWriter(w)
	for _, f := range fams {
		f.writeTo(bw)
	}
	return bw.Flush()
}

func (f *family) writeTo(w *bufio.Writer) {
	f.mu.Lock()
	series := make([]*seriesData, 0, len(f.series))
	for _, s := range f.series {
		series = append(series, s)
	}
	f.mu.Unlock()
	sort.Slice(series, func(i, j int) bool {
		return strings.Join(series[i].labelValues, "\x00") < strings.Join(series[j].labelValues, "\x00")
	})

	if f.help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", f.name, escapeHelp(f.help))
	}
	fmt.Fprintf(w, "# TYPE %s %s\n", f.name, f.typ)

	for _, s := range series {
		switch f.typ {
		case typeCounter, typeGauge:
			w.WriteString(f.name)
			writeLabels(w, f.labels, s.labelValues, "")
			w.WriteByte(' ')
			w.WriteString(formatFloat(loadFloat(&s.bits)))
			w.WriteByte('\n')
		case typeHistogram:
			total := atomic.LoadUint64(&s.count)
			for i, ub := range f.buckets {
				w.WriteString(f.name)
				w.WriteString("_bucket")
				writeLabels(w, f.labels, s.labelValues, formatFloat(ub))
				w.WriteByte(' ')
				w.WriteString(strconv.FormatUint(atomic.LoadUint64(&s.bucketCounts[i]), 10))
				w.WriteByte('\n')
			}
			w.WriteString(f.name)
			w.WriteString("_bucket")
			writeLabels(w, f.labels, s.labelValues, "+Inf")
			w.WriteByte(' ')
			w.WriteString(strconv.FormatUint(total, 10))
			w.WriteByte('\n')

			w.WriteString(f.name)
			w.WriteString("_sum")
			writeLabels(w, f.labels, s.labelValues, "")
			w.WriteByte(' ')
			w.WriteString(formatFloat(loadFloat(&s.sum)))
			w.WriteByte('\n')

			w.WriteString(f.name)
			w.WriteString("_count")
			writeLabels(w, f.labels, s.labelValues, "")
			w.WriteByte(' ')
			w.WriteString(strconv.FormatUint(total, 10))
			w.WriteByte('\n')
		}
	}
}

// writeLabels emits `{a="1",b="2"}` (plus an optional le= for histogram
// buckets). Nothing is written when there are no labels and no le.
func writeLabels(w *bufio.Writer, names, values []string, le string) {
	if len(names) == 0 && le == "" {
		return
	}
	w.WriteByte('{')
	first := true
	for i, n := range names {
		if !first {
			w.WriteByte(',')
		}
		first = false
		w.WriteString(n)
		w.WriteString(`="`)
		w.WriteString(escapeLabelValue(values[i]))
		w.WriteByte('"')
	}
	if le != "" {
		if !first {
			w.WriteByte(',')
		}
		w.WriteString(`le="`)
		w.WriteString(le) // le is always a number or +Inf — no escaping needed
		w.WriteByte('"')
	}
	w.WriteByte('}')
}

func loadFloat(bits *uint64) float64 { return math.Float64frombits(atomic.LoadUint64(bits)) }

func formatFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}

func escapeLabelValue(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	r := strings.NewReplacer("\\", `\\`, "\"", `\"`, "\n", `\n`)
	return r.Replace(s)
}

func escapeHelp(s string) string {
	if !strings.ContainsAny(s, "\\\n") {
		return s
	}
	r := strings.NewReplacer("\\", `\\`, "\n", `\n`)
	return r.Replace(s)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
