// Package reasonprose guards against free prose in error reason params
// (issue #2920, item 2). A reason param is a value substituted into a
// localized template; when the value is itself an English sentence, a client
// in any other language still shows English (the #2912 failure). The
// failure-mode suites feed every reason_params object they record through
// Check.
//
// The existing offenders are a counted allowlist in prose_allowlist.txt, one
// line per code/param (or key for values that are never prose) with a written
// reason. A new prose value fails; an allowlist entry is removed only when the
// offender is fixed.
package reasonprose

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// proseWordThreshold is the point at which a param value reads as a sentence
// rather than a field name or user value. A field label like "Work email
// address" is data the template interpolates; "The Meerkat instance could not
// be reached" is prose that cannot be translated.
const proseWordThreshold = 4

//go:embed prose_allowlist.txt
var allowlistText string

// Problem is one reason param that looks like free prose.
type Problem struct {
	Code  string
	Key   string
	Value string
	Why   string
}

type allowlist struct {
	reasons map[string]string // "code/key" or "*/key" -> reason
	count   int
}

var (
	loadOnce  sync.Once
	loaded    allowlist
	loadError error
)

// Check returns every reason param value that reads as free prose and is not
// covered by the counted allowlist. It ignores non-string values (numbers,
// booleans and sizes never contain sentences).
func Check(code string, params map[string]any) []Problem {
	if len(params) == 0 {
		return nil
	}
	allow, err := loadAllowlist()
	var problems []Problem
	if err != nil {
		// Fail closed: a malformed allowlist must not silently excuse prose.
		problems = append(problems, Problem{
			Code: code,
			Why:  "reason prose allowlist is malformed: " + err.Error(),
		})
	}
	for key, raw := range params {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if len(strings.Fields(value)) < proseWordThreshold {
			continue
		}
		if _, ok := allow.reasons[code+"/"+key]; ok {
			continue
		}
		if _, ok := allow.reasons["*/"+key]; ok {
			continue
		}
		problems = append(problems, Problem{
			Code:  code,
			Key:   key,
			Value: value,
			Why:   fmt.Sprintf("%d or more words read as prose that a translated template cannot interpolate; use a localizable reason code/template or add a reasoned allowlist entry", proseWordThreshold),
		})
	}
	sortProblems(problems)
	return problems
}

// sortProblems orders problems by key, then value, so Check output is stable.
func sortProblems(problems []Problem) {
	sort.Slice(problems, func(i, j int) bool {
		if problems[i].Key != problems[j].Key {
			return problems[i].Key < problems[j].Key
		}
		return problems[i].Value < problems[j].Value
	})
}

func loadAllowlist() (allowlist, error) {
	loadOnce.Do(func() {
		loaded, loadError = parseAllowlist(allowlistText)
	})
	return loaded, loadError
}

// parseAllowlist reads the counted allowlist. The format is a `# count: N`
// header (matching the number of entries), then one `code/key<TAB>reason` or
// `*/key<TAB>reason` line per entry. Blank lines and other `#` comments are
// ignored. A missing reason is an error: an allowlist entry without a reason
// is a silent weakening of the gate.
func parseAllowlist(text string) (allowlist, error) {
	allow := allowlist{reasons: map[string]string{}}
	sawCount := false
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			rest, ok := strings.CutPrefix(line, "# count:")
			if !ok {
				continue
			}
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(rest), "%d", &n); err != nil {
				return allowlist{}, fmt.Errorf("line %d: bad count header %q", i+1, line)
			}
			allow.count = n
			sawCount = true
			continue
		}
		key, reason, ok := strings.Cut(raw, "\t")
		if !ok {
			return allowlist{}, fmt.Errorf("line %d: want code/key<TAB>reason, got %q", i+1, line)
		}
		key = strings.TrimSpace(key)
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return allowlist{}, fmt.Errorf("line %d: allowlist entry %s has no reason", i+1, key)
		}
		code, param, ok := strings.Cut(key, "/")
		if !ok || code == "" || param == "" {
			return allowlist{}, fmt.Errorf("line %d: key %q must be code/key or */key", i+1, key)
		}
		if _, duplicate := allow.reasons[key]; duplicate {
			return allowlist{}, fmt.Errorf("line %d: duplicate entry for %s", i+1, key)
		}
		allow.reasons[key] = reason
	}
	if !sawCount {
		return allowlist{}, fmt.Errorf("missing # count: header")
	}
	if allow.count != len(allow.reasons) {
		return allowlist{}, fmt.Errorf("count header says %d but the allowlist has %d entries", allow.count, len(allow.reasons))
	}
	return allow, nil
}

// allowlistForTest exposes the embedded allowlist to this package's own test.
func allowlistForTest() (allowlist, error) { return loadAllowlist() }
