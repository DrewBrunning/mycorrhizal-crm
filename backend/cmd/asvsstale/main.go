// Command asvsstale is the calendar backstop for the ASVS/MASVS attestation
// (issue #949).
//
// The dated security claim in docs/security/asvs-l2-verification-report.md is
// re-verified on release, enforced mechanically by release.yml (issue #608):
// the tag cannot be pushed unless §10 carries a new row since the previous
// release tag. That forcing function has one blind spot the report's own §9
// names — if the release cadence stalls, the published claim goes stale with no
// warning at all. This command closes it:
//
//  1. Parse the report's header `| **Date** | YYYY-MM-DD |` row and every dated
//     row of its §10 changelog.
//  2. Take the newest of those dates as "when the claim was last attested".
//  3. Exit 1 with a §8 pointer when that date is older than the window (90 days
//     by default), so a scheduled job can alarm before the claim silently rots.
//
// It is deliberately a *staleness reminder*, not a re-pass: §9 rejects a full
// calendar audit, and the adequacy of a re-verification stays a human
// obligation (§8). This only answers "has anyone touched this recently".
//
// Exit status 0 means the attestation is within the window; 1 means it is
// stale; 2 means the check itself could not run (report missing, no dated row).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// reportFile is the dated attestation this command reads, repo-root-relative.
const reportFile = "docs/security/asvs-l2-verification-report.md"

// dateLayout is the report's date format (both the header row and §10 rows).
const dateLayout = "2006-01-02"

// defaultMaxAgeDays is the staleness window. 90 days matches the project's one
// "give people time / notice staleness" number (cmd/deprecations,
// cmd/depexceptions, dependency-upgrade-policy.md) rather than inventing a
// second cadence constant.
const defaultMaxAgeDays = 90

// section10Prefix marks the changelog heading. The report's changelog is the
// last section, so once this heading is seen everything after is its table.
const section10Prefix = "## 10."

// headerDateRE captures the second cell of the report header's
// `| **Date** | 2026-09-18 |` row. The cell is captured raw (not as a date
// pattern) so a malformed date is reported rather than silently skipped.
var headerDateRE = regexp.MustCompile(`^\|\s*\*\*Date\*\*\s*\|\s*([^|]*?)\s*\|`)

// changelogRowRE captures the second cell of a §10 data row — a row whose
// first cell is the pass number, e.g. `| 1.29 | 2026-09-17 | \`438bf17c\` | … |`.
// Captured raw for the same reason as headerDateRE.
var changelogRowRE = regexp.MustCompile(`^\|\s*\d+(?:\.\d+)?\s*\|\s*([^|]*?)\s*\|`)

// config is the parsed command configuration.
type config struct {
	reportPath string
	maxAgeDays int
	now        time.Time
}

func main() {
	os.Exit(mainExit(os.Args[1:], os.Stdout, time.Now().UTC())) // # pragma: no cover — os.Exit terminates the process; tests exercise run()
}

// mainExit parses flags and delegates to run. It is separated from main so the
// flag surface can be exercised without spawning a process.
func mainExit(args []string, w io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("asvsstale", flag.ContinueOnError)
	fs.SetOutput(w)
	report := fs.String("report", reportFile, "report to check, repo-root-relative")
	maxAge := fs.Int("max-age-days", defaultMaxAgeDays, "warn when the newest attestation date is older than this many days")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *maxAge < 0 {
		fmt.Fprintln(w, "asvsstale: -max-age-days must not be negative")
		return 2
	}
	root, err := findRepoRoot(mustGetwd())
	if err != nil {
		fmt.Fprintln(w, "asvsstale:", err)
		return 2
	}
	return run(w, root, config{reportPath: *report, maxAgeDays: *maxAge, now: now})
}

