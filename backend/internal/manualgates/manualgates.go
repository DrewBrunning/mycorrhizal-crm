// Package manualgates is the logic behind the manual-gate attestation ledger
// (issue #1486): the release obligations that only a human can discharge --
// today, running LocalOnlyModeE2eTest on a real arm64 device -- recorded in
// .github/manual-gates.json and enforced at release-dispatch time.
//
// A manual gate has no CI job that can go red, so before this package
// forgetting it was silent. The model mirrors release.yml's `ack_*` inputs:
// every gate that applies to a release must be named in the dispatch's
// `attest_manual_gates` input, either as a bare id (meaning "the ledger holds a
// fresh attestation") or as `id=skip:<reason>` (a recorded, reasoned skip). A
// bare id is VERIFIED against the ledger -- the attestation must be young
// enough, tied to a commit that is an ancestor of the release commit, and no
// watched path may have changed since -- so claiming a gate is not enough.
//
// Everything here is pure: git facts (is the attested commit an ancestor of
// the release commit; which files changed since) arrive as data, produced by
// .github/scripts/manual-gate-facts.sh, so the package needs no os/exec and
// every branch is unit-testable.
package manualgates

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Release kinds a gate can apply to.
const (
	KindFinal = "final"
	KindRC    = "rc"
)

// Decision modes recorded in the readiness artifact.
const (
	ModeAttested = "attested"
	ModeSkipped  = "skipped"
)

// MinReasonLen is the shortest accepted skip reason; "x" is not a reason.
const MinReasonLen = 8

// Attestation is one human run of a manual gate. The fields are what make it
// verifiable rather than a bare claim: the commit it ran against, the device
// ABI, and the JUnit counts + digest of the XML that proves the test RAN (a
// skipped test reports tests>=1 with skipped>=1, which is why skipped must be
// zero).
type Attestation struct {
	Date        string `json:"date"`         // YYYY-MM-DD (UTC) the run finished
	Commit      string `json:"commit"`       // full SHA of the tree the run exercised
	Attester    string `json:"attester"`     // who ran it
	Evidence    string `json:"evidence"`     // URL or note pointing at the retained JUnit XML / log
	Device      string `json:"device"`       // device model, e.g. "Pixel 8a"
	ABI         string `json:"abi"`          // primary ABI the test ran on
	Tests       int    `json:"tests"`        // JUnit <testsuite tests=>
	Skipped     int    `json:"skipped"`      // JUnit <testsuite skipped=>
	Failures    int    `json:"failures"`     // JUnit failures + errors
	JUnitSHA256 string `json:"junit_sha256"` // sha256 of the JUnit XML the counts came from
}

// Gate is one manual obligation.
type Gate struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	AppliesTo  []string `json:"applies_to"`
	MaxAgeDays int      `json:"max_age_days"`
	WatchPaths []string `json:"watch_paths"`
	RequireABI string   `json:"require_abi,omitempty"`
	// EvidenceMarker, when set, must appear in the retained JUnit XML (the test
	// class that proves the RIGHT test ran, not just some test).
	EvidenceMarker string        `json:"evidence_marker,omitempty"`
	Run            string        `json:"run"`
	Attestations   []Attestation `json:"attestations"`
}

// Ledger is .github/manual-gates.json.
type Ledger struct {
	Comment string `json:"_comment"`
	Gates   []Gate `json:"gates"`
}

// Fact is what git says about one attested commit relative to the release
// commit.
type Fact struct {
	Ancestor bool     `json:"ancestor"` // exists and is an ancestor of (or equal to) the release commit
	Changed  []string `json:"changed"`  // paths changed between the commit and the release commit
}

// Facts maps a commit SHA to its Fact.
type Facts map[string]Fact

// Record is what one release records per applicable gate in
// release-readiness.json (`manual_gates`).
type Record struct {
	ID          string       `json:"id"`
	Mode        string       `json:"mode"`
	Reason      string       `json:"reason"`       // the skip reason; "" when attested
	AttestedBy  string       `json:"attested_by"`  // the dispatching actor
	LedgerState string       `json:"ledger_state"` // "fresh" or why the ledger entry does not qualify
	Attestation *Attestation `json:"attestation"`  // the ledger entry relied on or ignored; nil when none
	CheckedAt   string       `json:"checked_at"`
}

