// Structural guards against a specific stale-claim bug class (issue #993):
// a source comment or operator doc naming a specific CI runner
// (an Android emulator API level, a browser vendor) as covered when
// android-tests.yml / min-version-tests.yml don't actually run it. Both
// findings shipped from the same author assumption drifting from the real
// CI config; these tests parse the workflows as ground truth and fail if a
// claim disagrees with them, the same shape as compatCoverage's
// doc-vs-workflow check above.
package compatci

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// androidTreeRel is the Android source tree scanned for stale
// "API N CI emulator" claims, relative to this package.
const androidTreeRel = "../../../android"

// apiLevelPattern matches android-tests.yml's
// `reactivecircus/android-emulator-runner` `api-level:` input -- the ground
// truth for which Android API levels any CI job actually boots an emulator
// at.
var apiLevelPattern = regexp.MustCompile(`(?m)^\s*api-level:\s*(\d+)`)

// ciEmulatorClaimPattern matches a source comment claiming a specific
// Android API level is "the CI emulator" -- e.g. the false
// "the API 37 CI emulator" this test exists to catch (issue #993: no CI
// emulator has ever run API 37; android-tests.yml pins 24/26/35).
var ciEmulatorClaimPattern = regexp.MustCompile(`API (\d+) CI emulator`)

// skipAndroidDirs are generated/build output, not source -- walking them
// wastes time and can spuriously match generated text.
var skipAndroidDirs = map[string]bool{
	"build":   true,
	".gradle": true,
	".idea":   true,
}

// androidEmulatorAPILevels parses .github/workflows/android-tests.yml and
// returns the set of Android API levels (as strings, matching the
// workflow's own formatting) any job in it actually boots an emulator at.
func androidEmulatorAPILevels(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join(workflowsDirRel, "android-tests.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	levels := make(map[string]bool)
	for _, m := range apiLevelPattern.FindAllStringSubmatch(string(data), -1) {
		levels[m[1]] = true
	}
	return levels
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestAndroidCIEmulatorClaimsAreReal is the issue #993 regression guard for
// finding G7: a comment naming "the API N CI emulator" where N is not an
// API level android-tests.yml actually configures. It would have failed on
// the original bug (E2eBaseTest.kt and app/build.gradle.kts both claimed
// "the API 37 CI emulator" when CI has only ever run 24/26/35) and fails
// again on any future reintroduction of the same false-claim pattern,
// anywhere in the Android tree -- not just the two files that had it.
func TestAndroidCIEmulatorClaimsAreReal(t *testing.T) {
	real := androidEmulatorAPILevels(t)
	if len(real) == 0 {
		t.Fatalf("found no `api-level:` entries in android-tests.yml -- did the workflow format change? "+
			"update apiLevelPattern in %s", "ci_claims_test.go")
	}

	err := filepath.WalkDir(androidTreeRel, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipAndroidDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".kt") && !strings.HasSuffix(path, ".kts") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range ciEmulatorClaimPattern.FindAllStringSubmatch(string(data), -1) {
			level := m[1]
			if !real[level] {
				t.Errorf("%s claims \"API %s CI emulator\", but android-tests.yml's real emulator "+
					"API levels are %v -- fix the comment (or the workflow, if a new emulator level "+
					"was genuinely added)", path, level, sortedKeys(real))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", androidTreeRel, err)
	}
}

// browserDownloadPattern matches a min-version-tests.yml step name that
// downloads a specific browser's binary in the `browser-minimum` job --
// e.g. "Download Firefox 111.0 (the documented floor) + a compatible
// geckodriver" or "Download Chrome for Testing 115 (...) + matching
// chromedriver" -- the ground truth for which browsers are actually
// fetched and driven in CI.
var browserDownloadPattern = regexp.MustCompile(`(?m)^\s*- name: Download (Firefox|Chrome|Edge|Safari)\b`)

// testedInCIPattern extracts the vendor list a "tested in CI" claim names,
// e.g. "Chrome/Firefox: tested in CI" -> "Chrome/Firefox".
var testedInCIPattern = regexp.MustCompile(`([A-Za-z][A-Za-z/ ]*[A-Za-z]): tested in CI`)

// TestSupportedVersionsBrowserCIClaimMatchesRealDownloads is the issue #993
// regression guard for finding G8: docs/supported-versions.md's Browser row
// claimed Edge was "tested in CI" alongside Chrome/Firefox, but
// min-version-tests.yml's browser-minimum job never downloads an Edge
// binary -- only Firefox and Chrome. This cross-checks the doc's
// "<vendors>: tested in CI" claim against the workflow's real "Download
// <vendor>" steps in both directions, so it fails on an overclaim (a vendor
// the doc says is tested but CI doesn't download) and on a stale doc (a
// vendor CI now downloads that the doc doesn't credit).
func TestSupportedVersionsBrowserCIClaimMatchesRealDownloads(t *testing.T) {
	wfPath := filepath.Join(workflowsDirRel, "min-version-tests.yml")
	wf, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("reading %s: %v", wfPath, err)
	}
	downloaded := make(map[string]bool)
	for _, m := range browserDownloadPattern.FindAllStringSubmatch(string(wf), -1) {
		downloaded[m[1]] = true
	}
	if len(downloaded) == 0 {
		t.Fatalf("found no \"Download <Browser>\" steps in %s's browser-minimum job -- did the step "+
			"naming change? update browserDownloadPattern in %s", wfPath, "ci_claims_test.go")
	}

	const docPath = "../../../docs/supported-versions.md"
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading %s: %v", docPath, err)
	}
	m := testedInCIPattern.FindStringSubmatch(string(doc))
	if m == nil {
		t.Fatalf("%s: found no \"<vendors>: tested in CI\" claim in the Web browser row -- did the "+
			"wording change? update testedInCIPattern in %s", docPath, "ci_claims_test.go")
	}
	claimed := make(map[string]bool)
	for _, name := range strings.Split(m[1], "/") {
		claimed[strings.TrimSpace(name)] = true
	}

	for name := range claimed {
		if !downloaded[name] {
			t.Errorf("%s claims %q is \"tested in CI\", but %s's browser-minimum job has no "+
				"\"Download %s\" step -- fix the doc, or add real CI coverage for it", docPath, name, wfPath, name)
		}
	}
	for name := range downloaded {
		if !claimed[name] {
			t.Errorf("%s's browser-minimum job downloads and drives %s, but %s's Web browser row "+
				"doesn't claim it's tested in CI -- update the doc", wfPath, name, docPath)
		}
	}
}
