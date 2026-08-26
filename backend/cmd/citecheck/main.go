// Command citecheck is the evidence-citation gate for the security checklists
// (issue #378, the ASVS L2 / MASVS-L1 verification pass).
//
// docs/security/asvs-l2.md and masvs-l1.md make a specific promise about
// themselves: "take any Vx.y.z from ASVS 4.0.3 or API# from the 2023 Top 10,
// grep for it below, and get a status + citation. No row is left `satisfied`
// without a citation." That promise decays silently — not when someone edits
// the doc, but when someone *moves code*: a renamed file or a shifted function
// turns a `file:line` citation into a dangling pointer, and nothing fails.
// A checklist whose citations no longer resolve is worse than no checklist,
// because it still reads as evidence.
//
// This command re-verifies the mechanical half of the verification pass, so a
// re-verification is a diff rather than a rewrite:
//
//  1. Every backticked `path[:line[-line]]` citation resolves to a real file in
//     the tree, and any line range is inside that file.
//  2. Every cited test identifier (Go `TestXxx`, Kotlin `SomeTest.method`)
//     still exists somewhere in the tree.
//  3. Every control row carries a status drawn from the file's own legend and a
//     non-empty evidence cell, and every `satisfied` row cites something.
//
// It also prints the per-chapter status census, which is what the dated
// verification report (docs/security/asvs-l2-verification-report.md) quotes —
// so the report's counts cannot drift away from the checklists.
//
// Exit status 0 means every citation resolves; 1 means at least one does not;
// 2 means the check itself could not run.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// securityDocs are the checked documents, relative to the repo root. Adding a
// security doc that cites code means adding it here.
var securityDocs = []string{
	"docs/security/asvs-l2.md",
	"docs/security/masvs-l1.md",
	"docs/security/threat-model.md",
}

// searchPrefixes are tried in order when a citation is written relative to a
// tree rather than to the repo root — the checklists cite `config/config.go`
// (backend-relative), `unit-tests.yml` (workflow-relative), `asvs-l2.md`
// (sibling doc), and so on. Repo-root-relative is tried first.
var searchPrefixes = []string{
	"",
	"backend/",
	"frontend/",
	"frontend/src/",
	".github/workflows/",
	"docs/",
	"docs/security/",
	"android/",
	"android/app/src/main/",
	"docker/",
}

// skipDirs are never walked when indexing the tree: build output and vendored
// dependencies are not citable evidence, and walking them is most of the cost.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "build": true,
	"dist": true, ".gradle": true, ".idea": true, "coverage": true,
	"_site": true, ".venv": true, "__pycache__": true,
}

// allowedMissing are citations that deliberately name something that is not a
// file in this repository. Each entry is a written-down decision, not a
// suppression: if one of these ever becomes a real path, delete its line.
var allowedMissing = map[string]string{
	// masvs-l1.md STORAGE-4 / P5: the Firebase config is deliberately NOT
	// committed — the row's whole point is that Firebase stays inert until an
	// operator supplies one.
	"google-services.json": "deliberately uncommitted (masvs-l1.md STORAGE-4)",
	// asvs-l2.md V14.x / API8: an RFC 9116 URL path served by the app, not a
	// file in the tree.
	"/.well-known/security.txt": "served URL path, not a repository file",
}

// legendStatuses is the status vocabulary both checklists declare in their
// "Status legend" section. A row using anything else is a typo, not a status.
var legendStatuses = map[string]bool{
	"satisfied":      true,
	"partial":        true,
	"not-applicable": true,
	"out-of-scope":   true,
}

// citableExts bounds what counts as a path citation, so prose in backticks
// (`SameSite`, `TokenVersion`, `encv1:`) is not mistaken for a file.
const citableExts = `go|ts|tsx|md|yml|yaml|sh|conf|sql|json|kt|kts|xml|example|toml|pro|txt|html`