var (
	idRE     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	shaRE    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ParseLedger decodes and structurally validates the ledger. It returns every
// finding it can (not just the first) so one run reports the whole problem.
func ParseLedger(data []byte) (Ledger, []string) {
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, []string{fmt.Sprintf("manual-gates ledger does not parse: %v", err)}
	}
	var findings []string
	if len(l.Gates) == 0 {
		findings = append(findings, "manual-gates ledger lists no gates")
	}
	seen := map[string]bool{}
	for i, g := range l.Gates {
		where := fmt.Sprintf("gate #%d", i+1)
		if g.ID != "" {
			where = "gate `" + g.ID + "`"
		}
		if !idRE.MatchString(g.ID) {
			findings = append(findings, where+": id must be lowercase letters, digits and hyphens")
		} else if seen[g.ID] {
			findings = append(findings, where+": duplicate id")
		}
		seen[g.ID] = true
		if strings.TrimSpace(g.Name) == "" {
			findings = append(findings, where+": name is empty")
		}
		if strings.TrimSpace(g.Run) == "" {
			findings = append(findings, where+": run (how to discharge it) is empty")
		}
		if g.MaxAgeDays <= 0 {
			findings = append(findings, where+": max_age_days must be positive")
		}
		if len(g.AppliesTo) == 0 {
			findings = append(findings, where+": applies_to is empty")
		}
		for _, k := range g.AppliesTo {
			if k != KindFinal && k != KindRC {
				findings = append(findings, fmt.Sprintf("%s: applies_to has unknown kind %q (want %q or %q)", where, k, KindFinal, KindRC))
			}
		}
		if len(g.WatchPaths) == 0 {
			findings = append(findings, where+": watch_paths is empty -- an attestation could never go stale when the code under test changes")
		}
		for _, p := range g.WatchPaths {
			if strings.TrimSpace(p) == "" || strings.HasPrefix(p, "/") {
				findings = append(findings, fmt.Sprintf("%s: watch path %q must be a non-empty repo-relative glob", where, p))
			} else if err := validGlob(p); err != nil {
				findings = append(findings, fmt.Sprintf("%s: watch path %q is not a valid glob: %v", where, p, err))
			}
		}
		for j, a := range g.Attestations {
			for _, f := range validateAttestation(g, a) {
				findings = append(findings, fmt.Sprintf("%s attestation #%d: %s", where, j+1, f))
			}
		}
	}
	return l, findings
}

