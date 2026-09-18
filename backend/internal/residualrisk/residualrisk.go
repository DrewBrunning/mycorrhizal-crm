// Package residualrisk assembles the release "what are we shipping with"
// statement (issue #953).
//
// A release's gate outcomes say what passed; they do not say what was accepted
// on the way. This package reads the repository's existing machine-readable
// accept lists and exception ledgers and produces one summary:
//
//   - the open accept-with-reason items across every justified ignore list
//     (.trivyignore, zap/dast.ignore, schemathesis/schemathesis.ignore,
//     docker/cis-hardening.ignore, android/.mobsf, .grype.yml,
//     docs/security/citation-drift.ignore, docs/security/crypto-surface.ignore,
//     docs/security/governance-drift.ignore);
//   - the open dependency-advisory exceptions, with their expiry and how soon
//     each is due;
//   - the current ASVS and MASVS documented-exception counts.
//
// It owns no policy: it only counts what those files already record, so the
// summary cannot become a second source of truth. The registry below is
// deliberately generic — it names the accept lists this project maintains, not
// the landing order of the issues that added any one of them.
package residualrisk

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Summary is the residual-risk section written into release-metadata.json.
type Summary struct {
	AcceptItems          AcceptItems       `json:"accept_items"`
	DependencyExceptions DependencySummary `json:"dependency_exceptions"`
	Exceptions           ExceptionCounts   `json:"exceptions"`
}

// AcceptItems is the count of open accept-with-reason entries across the
// project's justified ignore lists.
type AcceptItems struct {
	Total    int           `json:"total"`
	BySource []SourceCount `json:"by_source"`
}

// SourceCount is one accept list and how many entries it currently holds.
type SourceCount struct {
	Source  string `json:"source"`
	Entries int    `json:"entries"`
}

// DependencySummary is the state of docs/security/dependency-exceptions.ignore.
type DependencySummary struct {
	Open          int               `json:"open"`
	SoonExpiring  int               `json:"soon_expiring"`
	SoonestExpiry string            `json:"soonest_expiry,omitempty"`
	Entries       []DependencyEntry `json:"entries"`
}

// DependencyEntry is one open, time-bounded advisory exception.
type DependencyEntry struct {
	Advisory      string `json:"advisory"`
	Ecosystem     string `json:"ecosystem"`
	Package       string `json:"package"`
	Expires       string `json:"expires"`
	DaysRemaining int    `json:"days_remaining"`
	SoonExpiring  bool   `json:"soon_expiring"`
}

// ExceptionCounts is the documented-exception count per attestation. The
// values are read from the dated verification report, whose claim is itself
// asserted against the checklists by citecheck (issue #939).
type ExceptionCounts struct {
	ASVS  int `json:"asvs"`
	MASVS int `json:"masvs"`
}

// soonExpiringDays is how close an exception's expiry must be to be flagged
// "soon". 30 days is a warning window, not a policy number: the ledger's own
// 90-day ceiling (dependency-upgrade-policy.md, COMPAT-03) is unchanged.
const soonExpiringDays = 30

// countKind says how an accept list's entries are counted.
type countKind int

const (
	// countLines counts non-comment, non-blank lines — the shape shared by
	// the line-oriented ignore lists.
	countLines countKind = iota
	// countGrypeIgnore counts `- vulnerability:`-style items under `ignore:`.
	countGrypeIgnore
	// countMobsfRules counts `- rule` items under `ignore-rules:`.
	countMobsfRules
)

// acceptSource is one justified ignore list this project maintains.
type acceptSource struct {
	Path string
	Kind countKind
}

// acceptSources is the registry of accept-with-reason lists. Every one is a
// reviewed, justified ignore list (not a TODO); the summary counts what each
// holds so a release reader can see the accepted residual at a glance. Add a
// new list here when the project adopts one.
var acceptSources = []acceptSource{
	{Path: ".trivyignore", Kind: countLines},
	{Path: ".grype.yml", Kind: countGrypeIgnore},
	{Path: "zap/dast.ignore", Kind: countLines},
	{Path: "schemathesis/schemathesis.ignore", Kind: countLines},
	{Path: "docker/cis-hardening.ignore", Kind: countLines},
	{Path: "android/.mobsf", Kind: countMobsfRules},
	{Path: "docs/security/citation-drift.ignore", Kind: countLines},
	{Path: "docs/security/crypto-surface.ignore", Kind: countLines},
	{Path: "docs/security/governance-drift.ignore", Kind: countLines},
}