var (
	// backtickRe pulls the backticked spans out of a line; only whole spans are
	// considered, so `services/foo.go:12` is a citation but a path mentioned
	// inside a prose sentence is not.
	backtickRe = regexp.MustCompile("`([^`]+)`")

	// pathRe matches a whole backtick span that is a path, optionally with a
	// line or line range. The `...` form is the elision the Android rows use
	// for deep package paths (`core/data/.../DataModule.kt`).
	pathRe = regexp.MustCompile(`^([A-Za-z0-9_./-]*(?:\.\.\./)?[A-Za-z0-9_./-]+?\.(?:` + citableExts + `)|[A-Za-z0-9_./-]*Dockerfile[A-Za-z0-9_.-]*|\.[a-z]+ignore)(?::(\d+)(?:-(\d+))?)?$`)

	// goTestRe matches a Go test function name cited as evidence. A trailing
	// `*` is the "whole family" citation form (`TestResetUserTwoFactor_*`),
	// checked as a prefix rather than an exact name.
	goTestRe = regexp.MustCompile(`\bTest[A-Za-z0-9_]{3,}\*?`)

	// ktTestRe matches the Kotlin `SomeTest.methodName` citation form used by
	// the Android rows.
	ktTestRe = regexp.MustCompile(`\b([A-Z][A-Za-z0-9_]*Test)\.([a-z][A-Za-z0-9_]*)\b`)

	// identRe tokenizes source files when building the symbol set.
	identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

	// sourceExts are the files searched for cited test identifiers.
	sourceExts = map[string]bool{".go": true, ".kt": true, ".kts": true, ".ts": true, ".tsx": true}
)

// finding is one failed check, reported as `doc:line  message`.
type finding struct {
	doc  string
	line int
	msg  string
}

// treeIndex is the citable surface of the repository: every file by
// repo-relative path, plus a basename index for the shorthand citations
// (`mailer.go`, `rate_limiter.go`) the checklists use inside a row that has
// already established the directory.
type treeIndex struct {
	root   string
	files  map[string]bool
	byBase map[string][]string
	paths  []string

	lineCounts map[string]int
	symbols    map[string]bool
}

func main() {
	drift := flag.Bool("drift", false, "advisory pass: list line-range citations whose cited lines no longer look like what the row claims (heuristic, never gating)")
	flag.Parse()

	if *drift {
		if err := runDrift(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "citecheck:", err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	code, err := run(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "citecheck:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

// run performs the whole check and returns the process exit code (0 clean /
// 1 findings). Split out of main so the exit paths are covered by tests.
func run(w io.Writer) (int, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		return 0, err
	}
	idx, err := buildIndex(root)
	if err != nil {
		return 0, err
	}
	return check(w, root, idx, securityDocs)
}

// check runs every check over docs and writes the summary to w.
func check(w io.Writer, root string, idx *treeIndex, docs []string) (int, error) {
	var all []finding
	for _, doc := range docs {
		body, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", doc, err)
		}
		lines := strings.Split(string(body), "\n")

		cites, ids := extractCitations(lines)
		all = append(all, checkPaths(idx, doc, cites)...)
		all = append(all, checkIdentifiers(idx, doc, ids)...)

		rows := parseControlRows(lines)
		all = append(all, checkRows(doc, rows)...)

		fmt.Fprintf(w, "%s: %d path citations, %d test-identifier citations, %d control rows\n",
			doc, len(cites), len(ids), len(rows))
		for _, line := range censusLines(rows) {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}

	if len(all) == 0 {
		fmt.Fprintln(w, "\nAll citations resolve.")
		return 0, nil
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].doc != all[j].doc {
			return all[i].doc < all[j].doc
		}
		return all[i].line < all[j].line
	})
	fmt.Fprintf(w, "\n%d citation problem(s):\n", len(all))
	for _, f := range all {
		fmt.Fprintf(w, "  %s:%d  %s\n", f.doc, f.line, f.msg)
	}
	return 1, nil
}

// findRepoRoot walks up from start until it finds the checklist the whole
// command exists to verify, so the command works from backend/ (go run) and
// from backend/cmd/citecheck/ (go test) alike.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, securityDocs[0])); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for %s)", start, securityDocs[0])
		}
		dir = parent
	}
}

