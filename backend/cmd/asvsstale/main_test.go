package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureReport is a minimal but structurally faithful report: a header date
// row, then a §10 changelog with dated rows. It mirrors the real report's shape
// (the header `| **Date** |` row and the `| N.N | DATE | ... |` changelog rows)
// without copying its content.
const fixtureReport = `# ASVS L2 + MASVS-L1 verification report

| | |
|---|---|
| **Pass** | #2 |
| **Date** | 2026-05-01 |
| **Commit verified** | ` + "`deadbeef`" + ` |

## The claim

| 9.9 | 2030-01-01 | this date-shaped row is outside §10 and must be ignored |

## 10. Changelog

| Pass | Date | Commit | Claim | Findings |
|---|---|---|---|---|
| 1 | 2026-05-15 | ` + "`aaaa`" + ` | first | none |
| 1.1 | 2026-06-01 | (see PR) | second | none |

## Appendix

| 8.8 | 2030-02-02 | also ignored |
`

// writeFixture writes body as the report at the path asvsstale resolves,
// creating parent directories, and returns the fixture root.
func writeFixture(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, reportFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture report: %v", err)
	}
	return root
}

func TestNewestAttestation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr string
	}{
		{
			name: "newest is a section 10 row, not the header",
			body: fixtureReport,
			want: "2026-06-01",
		},
		{
			name: "header newer than every section 10 row",
			body: replace(fixtureReport, "| **Date** | 2026-05-01 |", "| **Date** | 2026-07-09 |"),
			want: "2026-07-09",
		},
		{
			name: "date-shaped row outside section 10 is ignored",
			body: fixtureReport,
			want: "2026-06-01",
		},
		{
			name:    "no dated row at all",
			body:    "# A report\n\nno dates here\n",
			wantErr: "no dated header row",
		},
		{
			name:    "malformed header date",
			body:    replace(fixtureReport, "| **Date** | 2026-05-01 |", "| **Date** | 01/06/2026 |"),
			wantErr: "header date",
		},
		{
			name:    "malformed section 10 date",
			body:    replace(fixtureReport, "| 1.1 | 2026-06-01 |", "| 1.1 | not-a-date |"),
			wantErr: "§10 row date",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newestAttestation([]byte(tc.body))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("newestAttestation() = %v, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newestAttestation() error = %v", err)
			}
			if got.Format(dateLayout) != tc.want {
				t.Fatalf("newest = %s, want %s", got.Format(dateLayout), tc.want)
			}
		})
	}
}

// replace is a tiny helper so the table above reads as intent, not boilerplate.
func replace(s, old, new string) string { return strings.Replace(s, old, new, 1) }

