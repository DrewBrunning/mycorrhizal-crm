package governance

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// RulesetStatus is one committed ruleset's live-vs-committed comparison
// result, as computed by governance-drift.yml's `gh api` + jq-normalization
// step. cmd/rulesetdrift turns a list of these, plus
// docs/security/governance-drift.ignore, into the job's pass/fail decision
// (issue #916).
type RulesetStatus struct {
	// Name is the ruleset's "name" field, matching a governance-drift.ignore
	// entry and a committed .github/rulesets/*.json.
	Name string `json:"name"`
	// Applied is false when the ruleset has no live counterpart at all --
	// the most severe drift, since the protection does not exist on GitHub
	// regardless of what is committed.
	Applied bool `json:"applied"`
	// Drifted is only meaningful when Applied is true: the live ruleset,
	// after the workflow's field-normalization, differs from the committed
	// JSON.
	Drifted bool `json:"drifted"`
}

// Mismatched reports whether this ruleset needs an accepted
// governance-drift.ignore entry to pass: it was never applied, or its live
// counterpart has drifted from what is committed.
func (s RulesetStatus) Mismatched() bool {
	return !s.Applied || s.Drifted
}

// IgnoreEntry is one parsed docs/security/governance-drift.ignore line.
type IgnoreEntry struct {
	Line   int
	Name   string
	Reason string
}

// driftIgnoreLineRE matches `<ruleset name>  # <reason>` -- the name is
// everything before the FIRST run of whitespace followed by `#` (non-greedy,
// since a reason may itself contain a `#`, e.g. "see #916"), the reason is
// everything after it.
var driftIgnoreLineRE = regexp.MustCompile(`^(\S.*?)\s+#\s*(\S.*)$`)

// ParseGovernanceDriftIgnore parses docs/security/governance-drift.ignore's
// `<ruleset name>  # <reason>` lines. A line that is not blank, not a
// whole-line `#` comment, and does not match that shape is reported as a
// finding rather than silently skipped -- same posture as cmd/depexceptions
// and cmd/citecheck's ignore-ledger parsers.
func ParseGovernanceDriftIgnore(body string) (map[string]IgnoreEntry, []string) {
	entries := map[string]IgnoreEntry{}
	var findings []string
	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lineNo := i + 1
		m := driftIgnoreLineRE.FindStringSubmatch(line)
		if m == nil {
			findings = append(findings, fmt.Sprintf(
				"governance-drift.ignore line %d: expected `<ruleset name>  # <reason>`, got %q", lineNo, line))
			continue
		}
		name := strings.TrimSpace(m[1])
		reason := strings.TrimSpace(m[2])
		if existing, dup := entries[name]; dup {
			findings = append(findings, fmt.Sprintf(
				"governance-drift.ignore line %d: duplicate entry for %q (first seen at line %d)", lineNo, name, existing.Line))
			continue
		}
		entries[name] = IgnoreEntry{Line: lineNo, Name: name, Reason: reason}
	}
	return entries, findings
}

// EvaluateGovernanceDrift decides, for a set of ruleset-drift results and the
// parsed governance-drift.ignore entries, which rulesets fail the job and
// which are accepted. It also flags ignore entries that no longer apply --
// the ruleset is back in sync, or the entry names no committed ruleset -- so
// the ignore file cannot quietly accumulate dead suppressions, the same rule
// citation-drift.ignore and crypto-surface.ignore already enforce.
//
// This is the #916 fix's core: before it, a mismatched status with no
// ignore entry produced no finding at all -- the workflow only ever warned.
func EvaluateGovernanceDrift(statuses []RulesetStatus, ignore map[string]IgnoreEntry) (findings, accepted []string) {
	seen := map[string]bool{}
	for _, s := range statuses {
		seen[s.Name] = true
		entry, ignored := ignore[s.Name]
		mismatched := s.Mismatched()
		switch {
		case mismatched && ignored:
			accepted = append(accepted, fmt.Sprintf("%s: %s -- accepted, see governance-drift.ignore: %s",
				s.Name, mismatchReason(s), entry.Reason))
		case mismatched && !ignored:
			findings = append(findings, fmt.Sprintf(
				"%s: %s. %s, or add a governance-drift.ignore entry.", s.Name, mismatchReason(s), mismatchAction(s)))
		case !mismatched && ignored:
			findings = append(findings, fmt.Sprintf(
				"governance-drift.ignore line %d: %q is back in sync with its live ruleset -- remove the stale entry",
				entry.Line, s.Name))
		}
	}
	for name, entry := range ignore {
		if !seen[name] {
			findings = append(findings, fmt.Sprintf(
				"governance-drift.ignore line %d: %q does not match any committed .github/rulesets/*.json", entry.Line, name))
		}
	}
	sort.Strings(findings)
	sort.Strings(accepted)
	return findings, accepted
}

func mismatchReason(s RulesetStatus) string {
	if !s.Applied {
		return "not applied to the repository -- no live ruleset with this name"
	}
	return "live ruleset has drifted from .github/rulesets/"
}

func mismatchAction(s RulesetStatus) string {
	if !s.Applied {
		return "Apply it"
	}
	return "Reconcile it"
}
