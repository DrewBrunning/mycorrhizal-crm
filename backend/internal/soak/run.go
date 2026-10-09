package soak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/services"

	"gorm.io/gorm"
)

// Target is an already-running server to soak instead of booting one in
// process. The DB path is optional and only usable where the file is visible
// (a local run or a bind mount): without it the end-of-run database checks are
// skipped and the report says so.
type Target struct {
	BaseURL      string
	MetricsToken string
	DBPath       string
}

// Config configures one soak run.
type Config struct {
	// Duration is how long the workload runs (excluding setup and the final
	// checks).
	Duration time.Duration
	// SampleEvery is the /metrics sampling cadence.
	SampleEvery time.Duration
	// Users and Rate shape the workload (accounts, aggregate ops per second).
	Users int
	Rate  float64
	Seed  int64
	// MinOps is the fewest operations the workload must complete for the run to
	// count — a harness that silently did nothing must not pass.
	MinOps int64
	// Budgets override the committed table (tests only).
	Budgets []Budget
	// Faults inject deliberate leaks (harness self-test; in-process only).
	Faults []Fault
	// Target soaks an external server; nil boots one in process.
	Target *Target
	// Dir is the working directory for an in-process instance.
	Dir string
	// Progress, when set, receives one human line per sample.
	Progress func(string)
}

// Check is one named end-of-run assertion.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	// Skipped marks a check that could not run in this mode; a skip never
	// fails the run but is always shown.
	Skipped bool `json:"skipped,omitempty"`
}

// Report is the outcome of a run.
type Report struct {
	Mode        string            `json:"mode"`
	Duration    float64           `json:"duration_s"`
	SampleEvery float64           `json:"sample_every_s"`
	Users       int               `json:"users"`
	Rate        float64           `json:"rate_ops_per_s"`
	Faults      []Fault           `json:"faults,omitempty"`
	GoVersion   string            `json:"go_version"`
	Series      map[string]Series `json:"series"`
	Verdicts    []Verdict         `json:"verdicts"`
	Workload    Result            `json:"workload"`
	Checks      []Check           `json:"checks"`
	Failures    []string          `json:"failures"`
}

// OK reports whether the run passed.
func (r *Report) OK() bool { return len(r.Failures) == 0 }

func (r *Report) fail(format string, args ...any) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

func (r *Report) check(c Check) {
	r.Checks = append(r.Checks, c)
	if !c.OK && !c.Skipped {
		r.fail("%s: %s", c.Name, c.Detail)
	}
}

// ErrRun wraps a harness-level failure (could not boot, register, scrape) as
// opposed to a soak finding, which is reported in Report.Failures.
var ErrRun = errors.New("soak run could not complete")

