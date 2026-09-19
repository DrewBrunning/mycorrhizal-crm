// Package covgate is the code side of issue #979: codecov.yml sits outside
// CI's own review surface (nothing parses it), so a one-line edit — a
// narrowed target, a widened threshold, a new `ignore:` entry — merges on
// human review alone, and a PR changing only codecov.yml trips no path
// filter that would otherwise run the suites whose coverage it governs.
//
// This package gives that file two mechanical checks, both wired by
// cmd/codecovcheck into the unconditional `docs-citations` CI job (like
// citecheck/depexceptions/docscheck — .github/filters.yaml maps
// codecov.yml to nothing, so an unconditional job is the only one a
// codecov.yml-only PR is guaranteed to run):
//
//  1. The `coverage.status.patch.*` targets/thresholds/flags/only_pulls in
//     codecov.yml must match, byte-for-byte after YAML parsing, the fenced
//     example block in docs/development/coverage.md (between the
//     `codecov-patch-status` markers). A target change that isn't also a
//     doc change — the exact "merges on human review alone" shape from the
//     finding — fails.
//  2. Every entry in codecov.yml's `ignore:` list must carry its own
//     justifying `#` comment (immediately above it, or trailing on the same
//     line) -- a bare, uncommented new entry fails, and so does one merely
//     appended after an existing justified entry with no comment of its
//     own. The "with a justifying comment" rule
//     docs/development/coverage.md's Override path already states in
//     prose, made mechanical.
package covgate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// PatchArea is one area's status block under coverage.status.patch in
// codecov.yml (or the equivalent fenced example in coverage.md). Target and
// Threshold are kept as strings ("95%") rather than parsed percentages —
// the comparison is textual, matching whatever the two files literally say.
type PatchArea struct {
	Target    string   `yaml:"target"`
	Threshold string   `yaml:"threshold"`
	Flags     []string `yaml:"flags"`
	OnlyPulls bool     `yaml:"only_pulls"`
}

type codecovYAML struct {
	Coverage struct {
		Status struct {
			Patch map[string]PatchArea `yaml:"patch"`
		} `yaml:"status"`
	} `yaml:"coverage"`
}

// ParsePatchAreas parses the coverage.status.patch section out of data
// (either a real codecov.yml or an extracted doc fenced block, both share
// the shape) and returns it plus any parse findings. label names the source
// in findings ("codecov.yml" / "docs/development/coverage.md").
func ParsePatchAreas(data []byte, label string) (map[string]PatchArea, []string) {
	var cfg codecovYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, []string{fmt.Sprintf("%s does not parse as YAML: %v", label, err)}
	}
	if len(cfg.Coverage.Status.Patch) == 0 {
		return nil, []string{fmt.Sprintf("%s has no coverage.status.patch entries", label)}
	}
	return cfg.Coverage.Status.Patch, nil
}

// CrossCheckPatchAreas asserts codecov.yml's real patch-status areas are
// exactly the doc's documented ones, field for field.
func CrossCheckPatchAreas(codecovAreas, docAreas map[string]PatchArea) []string {
	var findings []string

	names := map[string]bool{}
	for n := range codecovAreas {
		names[n] = true
	}
	for n := range docAreas {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		cv, inCV := codecovAreas[name]
		doc, inDoc := docAreas[name]
		switch {
		case inCV && !inDoc:
			findings = append(findings, fmt.Sprintf("codecov.yml has patch area %q with no matching block in docs/development/coverage.md's example", name))
		case !inCV && inDoc:
			findings = append(findings, fmt.Sprintf("docs/development/coverage.md documents patch area %q, which codecov.yml does not have", name))
		default:
			findings = append(findings, diffPatchArea(name, cv, doc)...)
		}
	}
	return findings
}

