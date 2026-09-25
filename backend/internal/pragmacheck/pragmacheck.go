// Package pragmacheck is the structural gate for coverage-override markers
// (docs/development/coverage.md's "Override path"): every
// `// # pragma: no cover` on non-test Go under backend/, and every
// `/* v8 ignore ... */` under frontend/src, must carry a discoverable reason.
//
// docs/development/coverage.md already asks in prose for a reason on every
// override, but nothing enforced it: roughly a tenth of the ~600 Go markers
// in the repo carried no reason at all when this check was written. A marker
// with no reason is invisible in review and unfalsifiable later -- nobody can
// tell, six months on, whether the line is still genuinely untestable or
// whether the guard it once excused has rotted.
//
// # What counts as a reason
//
//  1. Non-trivial text after the marker on the same line (after stripping the
//     marker itself and leading punctuation/dashes/whitespace), OR
//  2. The immediately preceding non-blank source line carries its own
//     non-trivial trailing (or whole-line) comment -- the common repo pattern
//     of stating the reason once on a guarding `if` line and marking every
//     line inside the block, or stating it on the line just above a bare
//     `// # pragma: no cover`.
//
// "Non-trivial" means at least a few non-whitespace characters remain after
// stripping marker syntax and leading punctuation -- enough to rule out a
// pragma with genuinely nothing next to it, without policing prose quality.
package pragmacheck

import (
	"regexp"
	"strings"
)

// goMarker matches a Go no-cover pragma comment and captures any trailing
// text after it on the same line. It requires the marker to open a `//`
// comment (only whitespace between `//` and `#`) so a doc comment that
// merely *discusses* the convention in prose -- e.g. quoting
// “ `# pragma: no cover` “ inside a sentence -- is not mistaken for an
// applied marker.
var goMarker = regexp.MustCompile(`//\s*#\s*pragma:\s*no cover(.*)$`)

// tsMarker matches a frontend v8-ignore block comment and captures any
// trailing text (v8's own convention is `-- reason` after the directive).
var tsMarker = regexp.MustCompile(`/\*\s*v8 ignore\s+(?:next|start|stop|file)\b(.*?)\*/`)

// trailingCommentRE pulls the text of a `//`-delimited trailing (or
// whole-line) comment off the end of a Go source line.
var trailingCommentRE = regexp.MustCompile(`//(.*)$`)

// Finding is one marker with no discoverable reason.
type Finding struct {
	Path string
	Line int    // 1-based
	Text string // the full source line, for the report
}

// reasonRE strips the reason down to alphanumeric content so a marker
// consisting only of punctuation ("--", "—", ":") is treated as bare.
var reasonRE = regexp.MustCompile(`[A-Za-z0-9]`)

// hasReason reports whether s (text found after a marker, or a whole
// candidate comment) contains a non-trivial reason: at least 3
// alphanumeric characters once marker/pragma syntax is stripped.
func hasReason(s string) bool {
	// A reason that is itself just another bare pragma re-statement doesn't
	// count -- strip a nested marker occurrence before judging length.
	s = goMarker.ReplaceAllString(s, "$1")
	letters := reasonRE.FindAllString(s, -1)
	return len(letters) >= 3
}

// precedingComment returns the trailing (or whole-line) `//` comment text of
// the last non-blank line before index i in lines (0-based), or "" if that
// line has no comment.
func precedingComment(lines []string, i int) string {
	for j := i - 1; j >= 0; j-- {
		line := strings.TrimSpace(lines[j])
		if line == "" {
			continue
		}
		m := trailingCommentRE.FindStringSubmatch(line)
		if m == nil {
			return ""
		}
		return m[1]
	}
	return ""
}

// CheckGoSource scans a single non-test Go file's content for no-cover
// pragmas lacking a discoverable reason. path is used only to label findings.
func CheckGoSource(path, content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := goMarker.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if hasReason(m[1]) {
			continue
		}
		if hasReason(precedingComment(lines, i)) {
			continue
		}
		findings = append(findings, Finding{Path: path, Line: i + 1, Text: strings.TrimRight(line, "\r")})
	}
	return findings
}

// CheckTSSource scans a single frontend source file's content for v8-ignore
// markers lacking a discoverable reason.
func CheckTSSource(path, content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := tsMarker.FindAllStringSubmatch(line, -1)
		if matches == nil {
			continue
		}
		for _, m := range matches {
			if hasReason(m[1]) {
				continue
			}
			if hasReason(precedingComment(lines, i)) {
				continue
			}
			findings = append(findings, Finding{Path: path, Line: i + 1, Text: strings.TrimRight(line, "\r")})
		}
	}
	return findings
}
