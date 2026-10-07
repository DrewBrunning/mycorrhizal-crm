package soak

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Signal names: the keys of Snapshot.Values and of Report.Series. Budgets
// refer to these.
const (
	SigRSS            = "rss_bytes"
	SigHeapInuse      = "heap_inuse_bytes"
	SigGoroutines     = "goroutines"
	SigOpenFDs        = "open_fds"
	SigWALBytes       = "wal_bytes"
	SigDBBytes        = "db_bytes"
	SigLimiterEntries = "ratelimiter_entries"
	SigDBConnsOpen    = "db_connections_open"
	SigInFlight       = "http_in_flight"
	SigServerErrors   = "http_5xx_total"
	SigJobFailures    = "job_failures_total"
	SigJobSuccesses   = "job_successes_total"
	SigP95Read        = "p95_read_s"
	SigP95Write       = "p95_write_s"
	SigP95Auth        = "p95_auth_s"
)

// Metric is one parsed exposition sample.
type Metric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// ParseExposition parses Prometheus text format 0.0.4 (the subset the
// project's own hand-rolled registry emits: counter/gauge/histogram samples,
// HELP/TYPE comments skipped). A malformed sample line is an error — a soak
// that silently dropped half the exposition would read as "no growth".
func ParseExposition(r io.Reader) ([]Metric, error) {
	var out []Metric
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m, err := parseSampleLine(line)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read exposition: %w", err)
	}
	return out, nil
}

func parseSampleLine(line string) (Metric, error) {
	m := Metric{}
	var rest string
	if i := strings.IndexByte(line, '{'); i >= 0 {
		j := strings.LastIndexByte(line, '}')
		if j < i {
			return m, fmt.Errorf("malformed sample (unbalanced braces): %q", line)
		}
		m.Name = line[:i]
		labels, err := parseLabels(line[i+1 : j])
		if err != nil {
			return m, fmt.Errorf("malformed labels in %q: %w", line, err)
		}
		m.Labels = labels
		rest = strings.TrimSpace(line[j+1:])
	} else {
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			return m, fmt.Errorf("malformed sample (no value): %q", line)
		}
		m.Name = line[:sp]
		rest = strings.TrimSpace(line[sp+1:])
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return m, fmt.Errorf("malformed sample (no value): %q", line)
	}
	v, err := parseValue(fields[0])
	if err != nil {
		return m, fmt.Errorf("malformed value in %q: %w", line, err)
	}
	m.Value = v
	return m, nil
}

func parseValue(s string) (float64, error) {
	switch s {
	case "+Inf":
		return math.Inf(1), nil
	case "-Inf":
		return math.Inf(-1), nil
	}
	return strconv.ParseFloat(s, 64)
}

// parseLabels parses `a="x",b="y\"z"` honouring the exposition escapes
// (\\ \" \n).
func parseLabels(s string) (map[string]string, error) {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			return nil, fmt.Errorf("label without '=' near %q", s[i:])
		}
		name := strings.TrimSpace(s[i : i+eq])
		i += eq + 1
		if i >= len(s) || s[i] != '"' {
			return nil, fmt.Errorf("label %q value is not quoted", name)
		}
		i++
		var sb strings.Builder
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				switch s[i+1] {
				case 'n':
					sb.WriteByte('\n')
				default:
					sb.WriteByte(s[i+1])
				}
				i += 2
				continue
			}
			if c == '"' {
				closed = true
				i++
				break
			}
			sb.WriteByte(c)
			i++
		}
		if !closed {
			return nil, fmt.Errorf("label %q value is unterminated", name)
		}
		out[name] = sb.String()
		if i < len(s) && s[i] == ',' {
			i++
		}
	}
	return out, nil
}

// Snapshot is one scrape reduced to the soak signals. Values holds the
// gauges/counters directly; hist holds the cumulative latency histogram per
// route class, kept so successive snapshots can be differenced into an
// interval p95.
type Snapshot struct {
	Values map[string]float64
	hist   map[string]histogram
}

// histogram is a cumulative bucket set: upper bound -> cumulative count.
type histogram map[float64]float64

// routeClass buckets a route template into a latency class. Auth routes are
// bcrypt-bound and would swamp the read p95 if mixed in; writes serialise on
// the single SQLite writer; reads are lock-free under WAL.
func routeClass(method, route string) string {
	switch {
	case strings.Contains(route, "/login"), strings.Contains(route, "/logout"), strings.Contains(route, "/register"):
		return "auth"
	case method == http.MethodGet, method == http.MethodHead:
		return "read"
	default:
		return "write"
	}
}

