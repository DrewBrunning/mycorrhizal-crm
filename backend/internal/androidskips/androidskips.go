// Package androidskips is the allowlist gate for the Android instrumented
// E2E suite's skipped tests (issue #1483).
//
// `Android E2E (emulator)` is a required, release-gating check, but a JUnit
// `Assume` failure is reported as a *skip*, and a skip reports green. A test
// that silently stops running therefore leaves the gate green. This package
// compares the set of skipped tests in AGP's connected-test JUnit XML against a
// committed allowlist (android/e2e-expected-skips.txt), one `class#method |
// reason` entry per line, and reports every difference in either direction:
//
//   - a skipped test that is not allowlisted (a new, unreviewed skip);
//   - an allowlisted test that did not skip (the skip went away -- delete the
//     entry so the gate cannot quietly regress later) or did not run at all
//     (renamed/deleted, or the suite did not execute it);
//   - a run with no test cases at all (nothing executed is not a pass).
//
// It is the Android counterpart of internal/citest.SkipOrRequire.
package androidskips

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Allowlist maps "fully.qualified.Class#method" to the reason it is expected
// to skip on the CI emulator.
type Allowlist map[string]string

var entryRe = regexp.MustCompile(`^([A-Za-z_][\w.$]*)#([A-Za-z_][\w$]*)$`)

// ParseAllowlist reads the allowlist file format: blank lines and `#`
// comments are ignored; every other line is `class#method | reason` with a
// non-empty reason. Returns the entries plus a finding per malformed or
// duplicate line.
func ParseAllowlist(r io.Reader) (Allowlist, []string) {
	allow := Allowlist{}
	var findings []string
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, reason, ok := strings.Cut(line, "|")
		key, reason = strings.TrimSpace(key), strings.TrimSpace(reason)
		switch {
		case !ok || reason == "":
			findings = append(findings, fmt.Sprintf("allowlist line %d: %q has no `| reason` (every expected skip must say why)", n, line))
		case !entryRe.MatchString(key):
			findings = append(findings, fmt.Sprintf("allowlist line %d: %q is not `fully.qualified.Class#method`", n, key))
		default:
			if _, dup := allow[key]; dup {
				findings = append(findings, fmt.Sprintf("allowlist line %d: duplicate entry %s", n, key))
				continue
			}
			allow[key] = reason
		}
	}
	if err := sc.Err(); err != nil {
		findings = append(findings, "allowlist read error: "+err.Error()) // # pragma: no cover — a bufio read error on an already-open in-memory/file reader
	}
	return allow, findings
}

