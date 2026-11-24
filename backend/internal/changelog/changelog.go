// Package changelog holds the CHANGELOG.md and changelog.d/ rules shared by
// cmd/changelogcheck, cmd/changelogfragmentcheck and cmd/changelogassemble
// (docs/changelog-policy.md, REL-05). Keeping section order and bullet
// validation in one place means the per-PR fragment check and the
// [Unreleased] lint cannot drift apart.
package changelog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MaxWords is the hard limit behind the policy's "about 40 words".
const MaxWords = 60

// Section is one Keep-a-Changelog section. Name is the display name used in
// CHANGELOG.md ("Added"); Key is the lowercase form used in a fragment
// filename ("added").
type Section struct {
	Name string
	Key  string
}

// Sections is the canonical section order, as required by the changelog policy.
var Sections = []Section{
	{Name: "Added", Key: "added"},
	{Name: "Changed", Key: "changed"},
	{Name: "Deprecated", Key: "deprecated"},
	{Name: "Removed", Key: "removed"},
	{Name: "Fixed", Key: "fixed"},
	{Name: "Security", Key: "security"},
	{Name: "Internal", Key: "internal"},
}

// SectionForName returns the section whose display name is name.
func SectionForName(name string) (Section, bool) {
	for _, s := range Sections {
		if s.Name == name {
			return s, true
		}
	}
	return Section{}, false
}

// SectionForKey returns the section whose fragment filename key is key.
func SectionForKey(key string) (Section, bool) {
	for _, s := range Sections {
		if s.Key == key {
			return s, true
		}
	}
	return Section{}, false
}

// SectionPosition returns the 1-based position of a display-name section in
// policy order.
func SectionPosition(name string) (int, bool) {
	for i, s := range Sections {
		if s.Name == name {
			return i + 1, true
		}
	}
	return 0, false
}

// SectionIndex returns the 0-based policy index of sec. An unknown section
// sorts after every known one.
func SectionIndex(sec Section) int {
	for i, s := range Sections {
		if s.Key == sec.Key && s.Name == sec.Name {
			return i
		}
	}
	return len(Sections)
}