// buildIndex walks the tree once and records every citable file.
func buildIndex(root string) (*treeIndex, error) {
	idx := &treeIndex{
		root:       root,
		files:      map[string]bool{},
		byBase:     map[string][]string{},
		lineCounts: map[string]int{},
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		idx.files[rel] = true
		idx.byBase[d.Name()] = append(idx.byBase[d.Name()], rel)
		idx.paths = append(idx.paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return idx, nil
}

// citation is one backticked path reference, with its line range if it had one.
type citation struct {
	line int
	span string // the whole backtick span, for the error message
	path string
	from int // 0 when no line was cited
	to   int
}

// testCitation is one cited test identifier. kind is "Go" or "Kotlin".
type testCitation struct {
	line int
	kind string
	name string
}

// extractCitations pulls both citation kinds out of a document's lines.
func extractCitations(lines []string) ([]citation, []testCitation) {
	var cites []citation
	var ids []testCitation
	for i, line := range lines {
		for _, m := range backtickRe.FindAllStringSubmatch(line, -1) {
			span := strings.TrimSpace(m[1])

			if p := pathRe.FindStringSubmatch(span); p != nil {
				c := citation{line: i + 1, span: span, path: p[1]}
				if p[2] != "" {
					c.from, _ = strconv.Atoi(p[2])
					c.to = c.from
					if p[3] != "" {
						c.to, _ = strconv.Atoi(p[3])
					}
				}
				cites = append(cites, c)
				continue
			}

			for _, name := range goTestRe.FindAllString(span, -1) {
				ids = append(ids, testCitation{line: i + 1, kind: "Go", name: name})
			}
			for _, kt := range ktTestRe.FindAllStringSubmatch(span, -1) {
				ids = append(ids,
					testCitation{line: i + 1, kind: "Kotlin", name: kt[1]},
					testCitation{line: i + 1, kind: "Kotlin", name: kt[2]},
				)
			}
		}
	}
	return cites, ids
}

// checkPaths resolves every path citation and validates its line range.
func checkPaths(idx *treeIndex, doc string, cites []citation) []finding {
	var out []finding
	for _, c := range cites {
		if _, ok := allowedMissing[c.path]; ok {
			continue
		}
		cands := idx.resolve(c.path)
		if len(cands) == 0 {
			out = append(out, finding{doc, c.line, fmt.Sprintf("citation `%s` does not resolve to any file", c.span)})
			continue
		}
		if c.from == 0 {
			continue
		}
		// A basename citation can match more than one file (`auth.go` is both
		// middleware/auth.go and carddav/auth.go); the surrounding row is what
		// disambiguates for a human, so the range only has to be valid for one
		// of the candidates.
		inRange := false
		var sizes []string
		for _, cand := range cands {
			n, err := idx.lineCount(cand)
			if err != nil {
				return append(out, finding{doc, c.line, fmt.Sprintf("cannot read %s: %v", cand, err)})
			}
			sizes = append(sizes, fmt.Sprintf("%s has %d lines", cand, n))
			if c.to <= n {
				inRange = true
				break
			}
		}
		if !inRange {
			out = append(out, finding{doc, c.line,
				fmt.Sprintf("citation `%s` names line %d but %s", c.span, c.to, strings.Join(sizes, "; "))})
		}
	}
	return out
}

// checkIdentifiers confirms every cited test identifier still exists.
func checkIdentifiers(idx *treeIndex, doc string, ids []testCitation) []finding {
	if len(ids) == 0 {
		return nil
	}
	syms, err := idx.symbolSet()
	if err != nil {
		return []finding{{doc, 0, fmt.Sprintf("cannot index source symbols: %v", err)}}
	}
	var out []finding
	for _, id := range ids {
		if strings.HasSuffix(id.name, "*") {
			if hasPrefixSymbol(syms, strings.TrimSuffix(id.name, "*")) {
				continue
			}
			out = append(out, finding{doc, id.line, fmt.Sprintf("%s test-family citation `%s` matches no identifier in the tree", id.kind, id.name)})
			continue
		}
		if syms[id.name] {
			continue
		}
		out = append(out, finding{doc, id.line, fmt.Sprintf("%s test identifier `%s` does not exist in the tree", id.kind, id.name)})
	}
	return out
}

// hasPrefixSymbol reports whether any indexed identifier starts with prefix,
// backing the `TestFoo_*` family citation form.
func hasPrefixSymbol(syms map[string]bool, prefix string) bool {
	for s := range syms {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// resolve returns every file a citation could mean, best match first.
func (idx *treeIndex) resolve(path string) []string {
	if idx.files[path] {
		return []string{path}
	}
	if strings.Contains(path, ".../") {
		// `core/data/.../DataModule.kt` — the Android rows elide deep package
		// paths. Match on the prefix before the elision and the tail after it.
		// The tail must land on a path-segment boundary, or `.../SettingsScreen.kt`
		// would also match `ImmichSettingsScreen.kt` in the same directory.
		head, tail, _ := strings.Cut(path, "...")
		tail = strings.TrimPrefix(tail, "/")
		var out []string
		for _, p := range idx.paths {
			if strings.Contains(p, head) && (p == tail || strings.HasSuffix(p, "/"+tail)) {
				out = append(out, p)
			}
		}
		return out
	}
	for _, pre := range searchPrefixes {
		if pre != "" && idx.files[pre+path] {
			return []string{pre + path}
		}
	}
	base := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		base = path[i+1:]
	}
	var suffix []string
	for _, cand := range idx.byBase[base] {
		if cand == path || strings.HasSuffix(cand, "/"+path) {
			suffix = append(suffix, cand)
		}
	}
	if len(suffix) > 0 {
		return suffix
	}
	return idx.byBase[base]
}

// lineCount returns the number of lines in a file, memoized.
func (idx *treeIndex) lineCount(rel string) (int, error) {
	if n, ok := idx.lineCounts[rel]; ok {
		return n, nil
	}
	body, err := os.ReadFile(filepath.Join(idx.root, rel))
	if err != nil {
		return 0, err
	}
	n := strings.Count(string(body), "\n")
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		n++
	}
	idx.lineCounts[rel] = n
	return n, nil
}

// symbolSet is every identifier appearing in the source trees, built once.
func (idx *treeIndex) symbolSet() (map[string]bool, error) {
	if idx.symbols != nil {
		return idx.symbols, nil
	}
	syms := map[string]bool{}
	for _, rel := range idx.paths {
		if !sourceExts[filepath.Ext(rel)] {
			continue
		}
		body, err := os.ReadFile(filepath.Join(idx.root, rel))
		if err != nil {
			return nil, err
		}
		for _, tok := range identRe.FindAllString(string(body), -1) {
			syms[tok] = true
		}
	}
	idx.symbols = syms
	return syms, nil
}

// controlRow is one row of a `| ID | Requirement | Status | Evidence |` table.
type controlRow struct {
	line     int
	chapter  string
	id       string
	status   string
	evidence string
}

// parseControlRows extracts the control tables — and only those: the
// checklists also contain tables of DoS limits and metadata, which have
// different columns and are not control rows.
func parseControlRows(lines []string) []controlRow {
	var out []controlRow
	chapter := ""
	inTable := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			chapter = strings.TrimPrefix(line, "## ")
			inTable = false
			continue
		}
		if !strings.HasPrefix(line, "|") {
			inTable = false
			continue
		}
		cells := splitRow(line)
		if len(cells) < 4 {
			inTable = false
			continue
		}
		if isControlHeader(cells) {
			inTable = true
			continue
		}
		if !inTable || isSeparator(cells) {
			continue
		}
		out = append(out, controlRow{
			line:     i + 1,
			chapter:  chapter,
			id:       strings.Trim(cells[0], "* "),
			status:   strings.Trim(cells[2], "* "),
			evidence: strings.TrimSpace(cells[3]),
		})
	}
	return out
}