// Results is what one connected-test run reported.
type Results struct {
	Total   int
	Passed  int
	Failed  int
	Skipped map[string]bool // "class#method" -> skipped
	Seen    map[string]bool // every "class#method" that appeared
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string    `xml:"name,attr"`
	ClassName string    `xml:"classname,attr"`
	Skipped   *struct{} `xml:"skipped"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
}

// ParseResults merges the JUnit XML documents (each a <testsuite> or a
// <testsuites>) into one Results. A document that is not JUnit XML is an error.
func ParseResults(docs map[string][]byte) (*Results, error) {
	res := &Results{Skipped: map[string]bool{}, Seen: map[string]bool{}}
	names := make([]string, 0, len(docs))
	for n := range docs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		suites, err := decode(docs[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, s := range suites {
			for _, c := range s.Cases {
				key := c.ClassName + "#" + c.Name
				res.Total++
				res.Seen[key] = true
				switch {
				case c.Failure != nil || c.Error != nil:
					res.Failed++
				case c.Skipped != nil:
					res.Skipped[key] = true
				default:
					res.Passed++
				}
			}
		}
	}
	return res, nil
}

func decode(b []byte) ([]junitSuite, error) {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("not JUnit XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "testsuites":
			var s junitSuites
			if err := dec.DecodeElement(&s, &se); err != nil {
				return nil, err
			}
			return s.Suites, nil
		case "testsuite":
			var s junitSuite
			if err := dec.DecodeElement(&s, &se); err != nil {
				return nil, err
			}
			return []junitSuite{s}, nil
		default:
			return nil, fmt.Errorf("not JUnit XML: root element <%s>", se.Name.Local)
		}
	}
}

// Check returns one finding per divergence between the observed skips and the
// allowlist. An empty slice means the run matches the allowlist exactly.
func Check(allow Allowlist, res *Results) []string {
	var findings []string
	if res.Total == 0 {
		return []string{"no test cases found in the results: a run that executed nothing is not a pass"}
	}
	for _, k := range sortedKeys(res.Skipped) {
		if _, ok := allow[k]; !ok {
			findings = append(findings, fmt.Sprintf("UNEXPECTED SKIP: %s was skipped but is not in android/e2e-expected-skips.txt "+
				"(a silently-skipped test leaves the required gate green; fix the cause, or add a reasoned entry)", k))
		}
	}
	for _, k := range sortedAllow(allow) {
		switch {
		case !res.Seen[k]:
			findings = append(findings, fmt.Sprintf("STALE ALLOWLIST ENTRY: %s did not appear in the results (renamed, deleted, or not executed)", k))
		case !res.Skipped[k]:
			findings = append(findings, fmt.Sprintf("UNEXPECTED RUN: %s is allowlisted as skipped but did not skip; delete its entry from android/e2e-expected-skips.txt", k))
		}
	}
	return findings
}

// Summary renders the markdown block appended to $GITHUB_STEP_SUMMARY.
func Summary(allow Allowlist, res *Results, findings []string) string {
	var b strings.Builder
	b.WriteString("### Android E2E skip report\n\n")
	fmt.Fprintf(&b, "%d test(s): %d passed, %d failed, %d skipped (%d expected by `android/e2e-expected-skips.txt`).\n\n",
		res.Total, res.Passed, res.Failed, len(res.Skipped), len(allow))
	if len(res.Skipped) > 0 {
		b.WriteString("| Skipped test | Reason |\n|---|---|\n")
		for _, k := range sortedKeys(res.Skipped) {
			reason, ok := allow[k]
			if !ok {
				reason = "**NOT ALLOWLISTED**"
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", k, strings.ReplaceAll(reason, "|", "\\|"))
		}
		b.WriteString("\n")
	}
	if len(findings) == 0 {
		b.WriteString("Skips match the allowlist exactly.\n")
	} else {
		b.WriteString("**Skip gate failed:**\n\n")
		for _, f := range findings {
			b.WriteString("- " + f + "\n")
		}
	}
	return b.String()
}

// CheckSources verifies every allowlist entry resolves to a real test: a
// `<Class>.kt` file under androidTestRoot (matching the package path) that
// declares `fun <method>`. It is the static half of the gate -- it runs on
// every PR (the emulator suite does not), so a rename that orphans an entry
// fails before the post-merge run does.
func CheckSources(allow Allowlist, androidTestRoot string) []string {
	var findings []string
	for _, k := range sortedAllow(allow) {
		class, method, _ := strings.Cut(k, "#")
		pkgPath := filepath.FromSlash(strings.ReplaceAll(class, ".", "/"))
		var src []byte
		var err error
		// Two candidate layouts: <root>/<pkg>/<Class>.kt and <root>/kotlin/<pkg>/<Class>.kt.
		for _, cand := range []string{
			filepath.Join(androidTestRoot, pkgPath+".kt"),
			filepath.Join(androidTestRoot, "kotlin", pkgPath+".kt"),
		} {
			// #nosec G304 -- cand is derived from the committed allowlist and the fixed androidTest root
			if src, err = os.ReadFile(cand); err == nil {
				break
			}
		}
		if err != nil {
			findings = append(findings, fmt.Sprintf("allowlist entry %s: no %s.kt under %s", k, pkgPath, androidTestRoot))
			continue
		}
		if !regexp.MustCompile(`\bfun\s+` + regexp.QuoteMeta(method) + `\s*\(`).Match(src) {
			findings = append(findings, fmt.Sprintf("allowlist entry %s: %s.kt declares no `fun %s(`", k, pkgPath, method))
		}
	}
	return findings
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedAllow(m Allowlist) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
