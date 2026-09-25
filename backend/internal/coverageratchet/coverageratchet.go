// Package coverageratchet implements the backend half of the per-file
// no-regression coverage ratchet (frontend counterpart:
// frontend/scripts/check-coverage-ratchet.mjs).
//
// The only coverage gate CI has today is Codecov's diff-based patch status
// (codecov.yml, docs/development/coverage.md): codecov/patch/backend judges
// only the lines a PR actually changed. A file that already sits at 0%
// coverage stays there forever -- nothing ever re-measures a file a PR
// doesn't touch -- and a PR that deletes or guts a test for a file it
// doesn't otherwise edit trips no status at all.
//
// This package reads the same merged coverprofile the `backend` job in
// .github/workflows/unit-tests.yml already produces (backend/coverage.out,
// the sum of all seven backend-tests legs), computes each file's covered
// statement percentage, and compares it against a committed baseline
// (testdata/baseline.json). A file whose percentage drops by more than the
// baseline's tolerance fails; an improved, new, or removed/renamed file
// does not -- this is a ratchet against regression, not an absolute floor
// (codecov.yml's project-wide number stays deliberately ungated, and this
// package does not change that).
//
// cmd/coverageratchet is the thin CLI wrapper: `-check` (default) compares
// against the committed baseline, `-update` regenerates it.
package coverageratchet

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Block is one line-range entry from a Go coverprofile: a statement block
// covering [StartLine,EndLine] with NumStmt statements, executed Count
// times (0 means never).
type Block struct {
	File      string // module-relative, e.g. "mycorrhizal/internal/foo/bar.go"
	StartLine int
	EndLine   int
	NumStmt   int
	Count     int64
}

var blockLineRE = regexp.MustCompile(`^(.+):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$`)

// ParseCoverprofile parses a Go coverprofile (`go test -coverprofile`,
// atomic/count/set mode -- the mode line is validated but otherwise
// unused). It does not merge duplicate blocks; callers that read an
// already-merged profile (as backend/coverage.out already is, per the
// `backend` job's "Merge coverage profiles" step) get one entry per block.
func ParseCoverprofile(r io.Reader) ([]Block, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !scanner.Scan() {
		return nil, fmt.Errorf("coverprofile is empty")
	}
	if !strings.HasPrefix(scanner.Text(), "mode:") {
		return nil, fmt.Errorf("coverprofile does not start with a mode: line, got %q", scanner.Text())
	}

	var blocks []Block
	lineNo := 1
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := blockLineRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("coverprofile line %d does not match the expected format: %q", lineNo, line)
		}
		startLine, err := strconv.Atoi(m[2])
		if err != nil { // # pragma: no cover — the regex already constrains this to digits
			return nil, err
		}
		endLine, err := strconv.Atoi(m[3])
		if err != nil { // # pragma: no cover — the regex already constrains this to digits
			return nil, err
		}
		numStmt, err := strconv.Atoi(m[4])
		if err != nil { // # pragma: no cover — the regex already constrains this to digits
			return nil, err
		}
		count, err := strconv.ParseInt(m[5], 10, 64)
		if err != nil { // # pragma: no cover — the regex already constrains this to digits
			return nil, err
		}
		blocks = append(blocks, Block{
			File:      m[1],
			StartLine: startLine,
			EndLine:   endLine,
			NumStmt:   numStmt,
			Count:     count,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading coverprofile: %w", err)
	}
	return blocks, nil
}

// FileStat is one file's statement coverage: how many of its NumStmt-weighted
// statements are covered (Count > 0 in at least the surviving, non-excluded
// blocks) versus the total.
type FileStat struct {
	Statements int
	Covered    int
}

// Percent returns the covered percentage, 100 for a file with zero
// statements (nothing to fail on).
func (f FileStat) Percent() float64 {
	if f.Statements == 0 {
		return 100
	}
	return 100 * float64(f.Covered) / float64(f.Statements)
}

// pragmaMarker is the same marker convention used throughout this repo (see
// CLAUDE.md's Override path and cmd/codecovcheck) for a line Codecov should
// not count against coverage. This package approximates that at the
// coverprofile's own block granularity, which is coarser than a single
// line: a coverprofile block spans StartLine..EndLine as one covered/
// uncovered unit, so a pragma comment anywhere in that range excludes the
// whole block. Codecov itself works line-by-line; this is deliberately a
// close approximation, not a byte-for-byte reimplementation of Codecov's
// annotation processor, done so the ratchet's numbers don't systematically
// diverge from what codecov/patch/backend already excludes.
const pragmaMarker = "pragma: no cover"

// FilterPragma drops every block whose source file has the pragma marker on
// any line within [StartLine,EndLine]. root is the directory blocks' module
// paths are resolved relative to after stripping modulePrefix (e.g.
// "mycorrhizal/") -- normally the backend/ directory the coverprofile's
// module paths (module "mycorrhizal") map onto directly.
func FilterPragma(blocks []Block, root, modulePrefix string) ([]Block, error) {
	markerLines := map[string]map[int]bool{} // file -> set of 1-based lines carrying the marker

	kept := make([]Block, 0, len(blocks))
	for _, b := range blocks {
		lines, ok := markerLines[b.File]
		if !ok {
			var err error
			lines, err = pragmaLinesOf(root, modulePrefix, b.File)
			if err != nil {
				return nil, err
			}
			markerLines[b.File] = lines
		}
		excluded := false
		for ln := b.StartLine; ln <= b.EndLine; ln++ {
			if lines[ln] {
				excluded = true
				break
			}
		}
		if !excluded {
			kept = append(kept, b)
		}
	}
	return kept, nil
}

