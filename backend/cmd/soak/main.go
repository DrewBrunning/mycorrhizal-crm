// Command soak runs the long-run (soak) test of issue #1496: a sustained mixed
// workload against a real server, sampling process-lifetime signals (memory,
// goroutines, open fds, WAL and database size, rate-limiter entries, latency)
// and failing on *growth*, any 5xx, a failed integrity check, FTS drift or a
// wedged scheduler. See docs/development/soak-testing.md.
//
//	go run ./cmd/soak -duration 45m                  # the nightly form
//	go run ./cmd/soak -duration 60s -sample 2s       # the per-PR smoke form
//	go run ./cmd/soak -target http://host:8080 -metrics-token ... [-db path]
//
// Without -target it boots the real server in this process (embedded.Start,
// the same entry point main() uses) on a loopback port against a fresh
// migrated SQLite file. -fault injects a deliberate leak and exists to prove
// the assertions bite; a run with -fault is expected to exit non-zero.
//
// Exit codes: 0 pass, 1 soak finding (budget breach, 5xx, failed check),
// 2 usage or harness error.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mycorrhizal/internal/soak"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code) // # pragma: no cover — os.Exit terminates; tests call run directly
}

// run is main's testable body; it returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("soak", flag.ContinueOnError)
	fs.SetOutput(stderr)
	duration := fs.Duration("duration", 60*time.Second, "how long the workload runs")
	sample := fs.Duration("sample", 5*time.Second, "/metrics sampling interval")
	users := fs.Int("users", 10, "distinct accounts the load is spread across")
	rate := fs.Float64("rate", 20, "aggregate operations per second")
	seed := fs.Int64("seed", 1, "workload RNG seed")
	minOps := fs.Int64("min-ops", 100, "fail if fewer operations complete")
	faultNames := fs.String("fault", "", "inject deliberate leaks, comma-separated (goroutines|heap|fds|limiter|wal), to prove the assertions bite; in-process only")
	target := fs.String("target", "", "soak an already-running server at this base URL instead of booting one")
	token := fs.String("metrics-token", os.Getenv("METRICS_TOKEN"), "METRICS_TOKEN of the -target server (default $METRICS_TOKEN)")
	dbPath := fs.String("db", "", "with -target: the server's SQLite file, if visible, for the end-of-run integrity/FTS checks")
	jsonOut := fs.String("json", "", "write the full report (every series) as JSON to this file")
	mdOut := fs.String("md", "", "write the markdown report to this file (e.g. $GITHUB_STEP_SUMMARY)")
	advisoryLatency := fs.Bool("advisory-latency", false, "report latency-degradation budgets without failing on them (for runs too short, or too contended, for a head/tail p95 ratio to mean anything)")
	quiet := fs.Bool("quiet", false, "suppress per-sample progress lines")
	budgetsDoc := fs.String("write-budgets-doc", "", "regenerate the budget table in this markdown file (docs/development/soak-baseline.md) and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *budgetsDoc != "" {
		raw, err := os.ReadFile(*budgetsDoc)
		if err != nil {
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
		out, err := soak.ReplaceBudgetBlock(string(raw))
		if err != nil {
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
		if err := os.WriteFile(*budgetsDoc, []byte(out), 0o644); err != nil { // #nosec G306 G703 -- operator-supplied docs path, a developer tool
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
		return 0
	}

	faults, err := soak.ParseFaults(*faultNames)
	if err != nil {
		fmt.Fprintln(stderr, "soak:", err)
		return 2
	}

	cfg := soak.Config{
		Duration: *duration, SampleEvery: *sample, Users: *users, Rate: *rate, Seed: *seed,
		MinOps: *minOps, Faults: faults,
	}
	if *advisoryLatency {
		cfg.Budgets = soak.DefaultBudgets()
		for i := range cfg.Budgets {
			if cfg.Budgets[i].Kind == soak.KindDegradation {
				cfg.Budgets[i].Advisory = true
			}
		}
	}
	if !*quiet {
		cfg.Progress = func(s string) { fmt.Fprintln(stdout, s) }
	}
	if *target != "" {
		if *token == "" {
			fmt.Fprintln(stderr, "soak: -target needs -metrics-token (or $METRICS_TOKEN): the server's METRICS_TOKEN")
			return 2
		}
		cfg.Target = &soak.Target{BaseURL: *target, MetricsToken: *token, DBPath: *dbPath}
	} else {
		dir, err := os.MkdirTemp("", "soak-*")
		if err != nil {
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
		defer func() { _ = os.RemoveAll(dir) }()
		cfg.Dir = dir
	}

	fmt.Fprintf(stdout, "soak: %s at %.0f ops/s over %d users, sampling every %s\n", *duration, *rate, *users, *sample)
	rep, err := soak.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "soak:", err)
		return 2
	}

	fmt.Fprintln(stdout, rep.Markdown())
	if *mdOut != "" {
		if err := appendFile(*mdOut, rep.Markdown()+"\n"); err != nil {
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
	}
	if *jsonOut != "" {
		f, err := os.Create(*jsonOut)
		if err != nil {
			fmt.Fprintln(stderr, "soak:", err)
			return 2
		}
		werr := rep.WriteJSON(f)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			fmt.Fprintln(stderr, "soak: write report:", werr, cerr)
			return 2
		}
	}
	if !rep.OK() {
		fmt.Fprintf(stderr, "soak: FAILED with %d finding(s)\n", len(rep.Failures))
		return 1
	}
	return 0
}

// appendFile appends s to path (creating it), the way $GITHUB_STEP_SUMMARY
// expects: several steps may add to one file.
func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- operator-supplied report path
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	_, werr := f.WriteString(s)
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("write %s: %w", path, werr)
	}
	if cerr != nil {
		return fmt.Errorf("close %s: %w", path, cerr)
	}
	return nil
}