func TestRunFresh(t *testing.T) {
	root := writeFixture(t, fixtureReport)
	now := mustParse(t, "2026-06-20")

	var out strings.Builder
	code := run(&out, root, config{reportPath: reportFile, maxAgeDays: 90, now: now})
	if code != 0 {
		t.Fatalf("run() = %d, want 0; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "current") {
		t.Fatalf("output should say current, got:\n%s", out.String())
	}
}

func TestRunStalePointsAtSection8(t *testing.T) {
	root := writeFixture(t, fixtureReport)
	now := mustParse(t, "2026-12-01")

	var out strings.Builder
	code := run(&out, root, config{reportPath: reportFile, maxAgeDays: 90, now: now})
	if code != 1 {
		t.Fatalf("run() = %d, want 1 (stale); output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "stale") || !strings.Contains(out.String(), "§8") {
		t.Fatalf("stale output must name staleness and point at §8, got:\n%s", out.String())
	}
}

func TestRunBoundary(t *testing.T) {
	root := writeFixture(t, fixtureReport)
	// The newest fixture date is 2026-06-01. Exactly maxAgeDays later is not
	// stale; one day more is.
	base := mustParse(t, "2026-06-01")

	tests := []struct {
		now      string
		maxAge   int
		wantCode int
	}{
		{now: base.AddDate(0, 0, 90).Format("2006-01-02"), maxAge: 90, wantCode: 0},
		{now: base.AddDate(0, 0, 91).Format("2006-01-02"), maxAge: 90, wantCode: 1},
		{now: base.Format("2006-01-02"), maxAge: 0, wantCode: 0},
		{now: base.AddDate(0, 0, 1).Format("2006-01-02"), maxAge: 0, wantCode: 1},
	}
	for _, tc := range tests {
		t.Run(tc.now, func(t *testing.T) {
			var out strings.Builder
			code := run(&out, root, config{reportPath: reportFile, maxAgeDays: tc.maxAge, now: mustParse(t, tc.now)})
			if code != tc.wantCode {
				t.Fatalf("run(now=%s, maxAge=%d) = %d, want %d; output:\n%s", tc.now, tc.maxAge, code, tc.wantCode, out.String())
			}
		})
	}
}

func TestRunFutureDateIsError(t *testing.T) {
	root := writeFixture(t, fixtureReport)
	var out strings.Builder
	code := run(&out, root, config{reportPath: reportFile, maxAgeDays: 90, now: mustParse(t, "2026-01-01")})
	if code != 2 {
		t.Fatalf("run() = %d, want 2 for a future-dated report; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "future") {
		t.Fatalf("expected a future-date message, got:\n%s", out.String())
	}
}

func TestRunMissingReportIsError(t *testing.T) {
	var out strings.Builder
	code := run(&out, t.TempDir(), config{reportPath: reportFile, maxAgeDays: 90, now: mustParse(t, "2026-06-20")})
	if code != 2 {
		t.Fatalf("run() = %d, want 2 for a missing report; output:\n%s", code, out.String())
	}
}

func TestRunUndatedReportIsError(t *testing.T) {
	root := writeFixture(t, "# report with no dates\n")
	var out strings.Builder
	code := run(&out, root, config{reportPath: reportFile, maxAgeDays: 90, now: mustParse(t, "2026-06-20")})
	if code != 2 {
		t.Fatalf("run() = %d, want 2 for an undated report; output:\n%s", code, out.String())
	}
}

func TestReadRepoFileRefusesEscapingPaths(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"..", "../secrets", "../../etc/passwd", "/etc/passwd"} {
		if _, err := readRepoFile(root, rel); err == nil {
			t.Errorf("readRepoFile(%q) succeeded, want a traversal refusal", rel)
		}
	}
}

func TestFindRepoRootWalksUp(t *testing.T) {
	root := writeFixture(t, fixtureReport)
	nested := filepath.Join(root, "backend", "cmd", "asvsstale")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	got, err := findRepoRoot(nested)
	if err != nil {
		t.Fatalf("findRepoRoot(%s): %v", nested, err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %s, want %s", got, root)
	}
}

func TestFindRepoRootNotFound(t *testing.T) {
	// A fresh temp dir has no report in it or any ancestor (t.TempDir is under
	// /tmp), so the walk must exhaust itself and fail clearly rather than loop.
	if _, err := findRepoRoot(t.TempDir()); err == nil {
		t.Fatal("findRepoRoot() succeeded, want an error when no report is found above")
	}
}

// TestRealReportParses is the structural guard that the parser still matches
// the committed report. It deliberately does NOT assert freshness — the report
// is allowed to age, that is what run() reports — only that a dated row is
// found, so a formatting change to the header or §10 fails here rather than in
// the scheduled workflow.
func TestRealReportParses(t *testing.T) {
	root, err := findRepoRoot(mustGetwd())
	if err != nil {
		t.Fatal(err)
	}
	body, err := readRepoFile(root, reportFile)
	if err != nil {
		t.Fatalf("reading the real report: %v", err)
	}
	newest, err := newestAttestation(body)
	if err != nil {
		t.Fatalf("newestAttestation(real report): %v", err)
	}
	if newest.Year() < 2020 || newest.After(time.Now().UTC().AddDate(0, 0, 1)) {
		t.Fatalf("real report newest date %s is implausible", newest.Format(dateLayout))
	}
}

func TestMainExitFlags(t *testing.T) {
	future := time.Now().UTC().AddDate(1, 0, 0)
	tests := []struct {
		name string
		args []string
		now  time.Time
		want int
	}{
		{name: "real report within a huge window", args: []string{"-max-age-days", "1000000"}, now: future, want: 0},
		{name: "negative window rejected", args: []string{"-max-age-days", "-1"}, now: future, want: 2},
		{name: "unknown flag rejected", args: []string{"-not-a-flag"}, now: future, want: 2},
		{name: "escaping report path rejected", args: []string{"-report", "../../secret.md"}, now: future, want: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			if got := mainExit(tc.args, &out, tc.now); got != tc.want {
				t.Fatalf("mainExit(%v) = %d, want %d; output:\n%s", tc.args, got, tc.want, out.String())
			}
		})
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return d
}