// validGlob reports a malformed segment pattern (`**` segments are the
// multi-segment wildcard and need no validation).
func validGlob(p string) error {
	for _, seg := range strings.Split(p, "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}

// validateAttestation is the verifiable-format rule: an attestation that does
// not prove the test ran, on the right hardware, without failures, is
// malformed, not merely stale.
func validateAttestation(g Gate, a Attestation) []string {
	var f []string
	if _, err := time.Parse("2006-01-02", a.Date); err != nil {
		f = append(f, fmt.Sprintf("date %q must be YYYY-MM-DD", a.Date))
	}
	if !shaRE.MatchString(a.Commit) {
		f = append(f, "commit must be a full 40-character lowercase SHA")
	}
	if strings.TrimSpace(a.Attester) == "" {
		f = append(f, "attester is empty")
	}
	if strings.TrimSpace(a.Evidence) == "" {
		f = append(f, "evidence is empty")
	}
	if strings.TrimSpace(a.Device) == "" {
		f = append(f, "device is empty")
	}
	if a.Tests < 1 {
		f = append(f, "tests must be at least 1 (the JUnit run executed nothing)")
	}
	if a.Skipped != 0 {
		f = append(f, fmt.Sprintf("skipped is %d: a skipped test is not evidence the gate ran", a.Skipped))
	}
	if a.Failures != 0 {
		f = append(f, fmt.Sprintf("failures is %d: the run did not pass", a.Failures))
	}
	if !sha256RE.MatchString(a.JUnitSHA256) {
		f = append(f, "junit_sha256 must be the 64-hex sha256 of the JUnit XML")
	}
	if g.RequireABI != "" && a.ABI != g.RequireABI {
		f = append(f, fmt.Sprintf("abi is %q, the gate requires %q", a.ABI, g.RequireABI))
	}
	return f
}

// Applies reports whether the gate applies to a release kind.
func (g Gate) Applies(kind string) bool {
	for _, k := range g.AppliesTo {
		if k == kind {
			return true
		}
	}
	return false
}

// Latest returns the gate's newest attestation (by date, ties to the later
// entry), or nil.
func (g Gate) Latest() *Attestation {
	var best *Attestation
	for i := range g.Attestations {
		if best == nil || g.Attestations[i].Date >= best.Date {
			best = &g.Attestations[i]
		}
	}
	return best
}

// Commits returns the distinct commits of every gate's latest attestation --
// the commits the facts script has to examine.
func (l Ledger) Commits() []string {
	set := map[string]bool{}
	for _, g := range l.Gates {
		if a := g.Latest(); a != nil {
			set[a.Commit] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Input is one gate's entry in the dispatch's attest_manual_gates.
type Input struct {
	Skip   bool
	Reason string
}

// ParseAttest parses the `attest_manual_gates` dispatch input: entries
// separated by `;` or newlines, each `<id>` (attest -- verified against the
// ledger) or `<id>=skip:<reason>` (a recorded skip). A skip reason may contain
// commas but not `;`.
func ParseAttest(s string) (map[string]Input, []string) {
	out := map[string]Input{}
	var findings []string
	for _, raw := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' }) {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		id, rest, hasEq := strings.Cut(tok, "=")
		id = strings.TrimSpace(id)
		if !idRE.MatchString(id) {
			findings = append(findings, fmt.Sprintf("attest_manual_gates entry %q: %q is not a gate id", tok, id))
			continue
		}
		if _, dup := out[id]; dup {
			findings = append(findings, fmt.Sprintf("attest_manual_gates names `%s` twice", id))
			continue
		}
		if !hasEq {
			out[id] = Input{}
			continue
		}
		reason, ok := strings.CutPrefix(strings.TrimSpace(rest), "skip:")
		reason = strings.TrimSpace(reason)
		switch {
		case !ok:
			findings = append(findings, fmt.Sprintf("attest_manual_gates entry %q: after `=` only `skip:<reason>` is accepted", tok))
		case len(reason) < MinReasonLen:
			findings = append(findings, fmt.Sprintf("attest_manual_gates entry for `%s`: a skip needs a real reason (at least %d characters), got %q", id, MinReasonLen, reason))
		default:
			out[id] = Input{Skip: true, Reason: reason}
		}
	}
	return out, findings
}

// Matches reports whether a repo-relative file path matches a watch glob. `**`
// matches any number of path segments (including none); other segments use
// path.Match, so `*` never crosses a `/`.
func Matches(glob, file string) bool {
	return matchSegs(strings.Split(glob, "/"), strings.Split(file, "/"))
}

func matchSegs(g, f []string) bool {
	for len(g) > 0 {
		if g[0] == "**" {
			if len(g) == 1 {
				return true
			}
			for i := 0; i <= len(f); i++ {
				if matchSegs(g[1:], f[i:]) {
					return true
				}
			}
			return false
		}
		if len(f) == 0 {
			return false
		}
		if ok, err := path.Match(g[0], f[0]); err != nil || !ok {
			return false
		}
		g, f = g[1:], f[1:]
	}
	return len(f) == 0
}

// ledgerState explains whether the gate's ledger entry qualifies as a fresh
// attestation for a release at the checked commit. "fresh" means it does.
func ledgerState(g Gate, now time.Time, facts Facts) (string, *Attestation) {
	a := g.Latest()
	if a == nil {
		return "no attestation recorded in the ledger", nil
	}
	day, err := time.Parse("2006-01-02", a.Date)
	if err != nil {
		return fmt.Sprintf("attestation date %q is unparseable", a.Date), a
	}
	if age := now.Sub(day); age > time.Duration(g.MaxAgeDays)*24*time.Hour {
		return fmt.Sprintf("last attested %s, %d days ago (max %d)", a.Date, int(age.Hours()/24), g.MaxAgeDays), a
	}
	fact, ok := facts[a.Commit]
	if !ok || !fact.Ancestor {
		return fmt.Sprintf("attested commit %.12s is not an ancestor of the release commit (or is unknown to this checkout)", a.Commit), a
	}
	var hit []string
	for _, file := range fact.Changed {
		for _, w := range g.WatchPaths {
			if Matches(w, file) {
				hit = append(hit, file)
				break
			}
		}
	}
	if len(hit) > 0 {
		sort.Strings(hit)
		more := ""
		if len(hit) > 5 {
			more = fmt.Sprintf(" (+%d more)", len(hit)-5)
			hit = hit[:5]
		}
		return fmt.Sprintf("watched paths changed since the attested commit %.12s: %s%s", a.Commit, strings.Join(hit, ", "), more), a
	}
	return "fresh", a
}

// Check enforces the dispatch's attestations against the ledger for a release
// of the given kind and returns one Record per applicable gate plus findings.
// Records are returned even alongside findings so a caller can show what it
// understood.
func Check(l Ledger, kind string, in map[string]Input, now time.Time, facts Facts, actor string) ([]Record, []string) {
	var records []Record
	var findings []string
	checked := now.UTC().Format(time.RFC3339)

	known := map[string]Gate{}
	for _, g := range l.Gates {
		known[g.ID] = g
	}
	ids := make([]string, 0, len(in))
	for id := range in {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g, ok := known[id]
		switch {
		case !ok:
			findings = append(findings, fmt.Sprintf("attest_manual_gates names `%s`, which is not a gate in the manual-gates ledger", id))
		case !g.Applies(kind):
			findings = append(findings, fmt.Sprintf("attest_manual_gates names `%s`, which does not apply to a %s release", id, kind))
		}
	}

	for _, g := range l.Gates {
		if !g.Applies(kind) {
			continue
		}
		choice, given := in[g.ID]
		if !given {
			findings = append(findings, fmt.Sprintf("manual gate `%s` (%s) is not addressed by attest_manual_gates: attest it (`%s`) after running `%s`, or skip it with `%s=skip:<reason>`", g.ID, g.Name, g.ID, g.Run, g.ID))
			continue
		}
		state, att := ledgerState(g, now, facts)
		rec := Record{ID: g.ID, AttestedBy: actor, LedgerState: state, Attestation: att, CheckedAt: checked}
		if choice.Skip {
			rec.Mode, rec.Reason = ModeSkipped, choice.Reason
		} else {
			rec.Mode = ModeAttested
			if state != "fresh" {
				findings = append(findings, fmt.Sprintf("manual gate `%s` cannot be attested: %s. Run `%s` and commit the recorded attestation, or skip it with `%s=skip:<reason>`", g.ID, state, g.Run, g.ID))
			}
		}
		records = append(records, rec)
	}
	return records, findings
}
