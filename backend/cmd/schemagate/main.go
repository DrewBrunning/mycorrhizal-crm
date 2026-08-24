// Command schemagate enforces the Schemathesis fuzz gate (issue #369) on the
// NDJSON event report a `schemathesis run` invocation emits with
// `--report ndjson`.
//
// Schemathesis is a generator, not a policy engine: it fails (non-zero exit)
// on *any* check failure, including the schema-conformance noise that is
// expected while the spec catches up with reality, and it has no notion of
// "this failure is accepted because it is legitimately scoped". schemagate is
// the small, deterministic policy layer that turns the NDJSON report into a
// pass/fail decision, in the same "ignore-list with justification" shape as
// zapgate (backend/cmd/zapgate) and docker/cis-hardening.ignore:
//
//  1. Failures are classified by the machine-readable Schemathesis check name:
//     `not_a_server_error` (a 5xx response) and `ignored_auth` (an
//     auth-protected operation returning 2xx without valid auth) are the two
//     classes the issue gates on. Everything else — status-code conformance,
//     response-schema conformance, negative-data rejection, missing headers,
//     unsupported methods — is reported but does not fail the gate.
//  2. A gated failure is accepted only by an explicit ignore-list entry
//     matching its kind and operation label (e.g. `GET /contacts`), with the
//     justification logged so acceptances stay visible.
//  3. Unknown/empty reports are a failure: a scan that produced no events did
//     not run, and a gate that passes on "no data" would be blind.
//
// Configuration is via SCHEMAGATE_* env vars (see loadConfig); the defaults
// match the repo layout (schemathesis/report.ndjson, schemathesis/schemathesis.ignore).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// gateCheckNames are the machine-readable Schemathesis check names the gate
// enforces, mapped to the kind recorded in the ignore-list and the human
// label used in messages. This is deliberately narrower than Schemathesis's
// full check set (see the package doc): conformance drift is surfaced by the
// report, but the gate is on 5xx + auth failures per issue #369.
var gateCheckNames = map[string]gateKind{
	"not_a_server_error": {kind: "server_error", label: "server error (5xx)"},
	"ignored_auth":       {kind: "auth", label: "auth-protected operation returned data without auth"},
}

type gateKind struct {
	kind  string
	label string
}

type config struct {
	reportPaths []string
	ignorePath  string
}

func defaultConfig() config {
	return config{
		reportPaths: []string{"schemathesis/report.ndjson"},
		ignorePath:  "schemathesis/schemathesis.ignore",
	}
}

// loadConfig reads SCHEMAGATE_REPORT (a comma-separated list of NDJSON report
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

// --- Schemathesis NDJSON report model ---

// ndjsonEvent is one line of a `--report ndjson` report. The report interleaves
// several event shapes; only ScenarioFinished carries the per-check results the
// gate reads, so every other event type is ignored.
type ndjsonEvent struct {
	ScenarioFinished *scenarioFinished `json:"ScenarioFinished"`
}

type scenarioFinished struct {
	Status   string           `json:"status"`
	Recorder scenarioRecorder `json:"recorder"`
}

type scenarioRecorder struct {
	Label  string                   `json:"label"`
	Cases  map[string]caseInfo      `json:"cases"`
	Checks map[string][]checkResult `json:"checks"`
}

type caseInfo struct {
	Value caseValue `json:"value"`
}