func pragmaLinesOf(root, modulePrefix, moduleFile string) (map[int]bool, error) {
	rel := strings.TrimPrefix(moduleFile, modulePrefix)
	path := filepath.Join(root, filepath.FromSlash(rel))
	f, err := os.Open(path) // #nosec G304 -- path is built from the coverprofile's own recorded source list, under a caller-controlled root
	if err != nil {
		// A source file the profile mentions but that no longer exists on
		// disk (a stale/foreign profile) is not this package's problem to
		// diagnose -- treat it as having no pragma lines.
		return map[int]bool{}, nil // # pragma: no cover — defensive; every profile this ships with is generated from the checked-out tree moments earlier
	}
	defer f.Close() //nolint:errcheck // read-only coverprofile handle

	lines := map[int]bool{}
	scanner := bufio.NewScanner(f)
	n := 0
	for scanner.Scan() {
		n++
		if strings.Contains(scanner.Text(), pragmaMarker) {
			lines[n] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return lines, nil
}

// PerFileStats aggregates blocks into per-file statement coverage. Callers
// pass module-relative paths through as-is; ToRelative below converts them
// to the shape used in the baseline.
func PerFileStats(blocks []Block) map[string]FileStat {
	stats := map[string]FileStat{}
	for _, b := range blocks {
		s := stats[b.File]
		s.Statements += b.NumStmt
		if b.Count > 0 {
			s.Covered += b.NumStmt
		}
		stats[b.File] = s
	}
	return stats
}

// ToRelative rekeys a per-file stats map from the coverprofile's module path
// ("mycorrhizal/internal/foo/bar.go") to the baseline's committed key
// ("internal/foo/bar.go"), stripping modulePrefix.
func ToRelative(stats map[string]FileStat, modulePrefix string) map[string]FileStat {
	out := make(map[string]FileStat, len(stats))
	for file, s := range stats {
		out[strings.TrimPrefix(file, modulePrefix)] = s
	}
	return out
}

// Baseline is the committed testdata/baseline.json shape.
type Baseline struct {
	Comment      string             `json:"_comment"`
	TolerancePct float64            `json:"tolerancePercentPoints"`
	Files        map[string]float64 `json:"files"`
}

const baselineComment = "Generated by `go run ./cmd/coverageratchet -update` (or `make gen-coverage-baseline`) " +
	"from the merged backend/coverage.out. Per-file statement coverage %. A drop past " +
	"tolerancePercentPoints percentage points fails CI. Commit the diff -- it is the review."

// BuildBaseline turns per-file stats into the committed Baseline shape,
// keeping an existing baseline's tolerance if one is supplied (nil for a
// from-scratch generation, which falls back to defaultTolerancePct).
func BuildBaseline(stats map[string]FileStat, existing *Baseline, defaultTolerancePct float64) Baseline {
	tolerance := defaultTolerancePct
	if existing != nil {
		tolerance = existing.TolerancePct
	}
	files := make(map[string]float64, len(stats))
	for file, s := range stats {
		files[file] = round2(s.Percent())
	}
	return Baseline{Comment: baselineComment, TolerancePct: tolerance, Files: files}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// LoadBaseline reads a Baseline from path.
func LoadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a caller-controlled CLI flag / repo-relative constant, not user input
	if err != nil {
		return Baseline{}, err
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return b, nil
}

// SaveBaseline writes b to path as indented JSON, matching this repo's other
// generated-artifact files.
func SaveBaseline(path string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil { // # pragma: no cover — MarshalIndent of this struct can never fail
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Report is the result of comparing current per-file coverage against a
// baseline.
type Report struct {
	OK        bool
	Findings  []string
	NewFiles  []string
	GoneFiles []string
	DropFiles []string
}

// Compare finds every file whose coverage percentage dropped by more than
// baseline.TolerancePct percentage points. A file in `current` with no
// baseline entry (new) is not gated -- that is codecov/patch/backend's job.
// A baseline entry with no matching current file (removed/renamed) is
// reported but does not fail: the baseline is simply stale for that entry
// until the next `-update`.
func Compare(baseline Baseline, current map[string]FileStat) Report {
	var report Report
	report.OK = true

	names := make(map[string]bool, len(baseline.Files)+len(current))
	for f := range baseline.Files {
		names[f] = true
	}
	for f := range current {
		names[f] = true
	}
	sorted := make([]string, 0, len(names))
	for f := range names {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)

	for _, file := range sorted {
		basePct, inBase := baseline.Files[file]
		nowStat, inNow := current[file]

		switch {
		case !inBase:
			report.NewFiles = append(report.NewFiles, file)
		case !inNow:
			report.GoneFiles = append(report.GoneFiles, file)
		default:
			nowPct := nowStat.Percent()
			drop := basePct - nowPct
			if drop > baseline.TolerancePct {
				report.OK = false
				report.DropFiles = append(report.DropFiles, file)
				report.Findings = append(report.Findings, fmt.Sprintf(
					"%s: statement coverage dropped from %.2f%% to %.2f%% (-%.2fpt, over the %.2fpt tolerance)",
					file, basePct, nowPct, drop, baseline.TolerancePct,
				))
			}
		}
	}
	return report
}
