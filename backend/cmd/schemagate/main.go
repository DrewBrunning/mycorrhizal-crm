// Command schemagate enforces the Schemathesis fuzz gate (issue #369) on the
// JUnit XML report a `schemathesis run` invocation emits with
// `--report junit`.
//
// Schemathesis is a generator, not a policy engine: it fails (non-zero exit)
// on *any* check failure, and it has no notion of "this failure is accepted
// because it is legitimately scoped". schemagate is the small, deterministic
// policy layer that turns the JUnit report into a pass/fail decision, in the
// same "ignore-list with justification" shape as docker/cis-hardening.ignore:
//
//  1. The workflow runs Schemathesis with `--checks
//     not_a_server_error,ignored_auth`, so the only `<failure>` elements in the
//     report are the two classes the issue gates on. Each is classified by its
//     check title: "Server error" (a 5xx) and "API accepts requests without/
//     invalid authentication" (an auth-protected operation returning 2xx
//     without valid auth). JUnit `<error>` elements are *network* errors — the
//     scanner could not connect — and are deliberately not gated: they are a
//     coverage gap, not a server bug.
//  2. A gated failure is accepted only by an explicit ignore-list entry
//     matching its kind and operation label (e.g. `GET /contacts`), with the
//     justification logged so acceptances stay visible.
//  3. An empty report is a failure: a scan that produced no testcases did not
//     run, and a gate that passes on "no data" would be blind.
//
// Configuration is via SCHEMAGATE_* env vars (see loadConfig); the defaults
// match the repo layout (schemathesis/report.xml, schemathesis/schemathesis.ignore).
package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// finding is one gated failure, deduplicated per (kind, operation) so the same
// operation failing across the coverage and fuzzing phases counts once.
type finding struct {
	kind      string
	operation string
}

type config struct {
	reportPaths []string
	ignorePath  string
}

func defaultConfig() config {
	return config{
		reportPaths: []string{"schemathesis/report.xml"},
		ignorePath:  "schemathesis/schemathesis.ignore",
	}
}

// loadConfig reads SCHEMAGATE_REPORT (a comma-separated list of JUnit report
// paths) and SCHEMAGATE_IGNORE (the ignore-list path) via getenv, injected so
// this is testable without mutating real process env.
func loadConfig(getenv func(string) string) config {
	cfg := defaultConfig()
	if v := getenv("SCHEMAGATE_REPORT"); v != "" {
		cfg.reportPaths = splitPaths(v)
	}
	if v := getenv("SCHEMAGATE_IGNORE"); v != "" {
		cfg.ignorePath = v
	}
	return cfg
}

func splitPaths(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	if err := run(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "schemagate:", err)
		os.Exit(1)
	}
}

// --- JUnit report model ---

type junitTestsuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Tests   int          `xml:"tests,attr"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name     string         `xml:"name,attr"`
	Failures []junitFailure `xml:"failure"`
	// <error> elements are network errors and are deliberately not read.
}

type junitFailure struct {
	Text string `xml:",chardata"`
}

// --- ignore-list model ---

type ignoreRule struct {
	kind          string // "server_error", "auth", or "*"
	operationRe   *regexp.Regexp
	justification string
}

func run(getenv func(string) string) error {
	cfg := loadConfig(getenv)

	var findings []finding
	totalTests := 0
	for _, path := range cfg.reportPaths {
		f, tests, err := readReport(path)
		if err != nil {
			return fmt.Errorf("read report: %w", err)
		}
		totalTests += tests
		findings = append(findings, f...)
	}

	if totalTests == 0 {
		return fmt.Errorf("no Schemathesis test cases found in %v — the scan did not run or the report is empty", cfg.reportPaths)
	}

	findings = dedupe(findings)

	rules, err := readIgnoreList(cfg.ignorePath)
	if err != nil {
		return fmt.Errorf("read ignore-list: %w", err)
	}

	var failures []string
	for _, f := range findings {
		if rule, ok := matchIgnore(rules, f); ok {
			fmt.Printf("[ACCEPT] %s %s — %s\n", f.kind, f.operation, rule.justification)
			continue
		}
		failures = append(failures, fmt.Sprintf("%s %s", f.kind, f.operation))
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d unaccepted finding(s):\n  %s",
			len(failures), strings.Join(failures, "\n  "))
	}
	fmt.Printf("OK: schemagate passed — %d test case(s) across %d report(s), no unaccepted 5xx/auth findings.\n",
		totalTests, len(cfg.reportPaths))
	return nil
}

// readReport parses one JUnit report file, returning the gated findings and the
// total number of test cases (used as the "did it actually run" signal). A
// non-existent file is an error.
func readReport(path string) ([]finding, int, error) {
	// #nosec G304 -- path is an operator-supplied report path (SCHEMAGATE_REPORT), not request input.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}

	var ts junitTestsuites
	if err := xml.Unmarshal(data, &ts); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}

	var findings []finding
	for _, suite := range ts.Suites {
		for _, c := range suite.Cases {
			for _, f := range c.Failures {
				if kind, ok := classifyFailure(f.Text); ok {
					findings = append(findings, finding{kind: kind, operation: c.Name})
				}
			}
		}
	}
	return findings, ts.Tests, nil
}

// classifyFailure maps a JUnit failure's text to a gated kind via its
// Schemathesis check title. With `--checks not_a_server_error,ignored_auth`
// the only titles that can appear are "Server error" (a 5xx) and the two
// ignored-auth variants ("...without authentication" / "...invalid
// authentication"). Any other text is a non-gated check and returns false.
func classifyFailure(text string) (string, bool) {
	if strings.Contains(text, "Server error") {
		return "server_error", true
	}
	if strings.Contains(text, "authentication") {
		return "auth", true
	}
	return "", false
}

// dedupe collapses findings to one per (kind, operation).
func dedupe(findings []finding) []finding {
	seen := map[string]bool{}
	var out []finding
	for _, f := range findings {
		key := f.kind + "\x00" + f.operation
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].kind != out[j].kind {
			return out[i].kind < out[j].kind
		}
		return out[i].operation < out[j].operation
	})
	return out
}

// matchIgnore returns the first ignore rule accepting the finding, or false.
func matchIgnore(rules []ignoreRule, f finding) (ignoreRule, bool) {
	for _, r := range rules {
		if r.kind != "*" && r.kind != f.kind {
			continue
		}
		if r.operationRe == nil || r.operationRe.MatchString(f.operation) {
			return r, true
		}
	}
	return ignoreRule{}, false
}

// readIgnoreList parses schemathesis/schemathesis.ignore. Format: one rule per
// line, `<kind> <operation-regex>`, where kind is server_error|auth|* and
// operation-regex is a Go regexp matched against the JUnit testcase name — the
// Schemathesis operation label (e.g. `POST /contacts/import/upload`). The kind
// is the first whitespace-delimited token; everything after it is the regex, so
// a label's internal "METHOD /path" space is preserved. A `#` begins a comment,
// on its own line or trailing a rule; trailing comments are captured as the
// justification shown on [ACCEPT].
func readIgnoreList(path string) ([]ignoreRule, error) {
	// #nosec G304 -- path is an operator-supplied config path (SCHEMAGATE_IGNORE), not request input.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rules []ignoreRule
	for lineNo, line := range strings.Split(string(data), "\n") {
		raw := line

		justification := ""
		if i := strings.Index(raw, "#"); i >= 0 {
			justification = strings.TrimSpace(raw[i+1:])
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		i := strings.IndexAny(raw, " \t")
		if i < 0 {
			return nil, fmt.Errorf("%s:%d: expected '<kind> <operation-regex>', got %q",
				path, lineNo+1, raw)
		}
		kind := strings.ToLower(strings.TrimSpace(raw[:i]))
		op := strings.TrimSpace(raw[i+1:])
		if op == "" {
			return nil, fmt.Errorf("%s:%d: expected '<kind> <operation-regex>', got %q",
				path, lineNo+1, raw)
		}
		if kind != "*" && kind != "server_error" && kind != "auth" {
			return nil, fmt.Errorf("%s:%d: kind must be server_error, auth, or *, got %q",
				path, lineNo+1, kind)
		}

		var opRe *regexp.Regexp
		if op != "*" {
			opRe, err = regexp.Compile(op)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: bad operation-regex %q: %w", path, lineNo+1, op, err)
			}
		}

		rules = append(rules, ignoreRule{
			kind:          kind,
			operationRe:   opRe,
			justification: justification,
		})
	}
	return rules, nil
}