// Reduce turns parsed exposition samples into a Snapshot.
func Reduce(ms []Metric) Snapshot {
	snap := Snapshot{Values: map[string]float64{}, hist: map[string]histogram{}}
	for _, m := range ms {
		switch m.Name {
		case "process_resident_memory_bytes":
			snap.Values[SigRSS] = m.Value
		case "go_memstats_heap_inuse_bytes":
			snap.Values[SigHeapInuse] = m.Value
		case "go_goroutines":
			snap.Values[SigGoroutines] = m.Value
		case "process_open_fds":
			snap.Values[SigOpenFDs] = m.Value
		case "db_connections_open":
			snap.Values[SigDBConnsOpen] = m.Value
		case "http_requests_in_flight":
			snap.Values[SigInFlight] = m.Value
		case "mycorrhizal_storage_bytes":
			switch m.Labels["kind"] {
			case "wal":
				snap.Values[SigWALBytes] = m.Value
			case "database":
				snap.Values[SigDBBytes] = m.Value
			}
		case "mycorrhizal_ratelimiter_entries":
			snap.Values[SigLimiterEntries] += m.Value
		case "http_requests_total":
			if strings.HasPrefix(m.Labels["status"], "5") {
				snap.Values[SigServerErrors] += m.Value
			}
		case "job_runs_total":
			switch m.Labels["result"] {
			case "failure":
				snap.Values[SigJobFailures] += m.Value
			case "success":
				snap.Values[SigJobSuccesses] += m.Value
			}
		case "http_request_duration_seconds_bucket":
			le, err := parseValue(m.Labels["le"])
			if err != nil {
				continue
			}
			class := routeClass(m.Labels["method"], m.Labels["route"])
			h := snap.hist[class]
			if h == nil {
				h = histogram{}
				snap.hist[class] = h
			}
			h[le] += m.Value
		}
	}
	return snap
}

// minIntervalRequests is how many requests a class must have served since its
// anchor for a p95 to be recorded; a handful of requests make a p95 noise.
const minIntervalRequests = 8

// P95Tracker turns successive snapshots into a p95 latency series per route
// class. Each class keeps its own anchor snapshot and only advances it when
// enough requests have accumulated to emit a point — so a rare class (login
// is a few requests per sampling interval) still yields a series, measured
// over a longer window, instead of being starved by a per-interval minimum.
type P95Tracker struct {
	anchor map[string]histogram
}

// NewP95Tracker starts tracking from the baseline snapshot.
func NewP95Tracker(base Snapshot) *P95Tracker {
	return &P95Tracker{anchor: copyHists(base.hist)}
}

func copyHists(in map[string]histogram) map[string]histogram {
	out := make(map[string]histogram, len(in))
	for class, h := range in {
		c := make(histogram, len(h))
		for le, v := range h {
			c[le] = v
		}
		out[class] = c
	}
	return out
}

// Update returns the p95 (seconds) for every class that has served at least
// minIntervalRequests since its anchor, and re-anchors those classes at cur.
func (t *P95Tracker) Update(cur Snapshot) map[string]float64 {
	out := map[string]float64{}
	for class, ch := range cur.hist {
		if p, ok := histP95(t.anchor[class], ch); ok {
			out[class] = p
			c := make(histogram, len(ch))
			for le, v := range ch {
				c[le] = v
			}
			t.anchor[class] = c
		}
	}
	return out
}

// histP95 differences two cumulative histograms and returns the upper bound of
// the bucket the 95th percentile of the difference falls in — histogram
// resolution, not an interpolation, which is all a growth/degradation
// comparison needs. ok is false with too few requests in the difference.
func histP95(prev, cur histogram) (float64, bool) {
	bounds := make([]float64, 0, len(cur))
	for le := range cur {
		bounds = append(bounds, le)
	}
	if len(bounds) == 0 {
		return 0, false
	}
	sort.Float64s(bounds)
	top := bounds[len(bounds)-1]
	total := cur[top] - prev[top]
	if total < minIntervalRequests {
		return 0, false
	}
	target := total * 0.95
	for i, le := range bounds {
		if cur[le]-prev[le] < target {
			continue
		}
		if math.IsInf(le, 1) {
			// Past the largest finite bucket: report that bucket's bound so
			// the series stays finite and still moves.
			if i == 0 {
				return 0, true
			}
			return bounds[i-1], true
		}
		return le, true
	}
	return top, true // # pragma: no cover — the top bucket always holds the whole total
}

// p95Signal maps a route class to its latency signal name.
func p95Signal(class string) string {
	switch class {
	case "auth":
		return SigP95Auth
	case "write":
		return SigP95Write
	default:
		return SigP95Read
	}
}

// Scraper fetches /metrics from a server.
type Scraper struct {
	Client  *http.Client
	URL     string
	Token   string
	prepare func() // optional hook run before each scrape (in-process GC)
}

// Scrape performs one authenticated /metrics fetch and reduces it.
func (s *Scraper) Scrape(ctx context.Context) (Snapshot, error) {
	if s.prepare != nil {
		s.prepare()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build scrape request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := s.Client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("scrape %s: %w", s.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("scrape %s: status %d (is METRICS_TOKEN set and correct?)", s.URL, resp.StatusCode)
	}
	ms, err := ParseExposition(resp.Body)
	if err != nil {
		return Snapshot{}, err
	}
	return Reduce(ms), nil
}