// ReportFile is the dated attestation whose claim carries the exception counts.
const ReportFile = "docs/security/asvs-l2-verification-report.md"

// ExceptionsFile is the time-bounded dependency-advisory ledger.
const ExceptionsFile = "docs/security/dependency-exceptions.ignore"

// dateLayout is the ledger's date format.
const dateLayout = "2006-01-02"

// asvsClaimRE / masvsClaimRE capture the documented-exception counts from the
// report's "The claim" block, e.g. "ASVS Level 2, with 23 documented
// exceptions" and "MASVS-L1, with 1 documented exception".
var (
	asvsClaimRE  = regexp.MustCompile(`ASVS Level 2, with (\d+) documented exception`)
	masvsClaimRE = regexp.MustCompile(`MASVS-L1, with (\d+) documented exception`)
)

// Summarize reads the repository rooted at root and returns the residual-risk
// summary as of now. It fails rather than silently omitting a declared source:
// an absent or malformed list is a reason to fix the list, not to ship a
// misleading "nothing accepted" statement.
func Summarize(root string, now time.Time) (Summary, error) {
	accept, err := summarizeAcceptItems(root)
	if err != nil {
		return Summary{}, err
	}
	deps, err := summarizeDependencyExceptions(root, now)
	if err != nil {
		return Summary{}, err
	}
	exceptions, err := summarizeExceptions(root)
	if err != nil {
		return Summary{}, err
	}
	return Summary{AcceptItems: accept, DependencyExceptions: deps, Exceptions: exceptions}, nil
}

// summarizeAcceptItems counts every registered accept list.
func summarizeAcceptItems(root string) (AcceptItems, error) {
	out := AcceptItems{BySource: make([]SourceCount, 0, len(acceptSources))}
	for _, src := range acceptSources {
		body, err := readRepoFile(root, src.Path)
		if err != nil {
			return AcceptItems{}, fmt.Errorf("accept list %s: %w", src.Path, err)
		}
		n, err := countAcceptEntries(body, src.Kind)
		if err != nil {
			return AcceptItems{}, fmt.Errorf("accept list %s: %w", src.Path, err)
		}
		out.BySource = append(out.BySource, SourceCount{Source: src.Path, Entries: n})
		out.Total += n
	}
	return out, nil
}

// countAcceptEntries counts entries of the given kind.
func countAcceptEntries(body []byte, kind countKind) (int, error) {
	switch kind {
	case countLines:
		return countNonCommentLines(body), nil
	case countGrypeIgnore:
		return countYAMLListItems(body, "ignore:"), nil
	case countMobsfRules:
		return countYAMLListItems(body, "ignore-rules:"), nil
	default:
		return 0, fmt.Errorf("unknown count kind %d", kind)
	}
}

// countNonCommentLines counts non-blank lines whose first non-space character
// is not `#`. A trailing justification comment is part of its entry line, so
// it is not stripped — only whole-line comments are excluded.
func countNonCommentLines(body []byte) int {
	n := 0
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		n++
	}
	return n
}