// run decides the exit status against a known repository root. Tests drive it
// directly with a fixture tree.
func run(w io.Writer, root string, cfg config) int {
	body, err := readRepoFile(root, cfg.reportPath)
	if err != nil {
		fmt.Fprintf(w, "asvsstale: %v\n", err)
		return 2
	}
	newest, err := newestAttestation(body)
	if err != nil {
		fmt.Fprintf(w, "asvsstale: %s: %v\n", cfg.reportPath, err)
		return 2
	}
	if newest.After(cfg.now) {
		fmt.Fprintf(w, "asvsstale: %s newest attestation date %s is in the future relative to %s\n",
			cfg.reportPath, newest.Format(dateLayout), cfg.now.Format(dateLayout))
		return 2
	}

	age := int(cfg.now.Sub(newest).Hours() / 24)
	if age > cfg.maxAgeDays {
		fmt.Fprintf(w, "ASVS/MASVS attestation is stale: newest verification date %s is %d days old (> %d-day window).\n",
			newest.Format(dateLayout), age, cfg.maxAgeDays)
		fmt.Fprintln(w, "Re-verify per docs/security/asvs-l2-verification-report.md §8 and add a §10 changelog row.")
		return 1
	}
	fmt.Fprintf(w, "ASVS/MASVS attestation is current: newest verification date %s (%d days old, window %d).\n",
		newest.Format(dateLayout), age, cfg.maxAgeDays)
	return 0
}

// newestAttestation returns the newest date across the report's header date row
// and every dated §10 changelog row. Using the newest of both means a milestone
// gate that adds a §10 row without touching the header still counts as
// attested — the header is only regenerated on a full pass (§8 step 8).
func newestAttestation(report []byte) (time.Time, error) {
	var newest time.Time
	sawDate := false
	inSection10 := false

	for _, line := range strings.Split(string(report), "\n") {
		if !inSection10 {
			if m := headerDateRE.FindStringSubmatch(line); m != nil {
				d, err := parseCell("header", m[1])
				if err != nil {
					return time.Time{}, err
				}
				sawDate = true
				if d.After(newest) {
					newest = d
				}
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inSection10 = strings.HasPrefix(strings.TrimSpace(line), section10Prefix)
			continue
		}
		if !inSection10 {
			continue
		}
		if m := changelogRowRE.FindStringSubmatch(line); m != nil {
			d, err := parseCell("§10 row", m[1])
			if err != nil {
				return time.Time{}, err
			}
			sawDate = true
			if d.After(newest) {
				newest = d
			}
		}
	}
	if !sawDate || newest.IsZero() {
		return time.Time{}, fmt.Errorf("no dated header row or §10 changelog row found")
	}
	return newest, nil
}

// parseCell parses a captured date cell, naming the source in an error. It is
// deliberately strict: a malformed date must fail loudly, not be skipped, or a
// typo in the newest row would silently fall back to an older date.
func parseCell(where, raw string) (time.Time, error) {
	cell := strings.TrimSpace(raw)
	d, err := time.Parse(dateLayout, cell)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s date %q is not %s: %w", where, cell, dateLayout, err)
	}
	return d, nil
}

// findRepoRoot walks up from start until it finds the report the command
// exists to verify, so it works from backend/ (go run) and from
// backend/cmd/asvsstale/ (go test) alike.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, reportFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for %s)", start, reportFile)
		}
		dir = parent
	}
}

// mustGetwd returns the working directory, falling back to "." so a resolution
// failure surfaces as the normal "no repository root" error rather than a
// panic.
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "."
	}
	return wd
}

// readRepoFile reads a repository-relative file, refusing anything that would
// escape root. Every read here comes from the fixed reportFile default or an
// operator-supplied `-report`, so the traversal check keeps that flag from
// becoming an arbitrary-file reader.
func readRepoFile(root, rel string) ([]byte, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("refusing to read %q — outside the repository root", rel)
	}
	// #nosec G304 -- clean is repo-relative and traversal-checked immediately
	// above; root comes from findRepoRoot walking up from the working
	// directory, never from request input. Same posture as cmd/citecheck's
	// readRepoFile.
	return os.ReadFile(filepath.Join(root, clean))
}