type caseValue struct {
	Path string `json:"path"`
}

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// finding is one gated failure, deduplicated per (kind, operation) so the same
// operation failing across the coverage and fuzzing phases counts once.
type finding struct {
	kind      string
	operation string
	path      string
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
	totalChecks := 0
	for _, path := range cfg.reportPaths {
		f, checks, err := readReport(path)
		if err != nil {
			return fmt.Errorf("read report: %w", err)
		}
		totalChecks += checks
		findings = append(findings, f...)
	}

	if totalChecks == 0 {
		return fmt.Errorf("no Schemathesis check results found in %v — the scan did not run or the report is empty", cfg.reportPaths)
	}

	findings = dedupe(findings)

	rules, err := readIgnoreList(cfg.ignorePath)
	if err != nil {
		return fmt.Errorf("read ignore-list: %w", err)
	}

	var failures []string
	for _, f := range findings {
		if rule, ok := matchIgnore(rules, f); ok {
			fmt.Printf("[ACCEPT] %s %s (%s) — %s\n", f.kind, f.operation, f.path, rule.justification)
			continue
		}
		failures = append(failures, fmt.Sprintf("%s %s (%s)", f.kind, f.operation, f.path))
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d unaccepted finding(s):\n  %s",
			len(failures), strings.Join(failures, "\n  "))
	}
	fmt.Printf("OK: schemagate passed — %d check result(s) across %d report(s), no unaccepted 5xx/auth findings.\n",
		totalChecks, len(cfg.reportPaths))
	return nil
}

// readReport parses one NDJSON report file, returning the gated findings and
// the total number of check results seen (used as the "did it actually run"
// signal). A non-existent file is an error; a file with zero check results is
// not (a report may legitimately contain only public endpoints with no gated
// failures) — the run() caller fails only when *every* report is empty.
func readReport(path string) ([]finding, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var findings []finding
	checks := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev ndjsonEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, 0, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		sf := ev.ScenarioFinished
		if sf == nil {
			continue
		}
		// Iterate the checks map in sorted case-ID order so a given report
		// parses to a deterministic finding order (maps are unordered).
		caseIDs := make([]string, 0, len(sf.Recorder.Checks))
		for id := range sf.Recorder.Checks {
			caseIDs = append(caseIDs, id)
		}
		sort.Strings(caseIDs)
		for _, id := range caseIDs {
			ci, ok := sf.Recorder.Cases[id]
			if !ok {
				continue
			}
			for _, c := range sf.Recorder.Checks[id] {
				checks++
				gk, ok := gateCheckNames[c.Name]
				if !ok {
					continue
				}
				if c.Status != "failure" && c.Status != "error" {
					continue
				}
				findings = append(findings, finding{
					kind:      gk.kind,
					operation: sf.Recorder.Label,
					path:      ci.Value.Path,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	return findings, checks, nil
}

// dedupe collapses findings to one per (kind, operation), preserving the first
// concrete request for the message.
func dedupe(findings []finding) []finding {
	seen := map[string]int{}
	var out []finding
	for _, f := range findings {
		key := f.kind + "\x00" + f.operation
		if idx, ok := seen[key]; ok {
			if out[idx].path == "" && f.path != "" {
				out[idx].path = f.path
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, f)
	}
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
// operation-regex is a Go regexp matched against the Schemathesis operation
// label (e.g. `POST /contacts/import/upload`). The kind is the first
// whitespace-delimited token; everything after it is the regex, so a label's
// internal "METHOD /path" space is preserved. A `#` begins a comment, on its
// own line or trailing a rule; trailing comments are captured as the
// justification shown on [ACCEPT].
func readIgnoreList(path string) ([]ignoreRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []ignoreRule
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()

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
				path, lineNo, raw)
		}
		kind := strings.ToLower(strings.TrimSpace(raw[:i]))
		op := strings.TrimSpace(raw[i+1:])
		if op == "" {
			return nil, fmt.Errorf("%s:%d: expected '<kind> <operation-regex>', got %q",
				path, lineNo, raw)
		}
		if kind != "*" && kind != "server_error" && kind != "auth" {
			return nil, fmt.Errorf("%s:%d: kind must be server_error, auth, or *, got %q",
				path, lineNo, kind)
		}

		var opRe *regexp.Regexp
		if op != "*" {
			opRe, err = regexp.Compile(op)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: bad operation-regex %q: %w", path, lineNo, op, err)
			}
		}

		rules = append(rules, ignoreRule{
			kind:          kind,
			operationRe:   opRe,
			justification: justification,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}