// splitRow splits a markdown table row on unescaped pipes — the evidence cells
// contain `normal\|private\|secret`, which is one cell, not three.
func splitRow(line string) []string {
	const sentinel = "\x00"
	line = strings.ReplaceAll(line, `\|`, sentinel)
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.TrimSpace(p), sentinel, `\|`)
	}
	return parts
}

func isControlHeader(cells []string) bool {
	if !strings.EqualFold(cells[2], "Status") || !strings.EqualFold(cells[3], "Evidence") {
		return false
	}
	return cells[0] == "ID" || cells[0] == "#"
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return true
}

// checkRows enforces the promise the checklists make about themselves: a
// status from the legend, evidence in every row, and a citation behind every
// `satisfied`.
func checkRows(doc string, rows []controlRow) []finding {
	var out []finding
	for _, r := range rows {
		if !legendStatuses[r.status] {
			out = append(out, finding{doc, r.line, fmt.Sprintf("control %s has status %q, which is not in the status legend", r.id, r.status)})
			continue
		}
		if r.evidence == "" {
			out = append(out, finding{doc, r.line, fmt.Sprintf("control %s (%s) has an empty evidence cell", r.id, r.status)})
			continue
		}
		if r.status == "satisfied" && !backtickRe.MatchString(r.evidence) {
			out = append(out, finding{doc, r.line, fmt.Sprintf("control %s is satisfied but cites nothing (no `citation` in the evidence cell)", r.id)})
		}
	}
	return out
}