func diffPatchArea(name string, cv, doc PatchArea) []string {
	var findings []string
	if cv.Target != doc.Target {
		findings = append(findings, fmt.Sprintf("patch area %q: codecov.yml target %q != docs/development/coverage.md target %q", name, cv.Target, doc.Target))
	}
	if cv.Threshold != doc.Threshold {
		findings = append(findings, fmt.Sprintf("patch area %q: codecov.yml threshold %q != docs/development/coverage.md threshold %q", name, cv.Threshold, doc.Threshold))
	}
	if cv.OnlyPulls != doc.OnlyPulls {
		findings = append(findings, fmt.Sprintf("patch area %q: codecov.yml only_pulls=%v != docs/development/coverage.md only_pulls=%v", name, cv.OnlyPulls, doc.OnlyPulls))
	}
	if !equalStringSlices(cv.Flags, doc.Flags) {
		findings = append(findings, fmt.Sprintf("patch area %q: codecov.yml flags %v != docs/development/coverage.md flags %v", name, cv.Flags, doc.Flags))
	}
	return findings
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const (
	docBeginMarker = "<!-- codecov-patch-status:begin -->"
	docEndMarker   = "<!-- codecov-patch-status:end -->"
)

var fencedYAMLRE = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")

// ExtractDocPatchStatusBlock pulls the fenced ```yaml block between the
// codecov-patch-status markers out of docs/development/coverage.md.
func ExtractDocPatchStatusBlock(doc string) (string, []string) {
	start := strings.Index(doc, docBeginMarker)
	end := strings.Index(doc, docEndMarker)
	if start == -1 || end == -1 || end < start {
		return "", []string{fmt.Sprintf("docs/development/coverage.md is missing the %s / %s markers", docBeginMarker, docEndMarker)}
	}
	block := doc[start+len(docBeginMarker) : end]

	m := fencedYAMLRE.FindStringSubmatch(block)
	if m == nil {
		return "", []string{"docs/development/coverage.md's codecov-patch-status block has no fenced ```yaml example"}
	}
	return m[1], nil
}

// CheckIgnoreEntriesJustified asserts every entry in codecov.yml's top-level
// ignore: list carries its own justifying comment -- a `#` comment
// immediately above it, or trailing on the same line. Deliberately requires
// an OWN comment per entry rather than letting one comment justify a whole
// run of entries below it: a shared block would let a bare new entry ride
// along silently just by being appended right after an already-justified
// group, which is the exact "one-line ignore: [...] merges on human review
// alone" shape from issue #979 -- this file does not, in general, come with
// a diff a reviewer reads line by line the way most code does. Every entry
// in codecov.yml as of #979 was reformatted to a one-entry-one-comment shape
// (matching .trivyignore / citation-drift.ignore's established convention)
// specifically so this rule holds with no false positives.
func CheckIgnoreEntriesJustified(data []byte) []string {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return []string{fmt.Sprintf("codecov.yml does not parse as YAML: %v", err)}
	}
	if len(root.Content) == 0 {
		return []string{"codecov.yml is empty"}
	}

	seq := findIgnoreSequence(root.Content[0])
	if seq == nil {
		// No ignore: list at all is fine -- nothing to justify.
		return nil
	}

	var findings []string
	for _, item := range seq.Content {
		hasOwnComment := strings.TrimSpace(item.HeadComment) != "" || strings.TrimSpace(item.LineComment) != ""
		if !hasOwnComment {
			findings = append(findings, fmt.Sprintf(
				"codecov.yml ignore: entry %q (line %d) has no justifying comment -- add a `#` comment (above it or trailing) explaining why this file is excluded from the coverage gate",
				item.Value, item.Line))
		}
	}
	return findings
}

// stubWorkflow is the shape of .github/workflows/unit-tests.yml that
// ExtractStubAreas reads: only the codecov-patch-stub job's step env matters.
type stubWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Env map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// ExtractStubAreas pulls the PATCH_AREAS env of the `codecov-patch-stub`
// job's status-posting step out of .github/workflows/unit-tests.yml. That
// job (issue #1188) posts a success status for every area Codecov will not
// measure on a PR that uploads no coverage; its list must equal codecov.yml's
// coverage.status.patch keys, so a new patch area can't be added without also
// arming the stub (and vice versa). The second return is any finding.
func ExtractStubAreas(workflow []byte) ([]string, []string) {
	var wf stubWorkflow
	if err := yaml.Unmarshal(workflow, &wf); err != nil {
		return nil, []string{fmt.Sprintf(".github/workflows/unit-tests.yml does not parse as YAML: %v", err)}
	}
	job, ok := wf.Jobs["codecov-patch-stub"]
	if !ok {
		return nil, []string{".github/workflows/unit-tests.yml has no jobs.codecov-patch-stub job (issue #1188)"}
	}
	for _, step := range job.Steps {
		if areas, ok := step.Env["PATCH_AREAS"]; ok {
			fields := strings.Fields(areas)
			if len(fields) == 0 {
				return nil, []string{"codecov-patch-stub's PATCH_AREAS env is empty (issue #1188)"}
			}
			return fields, nil
		}
	}
	return nil, []string{"codecov-patch-stub has no step with a PATCH_AREAS env (issue #1188)"}
}

// CrossCheckStubAreas asserts the codecov-patch-stub job posts a status for
// exactly codecov.yml's patch areas. A codecov.yml area missing from the stub
// is the bug this guards: that area's required context would be stranded on a
// PR with no upload. An area listed in the stub but not in codecov.yml is the
// reverse drift.
func CrossCheckStubAreas(codecovAreas map[string]PatchArea, stubAreas []string) []string {
	var findings []string
	stub := map[string]bool{}
	for _, a := range stubAreas {
		if stub[a] {
			findings = append(findings, fmt.Sprintf("codecov-patch-stub PATCH_AREAS lists %q twice (issue #1188)", a))
		}
		stub[a] = true
	}

	names := map[string]bool{}
	for n := range codecovAreas {
		names[n] = true
	}
	for n := range stub {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, n := range sorted {
		_, inCV := codecovAreas[n]
		_, inStub := stub[n]
		switch {
		case inCV && !inStub:
			findings = append(findings, fmt.Sprintf("codecov.yml has patch area %q but codecov-patch-stub's PATCH_AREAS does not list it -- codecov/patch/%s would be stranded on a PR that uploads no coverage (issue #1188)", n, n))
		case !inCV && inStub:
			findings = append(findings, fmt.Sprintf("codecov-patch-stub's PATCH_AREAS lists %q, which codecov.yml has no patch area for (issue #1188)", n))
		}
	}
	return findings
}

// findIgnoreSequence walks a top-level mapping node for the "ignore" key and
// returns its sequence value node, or nil if absent.
func findIgnoreSequence(mapping *yaml.Node) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		val := mapping.Content[i+1]
		if key.Value == "ignore" && val.Kind == yaml.SequenceNode {
			return val
		}
	}
	return nil
}
