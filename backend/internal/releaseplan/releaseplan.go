// Package releaseplan decides which release gates actually have to run
// (issue #1487).
//
// Release reliability data showed the release battery -- ~15 composed gates --
// being re-rolled in full on every attempt and again at tag time, so every
// flaky suite got another chance to fail and a failure anywhere cost the whole
// battery. This package is the testable core of the three mechanisms that stop
// that, shared by release.yml and docker-publish.yml through
// `cmd/releaseplan`:
//
//   - the gate LEDGER: one {sha, release_tag, gate, conclusion, run_id} record
//     per composed gate, written by release-validate.yml's `results` job, so a
//     later run can tell exactly what already passed and for which commit;
//   - PlanRerun: `release.yml`'s `rerun_gates` input -- re-run only the named
//     (or only the not-green) gates and carry every other gate's recorded
//     success forward from the prior ledger;
//   - DecideReuse: `docker-publish.yml`'s tag-time decision -- when the tagged
//     commit differs from the validated commit only by the schema-fixture
//     registration, accept the pre-tag battery instead of running it a second
//     time.
//
// All three are deliberately conservative: a carried or reused result is only
// ever a recorded `success` for the same commit and release tag, and anything
// unprovable (missing entry, other conclusion, an unexpected changed file, a
// ledger spanning two commits) falls back to running the gate.
package releaseplan

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// CandidateJob is the composer's non-gate build job; it is never skipped (the
// published image digest comes from it) and never appears in the ledger.
const CandidateJob = "build-candidate"

// ResultsJob is the composer's own bookkeeping job; not a gate.
const ResultsJob = "results"

// RetestOnReuse lists the gates a tag-time reuse still runs. Reuse accepts the
// pre-tag battery because the tagged tree differs from the validated tree only
// by inert fixture files -- but the image the release publishes is a fresh
// build from the tag, so its clean-install smoke (cheap, and the one gate that
// boots exactly the bytes that ship) is kept as the floor under that argument.
var RetestOnReuse = []string{"deploy-smoke"}

// FixtureAllowlist returns the only paths the release workflow's fixture commit
// may change (release.yml "Commit the schema fixture"): the new schema dump and
// the SupportedReleases registration. A tree diff confined to these is the
// "same tree" condition for reusing a validated battery.
func FixtureAllowlist(tag string) []string {
	return []string{
		"backend/database/testdata/schemas/" + tag + ".sql",
		"backend/internal/schemafixture/releases.go",
	}
}

// Conclusion values recorded in the ledger. They mirror GitHub's job results.
const (
	ConclusionSuccess = "success"
	ConclusionSkipped = "skipped"
)

// Entry is one gate's recorded outcome for one commit.
type Entry struct {
	SHA        string `json:"sha"`
	ReleaseTag string `json:"release_tag"`
	Gate       string `json:"gate"`
	Conclusion string `json:"conclusion"`
	RunID      string `json:"run_id"`
	// CarriedFromRun is set when this run did not execute the gate but carried
	// its success forward from an earlier run (rerun_gates or tag-time reuse);
	// RunID is then the CURRENT run and CarriedFromRun the run that earned it.
	CarriedFromRun string `json:"carried_from_run,omitempty"`
}

// Ledger is the per-gate record of one battery.
type Ledger []Entry

// ParseLedger decodes a ledger. An empty document is an empty ledger.
func ParseLedger(data []byte) (Ledger, error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("ledger does not parse: %w", err)
	}
	return l, nil
}