// censusLines renders the per-chapter status census the verification report
// quotes, so the report's counts are generated rather than transcribed.
func censusLines(rows []controlRow) []string {
	type counts map[string]int
	byChapter := map[string]counts{}
	var order []string
	total := counts{}
	for _, r := range rows {
		if _, ok := byChapter[r.chapter]; !ok {
			byChapter[r.chapter] = counts{}
			order = append(order, r.chapter)
		}
		byChapter[r.chapter][r.status]++
		total[r.status]++
	}
	render := func(label string, c counts) string {
		var parts []string
		for _, s := range []string{"satisfied", "partial", "not-applicable", "out-of-scope"} {
			if c[s] > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", s, c[s]))
			}
		}
		return fmt.Sprintf("%-56s %s", label, strings.Join(parts, ", "))
	}
	var out []string
	for _, ch := range order {
		out = append(out, render(ch, byChapter[ch]))
	}
	if len(order) > 0 {
		out = append(out, render("TOTAL", total))
	}
	return out
}

// driftStopWords are too generic to tell a reviewer anything about whether a
// cited range still says what the row claims.
var driftStopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "via": true, "see": true,
	"per": true, "its": true, "this": true, "that": true, "which": true, "only": true,
	"every": true, "all": true, "any": true, "not": true, "never": true, "both": true,
	"same": true, "real": true, "code": true, "line": true, "lines": true, "file": true,
	"files": true, "test": true, "tests": true, "row": true, "rows": true, "doc": true,
	"docs": true, "data": true, "user": true, "users": true, "app": true, "own": true,
	"new": true, "old": true, "one": true, "two": true, "three": true, "full": true,
	"issue": true, "issues": true, "incl": true, "plus": true, "also": true, "but": true,
}