// Run executes one soak: boot (or attach), register users, sample while the
// workload runs, then judge the series and run the end-of-run checks.
func Run(ctx context.Context, cfg Config) (*Report, error) {
	if cfg.Duration <= 0 {
		return nil, fmt.Errorf("%w: duration must be positive", ErrRun)
	}
	if cfg.SampleEvery <= 0 {
		cfg.SampleEvery = 5 * time.Second
	}
	if cfg.Users < 1 {
		cfg.Users = 1
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 20
	}
	budgets := cfg.Budgets
	if budgets == nil {
		budgets = DefaultBudgets()
	}

	rep := &Report{
		Duration: cfg.Duration.Seconds(), SampleEvery: cfg.SampleEvery.Seconds(),
		Users: cfg.Users, Rate: cfg.Rate, Faults: cfg.Faults, GoVersion: runtime.Version(),
		Series: map[string]Series{},
	}

	var (
		inst     *Instance
		baseURL  string
		token    string
		dbPath   string
		inProc   = cfg.Target == nil
		scrapeFn func()
	)
	if inProc {
		rep.Mode = "in-process"
		var err error
		inst, err = StartInstance(ctx, cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRun, err)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			_ = inst.Stop(sctx)
		}()
		baseURL, token, dbPath = inst.BaseURL, inst.MetricsToken, inst.DBPath
		// Heap is judged after a collection, so garbage awaiting the next GC
		// cycle never reads as growth. Not possible against an external
		// process; its heap series is noisier and the RSS budget backstops it.
		scrapeFn = func() { runtime.GC(); debug.FreeOSMemory() }
	} else {
		rep.Mode = "external"
		if len(cfg.Faults) > 0 {
			return nil, fmt.Errorf("%w: faults are in-process only", ErrRun)
		}
		baseURL = strings.TrimRight(cfg.Target.BaseURL, "/")
		token, dbPath = cfg.Target.MetricsToken, cfg.Target.DBPath
		if _, err := url.ParseRequestURI(baseURL); err != nil {
			return nil, fmt.Errorf("%w: bad target URL: %v", ErrRun, err)
		}
	}

	var faultDB *gorm.DB
	if inProc {
		faultDB = inst.DB()
	}
	inj, err := newInjector(ctx, cfg.Faults, faultDB)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRun, err)
	}
	defer inj.Close()

	wl, err := NewWorkload(ctx, WorkloadConfig{BaseURL: baseURL, Users: cfg.Users, Rate: cfg.Rate, Seed: cfg.Seed})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRun, err)
	}

	sc := &Scraper{Client: &http.Client{Timeout: 15 * time.Second}, URL: baseURL + "/metrics", Token: token, prepare: scrapeFn}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		wl.Run(runCtx)
	}()

	start := time.Now()
	base, err := sc.Scrape(ctx)
	if err != nil {
		cancel()
		<-done
		return nil, fmt.Errorf("%w: %v", ErrRun, err)
	}
	record := func(snap Snapshot, p95 map[string]float64) {
		t := time.Since(start).Seconds()
		for name, v := range snap.Values {
			rep.Series[name] = append(rep.Series[name], Point{T: t, V: v})
		}
		for class, v := range p95 {
			sig := p95Signal(class)
			rep.Series[sig] = append(rep.Series[sig], Point{T: t, V: v})
		}
	}
	record(base, nil)
	p95 := NewP95Tracker(base)

	tick := time.NewTicker(cfg.SampleEvery)
	defer tick.Stop()
loop:
	for {
		select {
		case <-done:
			break loop
		case <-tick.C:
			inj.Tick()
			snap, err := sc.Scrape(ctx)
			if err != nil {
				// One failed scrape under load is a finding, not a harness
				// abort: record it and keep sampling.
				rep.fail("scrape failed mid-run: %v", err)
				continue
			}
			record(snap, p95.Update(snap))
			if cfg.Progress != nil {
				cfg.Progress(progressLine(time.Since(start), snap))
			}
		}
	}
	<-done

	// One last scrape after the workload drained: the end-state counters
	// (5xx, job failures) and a final point for the series.
	final, err := sc.Scrape(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: final scrape: %v", ErrRun, err)
	}
	record(final, p95.Update(final))

	rep.Workload = wl.Result()
	rep.judge(cfg, budgets, final)
	rep.endChecks(ctx, inst, dbPath, wl)
	return rep, nil
}

func progressLine(el time.Duration, s Snapshot) string {
	return fmt.Sprintf("t=%5.0fs goroutines=%.0f heap=%.1fMiB rss=%.1fMiB fds=%.0f wal=%.1fMiB limiter=%.0f",
		el.Seconds(), s.Values[SigGoroutines], s.Values[SigHeapInuse]/mib, s.Values[SigRSS]/mib,
		s.Values[SigOpenFDs], s.Values[SigWALBytes]/mib, s.Values[SigLimiterEntries])
}

