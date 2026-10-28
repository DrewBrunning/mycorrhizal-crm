// Package versionpolicy is the code side of REL-01, the versioning policy
// (issue #445, docs/versioning-policy.md).
//
// It exists so the release-tag format has exactly one authoritative
// definition. TagPattern below is that definition; the copies embedded in
// .github/workflows/release.yml and .github/workflows/docker-publish.yml, and
// the copy quoted in docs/versioning-policy.md, are asserted identical to it by
// TestWorkflowsAndDocUseCanonicalPattern. Change the pattern here and the drift
// test tells you every place that must change with it — the same shape as
// schemafixture's SupportedUpgradeFloorVersion / TestDocsStateTheFloor.
package versionpolicy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TagPattern is the canonical release-tag format: `vMAJOR.MINOR.PATCH` with an
// optional `-rc.N` pre-release suffix and nothing else. It is an anchored ERE
// so the byte-identical string can be dropped into a shell `grep -qE` in a
// workflow.
//
// Examples that match: v0.6.12, v1.0.0, v1.0.0-rc.1, v10.20.30.
// Examples that do not: 1.0.0 (no v), v1.0 (two components),
// v1.0.0-alpha (non-rc suffix), v0.2.0-alpha-candidate (the pre-policy tag).
const TagPattern = `^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$`

var tagRe = regexp.MustCompile(TagPattern)

// ValidReleaseTag reports whether tag is a well-formed release tag under the
// versioning policy. It does not check that the tag exists, that its version is
// higher than the last release, or that MAJOR=1 is intentional — only the
// format.
func ValidReleaseTag(tag string) bool {
	return tagRe.MatchString(tag)
}

// IsReleaseCandidate reports whether tag is a well-formed release-candidate tag
// (`vX.Y.Z-rc.N`). A tag that is not a valid release tag at all is not a
// release candidate.
func IsReleaseCandidate(tag string) bool {
	return ValidReleaseTag(tag) && strings.Contains(tag, "-rc.")
}

var coreRe = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)(?:-rc\.[0-9]+)?$`)

// AtLeast reports whether tag's MAJOR.MINOR.PATCH is at or after
// major.minor.patch. A release candidate is ordered by its core version, so
// v1.6.0-rc.1 counts as v1.6.0: a rule that applies to a release applies to its
// candidates too. It returns an error for a tag that is not a valid release tag,
// so a caller never treats a typo as "before the milestone".
func AtLeast(tag string, major, minor, patch int) (bool, error) {
	m := coreRe.FindStringSubmatch(tag)
	if m == nil {
		return false, fmt.Errorf("%q is not a valid release tag (want vMAJOR.MINOR.PATCH with an optional -rc.N)", tag)
	}
	var got [3]int
	for i := range got {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return false, fmt.Errorf("%q: %w", tag, err)
		}
		got[i] = n
	}
	want := [3]int{major, minor, patch}
	for i := range got {
		if got[i] != want[i] {
			return got[i] > want[i], nil
		}
	}
	return true, nil
}

// AtLeastTag is AtLeast with the floor given as a release tag, so a caller can
// keep the milestone in one constant instead of three integers.
func AtLeastTag(tag, floor string) (bool, error) {
	m := coreRe.FindStringSubmatch(floor)
	if m == nil {
		return false, fmt.Errorf("floor %q is not a valid release tag", floor)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return AtLeast(tag, major, minor, patch)
}