// countYAMLListItems counts `- item` entries nested under a top-level `<key>`
// line. It is intentionally minimal: it exists to count the two YAML-shaped
// accept lists (grype's and mobsf's), not to be a general YAML reader. A
// `key: []` inline empty list yields zero.
func countYAMLListItems(body []byte, key string) int {
	lines := strings.Split(string(body), "\n")
	inKey := false
	n := 0
	for _, line := range lines {
		if !inKey {
			if strings.TrimRight(line, " \t") == key || strings.HasPrefix(line, key+" ") {
				if rest := strings.TrimSpace(strings.TrimPrefix(line, key)); rest == "[]" {
					return 0
				}
				// An inline non-empty list is out of scope for this counter;
				// treat it as the key being open and keep scanning, and a
				// following indented `- item` still counts.
				inKey = true
			}
			continue
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// A non-indented line ends the list.
		if line[0] != ' ' && line[0] != '\t' {
			inKey = false
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "- ") || strings.TrimSpace(line) == "-" {
			n++
		}
	}
	return n
}

// summarizeDependencyExceptions parses the pipe-delimited exception ledger and
// reports the entries that are still open as of now.
func summarizeDependencyExceptions(root string, now time.Time) (DependencySummary, error) {
	body, err := readRepoFile(root, ExceptionsFile)
	if err != nil {
		return DependencySummary{}, fmt.Errorf("%s: %w", ExceptionsFile, err)
	}
	out := DependencySummary{Entries: []DependencyEntry{}}
	for i, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(trimmed, "|")
		if len(fields) != 9 {
			return DependencySummary{}, fmt.Errorf("%s line %d: expected 9 pipe-delimited fields, got %d", ExceptionsFile, i+1, len(fields))
		}
		for j := range fields {
			fields[j] = strings.TrimSpace(fields[j])
		}
		expires, err := time.Parse(dateLayout, fields[5])
		if err != nil {
			return DependencySummary{}, fmt.Errorf("%s line %d: expires %q is not %s: %w", ExceptionsFile, i+1, fields[5], dateLayout, err)
		}
		days := int(expires.Sub(now).Hours() / 24)
		entry := DependencyEntry{
			Advisory:      fields[0],
			Ecosystem:     fields[1],
			Package:       fields[2],
			Expires:       expires.Format(dateLayout),
			DaysRemaining: days,
			SoonExpiring:  days <= soonExpiringDays,
		}
		out.Entries = append(out.Entries, entry)
		out.Open++
		if entry.SoonExpiring {
			out.SoonExpiring++
		}
		if out.SoonestExpiry == "" || expires.Before(mustParse(out.SoonestExpiry)) {
			out.SoonestExpiry = entry.Expires
		}
	}
	// Sort by soonest expiry so the report reads in the order that matters.
	sort.SliceStable(out.Entries, func(i, j int) bool {
		return out.Entries[i].Expires < out.Entries[j].Expires
	})
	return out, nil
}

// summarizeExceptions reads the documented-exception counts from the report's
// claim block.
func summarizeExceptions(root string) (ExceptionCounts, error) {
	body, err := readRepoFile(root, ReportFile)
	if err != nil {
		return ExceptionCounts{}, fmt.Errorf("%s: %w", ReportFile, err)
	}
	text := string(body)
	asvs, err := firstCapture(asvsClaimRE, text, "ASVS")
	if err != nil {
		return ExceptionCounts{}, fmt.Errorf("%s: %w", ReportFile, err)
	}
	masvs, err := firstCapture(masvsClaimRE, text, "MASVS")
	if err != nil {
		return ExceptionCounts{}, fmt.Errorf("%s: %w", ReportFile, err)
	}
	return ExceptionCounts{ASVS: asvs, MASVS: masvs}, nil
}

func firstCapture(re *regexp.Regexp, text, which string) (int, error) {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0, fmt.Errorf("could not find the %s documented-exception count in the claim block", which)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse %s exception count %q: %w", which, m[1], err) // # pragma: no cover — the regexp captures digits only
	}
	return n, nil
}

func mustParse(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

// readRepoFile reads a repository-relative file, refusing anything that would
// escape root.
func readRepoFile(root, rel string) ([]byte, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("refusing to read %q — outside the repository root", rel)
	}
	// #nosec G304 -- rel is a compile-time path from acceptSources, ReportFile
	// or ExceptionsFile and is traversal-checked immediately above; root comes
	// from findRepoRoot walking up from the working directory, never from
	// request input. Same posture as cmd/asvsstale's readRepoFile.
	return os.ReadFile(filepath.Join(root, clean))
}