// judge applies the budgets and the always-on health assertions.
func (r *Report) judge(cfg Config, budgets []Budget, final Snapshot) {
	r.Verdicts = EvaluateAll(budgets, r.Series)
	for _, v := range r.Verdicts {
		if v.Failed() {
			r.fail("budget: %s", v)
		}
	}
	for _, sig := range RequiredSignals {
		if len(r.Series[sig]) == 0 {
			r.fail("signal %s was never exported by /metrics", sig)
		}
	}
	if r.Workload.Total < cfg.MinOps {
		r.fail("workload completed only %d operations (want >= %d) — load generation itself may be broken", r.Workload.Total, cfg.MinOps)
	}
	if r.Workload.ServerErrs > 0 {
		r.fail("%d server errors (5xx / transport) in %d operations; samples: %s",
			r.Workload.ServerErrs, r.Workload.Total, strings.Join(r.Workload.Samples, " | "))
	}
	// These two counters are cumulative and the metrics registry is
	// process-global and never reset, so the absolute value spans every server
	// this process ever booted (a soak test boots several per run, and
	// `-count=2` boots them all again). Assert the DELTA across this run's own
	// window — the documented rule for a process-global counter
	// (docs/development/testing.md, "Order-dependence pass") — or a prior
	// instance's 5xx fails an otherwise healthy run (#1638).
	if n := final.Values[SigServerErrors] - seriesBase(r.Series[SigServerErrors]); n > 0 {
		r.fail("/metrics counts %.0f HTTP 5xx responses", n)
	}
	if n := final.Values[SigJobFailures] - seriesBase(r.Series[SigJobFailures]); n > 0 {
		r.fail("/metrics counts %.0f failed scheduled-job runs", n)
	}
	// 429s are the production limiter doing its job on the auth routes, but a
	// large share means the offered load or the raised API limit is
	// misconfigured and the run measured throttling, not the server.
	if r.Workload.Total > 0 && float64(r.Workload.RateLimited) > 0.05*float64(r.Workload.Total) {
		r.fail("%d/%d operations were rate-limited (429) — the soak is measuring throttling, not the server", r.Workload.RateLimited, r.Workload.Total)
	}
}

// seriesBase is the first (pre-workload) sample of a cumulative series, or 0
// when the signal was absent at baseline. Subtracting it turns the
// process-global registry's absolute counter into a per-run delta.
func seriesBase(s Series) float64 {
	if len(s) == 0 {
		return 0
	}
	return s[0].V
}

// endChecks runs the checks that need the finished server: scheduler health,
// the database's own integrity, derived-data (FTS) drift and the data-integrity
// (doctor) pass.
func (r *Report) endChecks(ctx context.Context, inst *Instance, dbPath string, wl *Workload) {
	if inst != nil {
		r.check(schedulerCheck(inst.Jobs(), time.Now()))
	} else {
		r.check(Check{Name: "scheduler", OK: true, Skipped: true, Detail: "external target: job state is only visible in process"})
	}

	r.check(wl.searchProbe(ctx))

	switch {
	case inst != nil:
		r.dbChecks(ctx, inst.DB(), inst.Cfg)
	case dbPath != "":
		db, err := openExternalDB(dbPath)
		if err != nil {
			r.check(Check{Name: "database", OK: false, Detail: err.Error()})
			return
		}
		defer closeGorm(db)
		r.dbChecks(ctx, db, nil)
	default:
		r.check(Check{Name: "database integrity", OK: true, Skipped: true, Detail: "external target without a visible database file"})
	}
}

// dbChecks runs the integrity, FTS-drift and doctor assertions against db.
// cfg is nil for an external target, where the config-dependent data pass
// cannot run.
func (r *Report) dbChecks(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	storage, err := services.RunStorageIntegrityChecks(db)
	switch {
	case err != nil:
		r.check(Check{Name: "PRAGMA integrity_check / foreign_key_check", Detail: err.Error()})
	case !storage.OK:
		r.check(Check{Name: "PRAGMA integrity_check / foreign_key_check", Detail: "integrity_check=" + storage.IntegrityCheck + " foreign_key_check=" + storage.ForeignKeyCheck})
	default:
		r.check(Check{Name: "PRAGMA integrity_check / foreign_key_check", OK: true, Detail: "ok"})
	}

	r.check(ftsCheck(ctx, db))

	if cfg == nil {
		r.check(Check{Name: "doctor (data integrity)", OK: true, Skipped: true, Detail: "external target: needs the server's configuration"})
		return
	}
	data, err := services.RunDataIntegrityChecks(ctx, db, *cfg)
	switch {
	case err != nil:
		r.check(Check{Name: "doctor (data integrity)", Detail: err.Error()})
	case !data.OK:
		var parts []string
		for _, f := range data.Findings {
			if f.Severity == services.IntegritySeverityViolation {
				parts = append(parts, fmt.Sprintf("%s (%d): %s", f.Check, f.Count, f.Detail))
			}
		}
		r.check(Check{Name: "doctor (data integrity)", Detail: strings.Join(parts, "; ")})
	default:
		r.check(Check{Name: "doctor (data integrity)", OK: true, Detail: "no violations"})
	}
}
