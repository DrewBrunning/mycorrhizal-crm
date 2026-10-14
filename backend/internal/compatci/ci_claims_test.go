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
// e.g. "Chrome: tested in CI" -> "Chrome".
var testedInCIPattern = regexp.MustCompile(`([A-Za-z][A-Za-z/ ]*[A-Za-z]): tested in CI`)

// advisoryCIPattern extracts the vendor list an advisory claim names, e.g.
// "Firefox: advisory only" -> "Firefox". An advisory vendor is downloaded and
// driven in CI, but its assertion step is continue-on-error (issue #1002).
var advisoryCIPattern = regexp.MustCompile(`([A-Za-z][A-Za-z/ ]*[A-Za-z]): advisory only`)

// assertStepPattern matches the browser-minimum assertion step for a vendor.
var assertStepPattern = regexp.MustCompile(`(?m)^\s*- name: Assert the app loads on (Firefox|Chrome|Edge|Safari)\b`)

func splitVendors(s string) map[string]bool {
	out := make(map[string]bool)
	for _, name := range strings.Split(s, "/") {
		out[strings.TrimSpace(name)] = true
	}
	return out
}

// advisorySteps returns, per vendor, whether its assertion step carries
// continue-on-error: true. A step block runs from its "- name:" line to the
// next "- name:" line.
func advisorySteps(wf string) map[string]bool {
	out := make(map[string]bool)
	locs := assertStepPattern.FindAllStringSubmatchIndex(wf, -1)
	for _, loc := range locs {
		rest := wf[loc[1]:]
		end := len(rest)
		if i := strings.Index(rest, "\n      - name:"); i >= 0 {
			end = i
		}
		out[wf[loc[2]:loc[3]]] = strings.Contains(rest[:end], "continue-on-error: true")
	}
	return out
}

// checkBrowserClaims cross-checks the doc's tested/advisory claims against the
// workflow; it returns one message per mismatch.
func checkBrowserClaims(wf, doc string) []string {
	var errs []string
	downloaded := make(map[string]bool)
	for _, m := range browserDownloadPattern.FindAllStringSubmatch(wf, -1) {
		downloaded[m[1]] = true
	}
	tested, advisory := map[string]bool{}, map[string]bool{}
	if m := testedInCIPattern.FindStringSubmatch(doc); m != nil {
		tested = splitVendors(m[1])
	}
	if m := advisoryCIPattern.FindStringSubmatch(doc); m != nil {
		advisory = splitVendors(m[1])
	}
	if len(tested) == 0 && len(advisory) == 0 {
		return []string{"no \"<vendors>: tested in CI\" or \"<vendors>: advisory only\" claim in the Web browser row"}
	}
	blocking := advisorySteps(wf)
	for name := range tested {
		switch {
		case !downloaded[name]:
			errs = append(errs, name+" claimed \"tested in CI\" but has no Download step")
		case blocking[name]:
			errs = append(errs, name+" claimed \"tested in CI\" but its assertion step is continue-on-error (advisory)")
		}
	}
	for name := range advisory {
		switch {
		case tested[name]:
			errs = append(errs, name+" claimed both tested and advisory")
		case !downloaded[name]:
			errs = append(errs, name+" claimed \"advisory only\" but has no Download step")
		case !blocking[name]:
			errs = append(errs, name+" claimed \"advisory only\" but its assertion step is blocking (no continue-on-error: true)")
		}
	}
	for name := range downloaded {
		if !tested[name] && !advisory[name] {
			errs = append(errs, name+" is downloaded and driven in CI but the Web browser row claims neither \"tested in CI\" nor \"advisory only\"")
		}
	}
	return errs
}

func TestCheckBrowserClaims(t *testing.T) {
	step := func(v string, coe bool) string {
		s := "      - name: Download " + v + " 1\n        run: x\n      - name: Assert the app loads on " + v + " 1\n"
		if coe {
			s += "        continue-on-error: true\n"
		}
		return s + "        run: y\n"
	}
	wf := step("Firefox", true) + step("Chrome", false)
	cases := []struct {
		name, wf, doc string
		want          int
	}{
		{"ok", wf, "Chrome: tested in CI. Firefox: advisory only", 0},
		{"advisory called tested", wf, "Chrome/Firefox: tested in CI", 1},
		{"blocking called advisory", step("Firefox", false) + step("Chrome", false), "Chrome: tested in CI. Firefox: advisory only", 1},
		{"unclaimed download", wf, "Chrome: tested in CI", 1},
		{"claimed not downloaded", wf, "Chrome/Edge: tested in CI. Firefox: advisory only", 1},
		{"no claim", wf, "nothing", 1},
	}
	for _, c := range cases {
		if got := checkBrowserClaims(c.wf, c.doc); len(got) != c.want {
			t.Errorf("%s: got %v, want %d errors", c.name, got, c.want)
		}
	}
}

// TestSupportedVersionsBrowserCIClaimMatchesRealDownloads is the issue #993
// regression guard for finding G8, extended by issue #1002 with an advisory
// tier: every vendor min-version-tests.yml downloads must be claimed in
// docs/supported-versions.md's Web browser row as "tested in CI" (assertion
// step blocking) or "advisory only" (assertion step continue-on-error), and
// every claimed vendor must actually be downloaded.
func TestSupportedVersionsBrowserCIClaimMatchesRealDownloads(t *testing.T) {
	wfPath := filepath.Join(workflowsDirRel, "min-version-tests.yml")
	wf, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("reading %s: %v", wfPath, err)
	}
	if !browserDownloadPattern.Match(wf) {
		t.Fatalf("found no \"Download <Browser>\" steps in %s's browser-minimum job -- did the step "+
			"naming change? update browserDownloadPattern in %s", wfPath, "ci_claims_test.go")
	}
	if !assertStepPattern.Match(wf) {
		t.Fatalf("found no \"Assert the app loads on <Browser>\" steps in %s -- update assertStepPattern", wfPath)
	}
	const docPath = "../../../docs/supported-versions.md"
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading %s: %v", docPath, err)
	}
	for _, e := range checkBrowserClaims(string(wf), string(doc)) {
		t.Errorf("%s vs %s: %s -- fix the doc or the workflow", docPath, wfPath, e)
	}
}