// ComposedGates returns the gate ids of the composer: every job that calls a
// reusable workflow (`uses:`), sorted. Reading them from the composer rather
// than listing them here is what stops this package drifting from it.
func ComposedGates(composerYAML string) ([]string, error) {
	var doc struct {
		Jobs map[string]struct {
			Uses string `yaml:"uses"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(composerYAML), &doc); err != nil {
		return nil, fmt.Errorf("composer does not parse: %w", err)
	}
	var gates []string
	for id, j := range doc.Jobs {
		if j.Uses != "" {
			gates = append(gates, id)
		}
	}
	if len(gates) == 0 {
		return nil, fmt.Errorf("composer declares no gate jobs (jobs with `uses:`)")
	}
	sort.Strings(gates)
	return gates, nil
}

// SplitList parses a comma-separated gate list: whitespace-trimmed, empty
// items dropped, duplicates removed, order preserved.
func SplitList(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func in(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func minus(all, remove []string) []string {
	var out []string
	for _, g := range all {
		if !in(remove, g) {
			out = append(out, g)
		}
	}
	return out
}

// successFor returns the ledger's success entry for gate at (sha, tag).
func (l Ledger) successFor(gate, sha, tag string) (Entry, bool) {
	for _, e := range l {
		if e.Gate == gate && e.SHA == sha && e.ReleaseTag == tag && e.Conclusion == ConclusionSuccess {
			return e, true
		}
	}
	return Entry{}, false
}

// carriedSuccess is the success entry the planner already vetted for gate. It
// matches on the gate alone because the carried entry's own sha is the commit
// that earned it -- at tag time that is the validated commit, not the tag's.
func (l Ledger) carriedSuccess(gate string) (Entry, bool) {
	for _, e := range l {
		if e.Gate == gate && e.Conclusion == ConclusionSuccess {
			return e, true
		}
	}
	return Entry{}, false
}

// origin is the run that actually earned an entry's success.
func (e Entry) origin() string {
	if e.CarriedFromRun != "" {
		return e.CarriedFromRun
	}
	return e.RunID
}

// BuildInput is what the composer's `results` job knows.
type BuildInput struct {
	Gates   []string          // ComposedGates
	Results map[string]string // needs.<job>.result for every needed job
	SHA     string            // the commit the battery ran against
	Tag     string
	RunID   string
	Skip    []string // gates the composer was told not to run (skip_gates)
	Carried Ledger   // the success entries those skipped gates stand on
}

// BuildLedger records every composed gate. A gate that ran records its own
// result. A gate in Skip records the carried success (sha/tag taken from the
// carried entry, never from the current run, so the ledger can never claim a
// commit it did not earn). A gate that did not run for any other reason (release_tier
// off, a failed dependency) records "skipped" -- which no later carry or reuse
// ever accepts.
func BuildLedger(b BuildInput) (Ledger, error) {
	if b.SHA == "" || b.Tag == "" || b.RunID == "" {
		return nil, fmt.Errorf("sha, release tag and run id are all required")
	}
	var out Ledger
	for _, g := range b.Gates {
		res, ok := b.Results[g]
		if !ok {
			return nil, fmt.Errorf("no result for gate %q in the composer's needs (the results job must need every gate)", g)
		}
		if in(b.Skip, g) && res == ConclusionSkipped {
			if c, ok := b.Carried.carriedSuccess(g); ok {
				out = append(out, Entry{SHA: c.SHA, ReleaseTag: c.ReleaseTag, Gate: g, Conclusion: ConclusionSuccess, RunID: b.RunID, CarriedFromRun: c.origin()})
				continue
			}
		}
		out = append(out, Entry{SHA: b.SHA, ReleaseTag: b.Tag, Gate: g, Conclusion: res, RunID: b.RunID})
	}
	return out, nil
}

// RerunInput is a release.yml dispatch with `rerun_gates` set.
type RerunInput struct {
	Gates     []string // ComposedGates
	Requested []string // rerun_gates; the single word "failed" means "every gate that is not recorded green"
	Prior     Ledger
	SHA       string
	Tag       string
}

// RerunPlan is the outcome: which gates run, which are skipped on the strength
// of a carried success, and the carried entries that justify the skips.
type RerunPlan struct {
	Rerun   []string `json:"rerun_gates"`
	Skip    []string `json:"skip_gates"`
	Carried Ledger   `json:"carried"`
	// Added lists gates the operator did not name but that had no recorded
	// success for this commit and tag, so they must run anyway.
	Added []string `json:"added"`
}

// FailedKeyword asks PlanRerun to re-run exactly the gates with no recorded
// success for the commit and tag.
const FailedKeyword = "failed"

// PlanRerun validates the requested gates against the composer, carries
// forward every other gate that has a recorded success for exactly (SHA, Tag),
// and forces any other gate to run. It never carries a result it cannot prove.
func PlanRerun(r RerunInput) (RerunPlan, error) {
	if len(r.Requested) == 0 {
		return RerunPlan{}, fmt.Errorf("rerun_gates is empty")
	}
	if r.SHA == "" || r.Tag == "" {
		return RerunPlan{}, fmt.Errorf("sha and release tag are required")
	}
	failedOnly := len(r.Requested) == 1 && r.Requested[0] == FailedKeyword
	var named []string
	if !failedOnly {
		var unknown []string
		for _, g := range r.Requested {
			if g == FailedKeyword {
				return RerunPlan{}, fmt.Errorf("%q cannot be combined with gate names", FailedKeyword)
			}
			if !in(r.Gates, g) {
				unknown = append(unknown, g)
			}
		}
		if len(unknown) > 0 {
			return RerunPlan{}, fmt.Errorf("unknown gate(s) %s; the composer's gates are: %s", strings.Join(unknown, ", "), strings.Join(r.Gates, ", "))
		}
		named = r.Requested
	}

	plan := RerunPlan{Rerun: []string{}, Skip: []string{}, Carried: Ledger{}, Added: []string{}}
	for _, g := range r.Gates {
		if in(named, g) {
			plan.Rerun = append(plan.Rerun, g)
			continue
		}
		if e, ok := r.Prior.successFor(g, r.SHA, r.Tag); ok {
			plan.Skip = append(plan.Skip, g)
			plan.Carried = append(plan.Carried, e)
			continue
		}
		plan.Rerun = append(plan.Rerun, g)
		if !failedOnly {
			plan.Added = append(plan.Added, g)
		}
	}
	if len(plan.Rerun) == 0 {
		return RerunPlan{}, fmt.Errorf("every gate already has a recorded success for %s at %s: nothing to rerun (dispatch without rerun_gates for a full battery)", r.Tag, r.SHA)
	}
	return plan, nil
}

// ReuseInput describes a tag-time battery: what the pre-tag run recorded and
// what changed between the validated commit and the tag.
type ReuseInput struct {
	Gates        []string
	Prior        Ledger
	Tag          string
	TagSHA       string
	ChangedFiles []string // `git diff --name-only <validated>..<tag>`
}

// ReuseDecision is recorded in the release's readiness record.
type ReuseDecision struct {
	Reuse        bool     `json:"reuse"`
	Reason       string   `json:"reason"`
	ValidatedSHA string   `json:"validated_sha,omitempty"`
	TagSHA       string   `json:"tag_sha"`
	ChangedFiles []string `json:"changed_files"`
	Skip         []string `json:"skip_gates"`
	Retest       []string `json:"retest_gates"`
	Carried      Ledger   `json:"carried"`
}

// ValidatedSHA returns the single commit the ledger's entries cover for tag, or
// "" when the ledger spans none or several (an ambiguous record is not reused).
func (l Ledger) ValidatedSHA(tag string) string {
	sha := ""
	for _, e := range l {
		if e.ReleaseTag != tag {
			continue
		}
		if sha != "" && e.SHA != sha {
			return ""
		}
		sha = e.SHA
	}
	return sha
}

// DecideReuse accepts the pre-tag battery only when every composed gate is a
// recorded success for ONE validated commit and the tree diff to the tag is
// confined to the fixture allowlist. It never errors: any doubt is
// Reuse=false with the reason, and the caller runs the full battery.
func DecideReuse(r ReuseInput) ReuseDecision {
	d := ReuseDecision{TagSHA: r.TagSHA, ChangedFiles: append([]string{}, r.ChangedFiles...), Skip: []string{}, Retest: []string{}, Carried: Ledger{}}
	no := func(format string, a ...any) ReuseDecision {
		d.Reuse = false
		d.Reason = fmt.Sprintf(format, a...)
		return d
	}
	if len(r.Prior) == 0 {
		return no("no pre-tag gate ledger for %s (tag not cut by release.yml, the artifact expired, or it predates the ledger): running the full battery", r.Tag)
	}
	d.ValidatedSHA = r.Prior.ValidatedSHA(r.Tag)
	if d.ValidatedSHA == "" {
		return no("the pre-tag ledger does not cover exactly one commit for %s: running the full battery", r.Tag)
	}
	var missing []string
	for _, g := range r.Gates {
		if _, ok := r.Prior.successFor(g, d.ValidatedSHA, r.Tag); !ok {
			missing = append(missing, g)
		}
	}
	if len(missing) > 0 {
		return no("the pre-tag battery has no recorded success for: %s: running the full battery", strings.Join(missing, ", "))
	}
	if d.ValidatedSHA != r.TagSHA {
		allow := FixtureAllowlist(r.Tag)
		var stray []string
		for _, f := range r.ChangedFiles {
			if !in(allow, path.Clean(f)) {
				stray = append(stray, f)
			}
		}
		if len(r.ChangedFiles) == 0 {
			// Different commits, identical tree (e.g. an empty or merge-only
			// commit): the same tree is the condition.
			d.Reason = "tagged commit has the validated commit's tree"
		} else if len(stray) > 0 {
			return no("the tag differs from the validated commit %s beyond the schema-fixture registration (%s): running the full battery", d.ValidatedSHA, strings.Join(stray, ", "))
		}
	}
	d.Reuse = true
	if d.Reason == "" {
		if d.ValidatedSHA == r.TagSHA {
			d.Reason = "tagged commit is the validated commit"
		} else {
			d.Reason = "tag differs from the validated commit only by the schema-fixture registration (" + strings.Join(r.ChangedFiles, ", ") + ")"
		}
	}
	d.Retest = []string{}
	for _, g := range RetestOnReuse {
		if in(r.Gates, g) {
			d.Retest = append(d.Retest, g)
		}
	}
	d.Skip = minus(r.Gates, d.Retest)
	for _, g := range d.Skip {
		e, _ := r.Prior.successFor(g, d.ValidatedSHA, r.Tag)
		d.Carried = append(d.Carried, e)
	}
	return d
}