// SectionNameList returns the display names in policy order, for error text.
func SectionNameList() string {
	names := make([]string, 0, len(Sections))
	for _, s := range Sections {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

// SectionKeyList returns the fragment filename keys in policy order.
func SectionKeyList() string {
	keys := make([]string, 0, len(Sections))
	for _, s := range Sections {
		keys = append(keys, s.Key)
	}
	return strings.Join(keys, ", ")
}

var (
	urlRe      = regexp.MustCompile(`https?://\S+`)
	linkTextRe = regexp.MustCompile(`\[#\d+\]\(`)
	bareRefRe  = regexp.MustCompile(`#\d+`)
	issueURLRe = regexp.MustCompile(`/issues/([0-9]+)`)
)

// ValidateBullet returns the findings for one Keep-a-Changelog bullet: a bold
// lead phrase, no bare #number references, and the word limit. Continuation
// lines are folded into the same text by cmd/changelogcheck before this call.
func ValidateBullet(text string) []string {
	var out []string
	if !strings.HasPrefix(text, "- **") {
		out = append(out, "bullet must start with a bold lead phrase: - **Lead**: ...")
	}
	stripped := linkTextRe.ReplaceAllString(text, "(")
	stripped = urlRe.ReplaceAllString(stripped, "")
	if bareRefRe.MatchString(stripped) {
		out = append(out, "reference issues with a full URL, not a bare #number")
	}
	if words := len(strings.Fields(stripped)); words > MaxWords {
		out = append(out, fmt.Sprintf("bullet has %d words; keep it to about 40 (limit %d)", words, MaxWords))
	}
	return out
}

// BulletReferencesIssue reports whether bullet links issue in an /issues/<n>
// URL.
func BulletReferencesIssue(bullet string, issue int) bool {
	for _, m := range issueURLRe.FindAllStringSubmatch(bullet, -1) {
		n, err := strconv.Atoi(m[1])
		if err == nil && n == issue {
			return true
		}
	}
	return false
}

// FragmentName is the parsed form of a fragment filename.
type FragmentName struct {
	Section Section
	Issue   int
	Suffix  int
}

var fragmentNameRe = regexp.MustCompile(`^([a-z]+)-([0-9]+)(?:-([0-9]+))?\.md$`)

// ParseFragmentName parses <section>-<issue>[-<n>].md.
func ParseFragmentName(name string) (FragmentName, error) {
	m := fragmentNameRe.FindStringSubmatch(name)
	if m == nil {
		return FragmentName{}, fmt.Errorf("fragment %q must be named <section>-<issue>[-<n>].md (sections: %s)", name, SectionKeyList())
	}
	sec, ok := SectionForKey(m[1])
	if !ok {
		return FragmentName{}, fmt.Errorf("unknown section %q in fragment %q (use %s)", m[1], name, SectionKeyList())
	}
	issue, err := strconv.Atoi(m[2])
	if err != nil || issue < 1 {
		return FragmentName{}, fmt.Errorf("fragment %q needs a positive GitHub issue number", name)
	}
	suffix := 0
	if m[3] != "" {
		suffix, err = strconv.Atoi(m[3])
		if err != nil {
			return FragmentName{}, fmt.Errorf("fragment %q has an invalid -<n> suffix", name)
		}
	}
	return FragmentName{Section: sec, Issue: issue, Suffix: suffix}, nil
}

// Fragment is one changelog.d/ entry.
type Fragment struct {
	Path       string
	Name       string
	Section    Section
	Issue      int
	Suffix     int
	Bullet     string
	BulletLine int
}

// Finding is one validation problem, rendered as path:line: message.
type Finding struct {
	Path    string
	Line    int
	Message string
}

// String renders the finding in the form the commands print.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Message)
}

// LoadDir reads every *.md fragment in dir. README.md and non-Markdown files
// are ignored, and subdirectories are not descended into. Fragments are
// returned in deterministic order (section policy order, then filename);
// validation problems are returned as findings without aborting the load.
func LoadDir(dir string) ([]Fragment, []Finding, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var fragments []Fragment
	var findings []Finding
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if lower == "readme.md" || !strings.HasSuffix(lower, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		meta, err := ParseFragmentName(name)
		if err != nil {
			findings = append(findings, Finding{Path: path, Line: 1, Message: err.Error()})
			continue
		}
		data, err := os.ReadFile(path) // #nosec G304 -- name comes from ReadDir of the caller's -dir
		if err != nil {
			return nil, nil, err
		}
		fragment, fragmentFindings := parseFragment(path, name, meta, string(data))
		fragments = append(fragments, fragment)
		findings = append(findings, fragmentFindings...)
	}
	SortFragments(fragments)
	return fragments, findings, nil
}

func parseFragment(path, name string, meta FragmentName, body string) (Fragment, []Finding) {
	f := Fragment{
		Path:    path,
		Name:    name,
		Section: meta.Section,
		Issue:   meta.Issue,
		Suffix:  meta.Suffix,
	}
	var findings []Finding

	type line struct {
		number int
		text   string
	}
	var content []line
	for i, raw := range strings.Split(body, "\n") {
		text := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		content = append(content, line{number: i + 1, text: text})
	}
	if len(content) == 0 {
		findings = append(findings, Finding{Path: path, Line: 1, Message: "fragment is empty; expected one bullet line"})
		return f, findings
	}

	f.BulletLine = content[0].number
	f.Bullet = strings.TrimSpace(content[0].text)
	if strings.HasPrefix(f.Bullet, "#") {
		findings = append(findings, Finding{Path: path, Line: f.BulletLine, Message: "fragment must not contain a section header; write one bullet line only"})
	}
	if len(content) > 1 {
		findings = append(findings, Finding{Path: path, Line: content[1].number, Message: "fragment must contain exactly one bullet line; found another non-blank line"})
	}
	for _, message := range ValidateBullet(f.Bullet) {
		findings = append(findings, Finding{Path: path, Line: f.BulletLine, Message: message})
	}
	if !BulletReferencesIssue(f.Bullet, f.Issue) {
		findings = append(findings, Finding{
			Path:    path,
			Line:    f.BulletLine,
			Message: fmt.Sprintf("bullet must link issue #%d in an issues/%d URL", f.Issue, f.Issue),
		})
	}
	return f, findings
}

// SortFragments orders fragments by section policy order, then filename.
func SortFragments(fragments []Fragment) {
	sort.SliceStable(fragments, func(i, j int) bool {
		pi, pj := SectionIndex(fragments[i].Section), SectionIndex(fragments[j].Section)
		if pi != pj {
			return pi < pj
		}
		return fragments[i].Name < fragments[j].Name
	})
}

// ErrMissingUnreleased is returned when CHANGELOG.md has no [Unreleased]
// heading.
var ErrMissingUnreleased = errors.New("CHANGELOG.md has no ## [Unreleased] section")