var (
	driftCiteRe = regexp.MustCompile("`([A-Za-z0-9_./-]*(?:\\.\\.\\./)?[A-Za-z0-9_./-]+?\\.(?:" + citableExts + ")):(\\d+)(?:-(\\d+))?`")
	driftWordRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]{2,}`)
)

// runDrift is the advisory half of the verification pass. citecheck's gate can
// prove a citation points *somewhere real*; it cannot prove it still points at
// what the row claims, and that is the failure mode that actually accumulated
// here — the ASVS L2 pass of 2026-08-26 found 74 line ranges that resolved
// cleanly while naming code that had moved out from under them (a CORS row
// citing the scheduler, a govulncheck row citing the fuzz step).
//
// The heuristic: take the prose immediately before a line-range citation — the
// words the row uses to say what it is pointing at — and check whether any of
// them still appears in the cited lines. A miss is a candidate for review, not
// a failure: negative claims and pure-structure citations legitimately share no
// vocabulary with their target. It is deliberately not part of the gate.
func runDrift(w io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		return err
	}
	idx, err := buildIndex(root)
	if err != nil {
		return err
	}
	return driftReport(w, root, idx, securityDocs)
}

// driftReport is runDrift's body, with the root and document list injected so
// tests can drive it against a fixture tree.
func driftReport(w io.Writer, root string, idx *treeIndex, docs []string) error {
	total, flagged := 0, 0
	for _, doc := range docs {
		body, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			return fmt.Errorf("reading %s: %w", doc, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			for _, m := range driftCiteRe.FindAllStringSubmatchIndex(line, -1) {
				total++
				span := line[m[0]:m[1]]
				path := line[m[2]:m[3]]
				from, _ := strconv.Atoi(line[m[4]:m[5]])
				to := from
				if m[6] >= 0 {
					to, _ = strconv.Atoi(line[m[6]:m[7]])
				}
				words := driftKeywords(line[:m[0]], path)
				if len(words) == 0 {
					continue // nothing to judge against
				}
				cands := idx.resolve(path)
				if len(cands) == 0 {
					continue // the gate already reports this
				}
				hit := false
				for _, cand := range cands {
					ok, err := citedRangeMentions(idx, cand, from, to, words)
					if err != nil {
						return err
					}
					if ok {
						hit = true
						break
					}
				}
				if !hit {
					flagged++
					fmt.Fprintf(w, "%s:%d  %s -> %s  (looked for: %s)\n",
						doc, i+1, span, cands[0], strings.Join(words, ", "))
				}
			}
		}
	}
	fmt.Fprintf(w, "\n%d line-range citations, %d for review.\n", total, flagged)
	fmt.Fprintln(w, "Advisory only: confirm each by eye, then correct the range or leave it.")
	return nil
}

// driftKeywords pulls the last few meaningful words of the prose leading up to
// a citation, dropping the cited file's own stem (which would otherwise match
// its contents trivially).
func driftKeywords(before, path string) []string {
	if i := strings.LastIndex(before, "|"); i >= 0 {
		before = before[i+1:] // stay inside this table cell
	}
	if len(before) > 140 {
		before = before[len(before)-140:]
	}
	stem := strings.ToLower(strings.SplitN(filepath.Base(path), ".", 2)[0])
	var out []string
	for _, w := range driftWordRe.FindAllString(before, -1) {
		lw := strings.ToLower(w)
		if driftStopWords[lw] || lw == stem || lw == strings.ReplaceAll(stem, "_", "") {
			continue
		}
		out = append(out, w)
	}
	if len(out) > 8 {
		out = out[len(out)-8:]
	}
	return out
}

// citedRangeMentions reports whether the cited lines contain any of words.
func citedRangeMentions(idx *treeIndex, rel string, from, to int, words []string) (bool, error) {
	body, err := os.ReadFile(filepath.Join(idx.root, rel))
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(body), "\n")
	if from < 1 {
		from = 1
	}
	if to > len(lines) {
		to = len(lines)
	}
	if from > to {
		return false, nil
	}
	snippet := strings.ToLower(strings.Join(lines[from-1:to], "\n"))
	for _, w := range words {
		if strings.Contains(snippet, strings.ToLower(w)) {
			return true, nil
		}
	}
	return false, nil
}
