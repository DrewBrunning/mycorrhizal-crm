// Command docscheck is the DOC-04 (issue #489) structural test for the docs
// site: it verifies that the prose under docs/ cannot silently drift from
// reality. Unlike the substring-guard family of *test files elsewhere in this
// repo, it runs unconditionally in CI (unit-tests.yml, the docs-citations
// job), because a docs-only PR would otherwise never exercise it.
//
// Checks (each finding is a human-actionable line on stdout; any finding is an
// exit 1):
//
//  1. Internal links and anchors. Every markdown link inside docs/**/*.md must
//     resolve to a file that exists and, when it carries a #fragment, to a
//     heading id or <a id> the target actually has. Both link spellings the
//     site uses are covered: GitHub-style .md relative links and the
//     just-the-docs-rendered .html form (deployment.html#backups), plus
//     github.com/DrewBrunning/mycorrhizal-crm/blob/main/<path> links mapped
//     back onto the checkout.
//
//  2. Repo-file references. A path-shaped reference to a tracked file
//     (backend/..., frontend/..., docs/..., cmd/<tool>, ...) must exist.
//
//  3. Operator-facing command blocks must not assume a toolchain their
//     audience does not have (issue #461 / DOC-04 step 5). A shell fence that
//     runs `go run`, `make`, `cd backend`, or a JS/Gradle tool must be
//     annotated — a shell comment in the block or a note in the section that
//     names the Go toolchain / a repo checkout — or the document must declare
//     up front that its commands assume one.
//
//  4. DOC-01 drift (issue #486). The operator-facing supported-versions page
//     states the same floors the engineering matrix
//     (development/supported-runtime-matrix.md) does. Both are prose, so the
//     consistency is pinned by token; changing a floor means changing both
//     documents (or this table).
//
//  5. DOC-03 structure (issue #488). Every outbound integration registered in
//     backend/integrations Registry() has an operator-facing section
//     (<a id="<id>">) in docs/integration-ownership.md, so a new integration
//     without a documentation section fails the build.
//
//  6. CI follows the documented commands. deploy-smoke CI and
//     docs/getting-started.md must state the same bring-up command, and
//     docs/deployment.md's upgrade block must state the same command pair the
//     operator is told to run.
//
//  7. PII-inventory ↔ schema drift (issue #1316). Every table in a freshly
//     migrated database appears in docs/security/pii-inventory.md (a code span
//     in a table row or heading) or in the reasoned exclusion list in
//     pii_inventory.go.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/integrations"
	"mycorrhizal/internal/mdref"
)

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "docscheck:", err) // # pragma: no cover — main is not exercised by tests; runAt is
		os.Exit(1)                                 // # pragma: no cover — see above; main's own os.Exit is not exercised by tests
	}
	if err := runAt(root); err != nil {
		fmt.Fprintln(os.Stderr, "docscheck:", err) // # pragma: no cover — main is not exercised by tests; runAt is
		os.Exit(1)                                 // # pragma: no cover — see above; main's own os.Exit is not exercised by tests
	}
}

// runAt executes every check against the repository rooted at root and returns
// the first hard error (I/O) or a findings error listing every problem. It is
// separate from main so tests can drive it against a fixture tree.
func runAt(root string) error {
	findings := collectFindings(root)
	if len(findings) == 0 {
		return nil
	}
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Fprintln(os.Stdout, f) // # pragma: no cover — output path; tests assert on collectFindings
	}
	return fmt.Errorf("%d documentation finding(s) — see output above", len(findings)) // # pragma: no cover — error path; tests assert on collectFindings
}

// collectFindings runs every check and returns the human-readable findings.
func collectFindings(root string) []string {
	var findings []string
	findings = append(findings, checkLinks(root)...)
	findings = append(findings, checkCommandBlocks(root)...)
	findings = append(findings, checkSupportedVersionsDrift(root)...)
	findings = append(findings, checkIntegrationOwnership(root)...)
	findings = append(findings, checkCICommandBinding(root)...)
	findings = append(findings, checkPIIInventory(root)...)
	return findings
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover — Getwd fails only if the cwd was deleted
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no backend/go.mod) from %s", dir)
		}
		dir = parent
	}
}

func mustCompile(pat string) *regexp.Regexp { return regexp.MustCompile(pat) }

var mdHeadingRe = mustCompile(`^(#{1,6})\s+(.*)$`)

// ---------------------------------------------------------------------------
// 1 + 2. Links, anchors, and repo-file references
// ---------------------------------------------------------------------------

func checkLinks(root string) []string {
	var findings []string
	docsDir := filepath.Join(root, "docs")
	files := listMarkdown(docsDir)
	resolver := mdref.Resolver{Root: root}
	for _, rel := range files {
		abs := filepath.Join(docsDir, rel)
		body, err := os.ReadFile(abs) // #nosec G304 -- rel comes from the filepath.Walk over docs/, never request input
		if err != nil {
			// # pragma: no cover — the file was listed by the walk above, so a
			// read failure is a racing deletion, not a normal state.
			findings = append(findings, fmt.Sprintf("%s: unreadable: %v", rel, err))
			continue
		}
		file := mdref.File{Path: abs, Display: rel, Text: string(body)}
		for _, f := range resolver.Links(file) {
			findings = append(findings, f.String())
		}
		for _, f := range resolver.RepoReferences(file) {
			findings = append(findings, f.String())
		}
	}
	return findings
}

func listMarkdown(docsDir string) []string {
	var files []string
	_ = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			rel, rerr := filepath.Rel(docsDir, path)
			if rerr == nil {
				files = append(files, rel)
			}
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// ---------------------------------------------------------------------------
// 3. Operator command blocks must not assume an unstated toolchain
// ---------------------------------------------------------------------------

var (
	toolchainCommandRe = mustCompile(`(?:^|\s)(go\s+run|go\s+build|make\s|cd\s+backend|\./gradlew|yarn\s|npx\s|npm\s)`)
	toolchainNoteRe    = mustCompile(`(?i)(go toolchain|go\s+1\.\d|repo clone|a clone of|from source|building from source|a checkout|checkout of this repo|developer|with a go toolchain|no go toolchain)`)
	docDeclaresReposRe = mustCompile(`(?i)(requires? a checkout|assumes? .*checkout|checkout of this repo with a go toolchain|requires? .*go toolchain)`)
)

// operatorDocs is the set of pages aimed at a Docker operator following
// commands on their host. The contributor-facing pages (development/*) are
// intentionally not in this set.
var operatorDocs = []string{
	"getting-started.md",
	"deployment.md",
	"supported-versions.md",
	"upgrade-compatibility.md",
	"integration-ownership.md",
	"operations/migration-recovery.md",
	"operations/disaster-recovery.md",
	"operations/search-index.md",
}

func checkCommandBlocks(root string) []string {
	var findings []string
	for _, rel := range operatorDocs {
		// #nosec G304 -- rel is from the fixed operatorDocs list, never request input
		body, err := os.ReadFile(filepath.Join(root, "docs", filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		lines := strings.Split(string(body), "\n")
		// A document-level declaration ("commands assume a checkout of this
		// repo with a Go toolchain") licenses the blocks below it, matching how
		// migration-recovery.md and disaster-recovery.md state the assumption.
		declaration := -1
		for i, line := range lines {
			if docDeclaresReposRe.MatchString(line) {
				declaration = i
				break
			}
		}
		lastHeading := -1
		for i := 0; i < len(lines); i++ {
			t := strings.TrimSpace(lines[i])
			if !strings.HasPrefix(t, "```") || strings.HasPrefix(t, "````") {
				if mdHeadingRe.MatchString(lines[i]) {
					lastHeading = i
				}
				continue
			}
			info := strings.TrimSpace(strings.TrimPrefix(t, "```"))
			start := i
			i++
			var block []string
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "```" {
					break
				}
				block = append(block, lines[i])
				i++
			}
			if !isShell(info) {
				continue
			}
			body := strings.Join(block, "\n")
			if !toolchainCommandRe.MatchString("\n" + body + "\n") {
				continue
			}
			if declaration >= 0 && declaration < start {
				continue
			}
			if !annotated(block, lines, lastHeading, start) {
				findings = append(findings, fmt.Sprintf("%s:%d: shell block runs %q but nothing nearby says it needs a repo checkout + Go toolchain (a Docker operator cannot run it — issue #461 / DOC-04)", rel, start+1, firstToolchainCommand(body)))
			}
		}
	}
	return findings
}

func isShell(info string) bool {
	switch info {
	case "", "sh", "bash", "shell", "console", "bash shell":
		return true
	}
	return false
}

// annotated reports whether the block, its section heading, or the prose
// between the previous heading and the block names the toolchain/checkout
// requirement.
func annotated(block, lines []string, lastHeading, blockStart int) bool {
	if toolchainNoteRe.MatchString(strings.Join(block, "\n")) {
		return true
	}
	proseStart := 0
	if lastHeading >= 0 {
		proseStart = lastHeading + 1
		if toolchainNoteRe.MatchString(lines[lastHeading]) {
			return true
		}
	}
	if blockStart-proseStart > 60 {
		proseStart = blockStart - 60
	}
	scanned := 0
	for j := blockStart - 1; j >= proseStart && scanned < 60; j-- {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		scanned++
		if toolchainNoteRe.MatchString(t) {
			return true
		}
	}
	return false
}

func firstToolchainCommand(block string) string {
	for _, line := range strings.Split(block, "\n") {
		if m := toolchainCommandRe.FindString(line); m != "" {
			return strings.TrimSpace(m)
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 4. DOC-01 (issue #486): supported-versions vs the engineering matrix
// ---------------------------------------------------------------------------

// operatorFloors is the operator-visible subset of the supported-runtime-matrix
// table. Each floor's token must appear in the engineering matrix and (in the
// given or alternative phrasing) in the operator page; moving a floor means
// updating both documents or this table fails. The Docker Engine floor is not
// in this token table: a whole-page Contains would be satisfied by any stray
// mention of the number (issue #941 — the operator page stated `>= 22.0` in its
// Docker row while a note elsewhere said `23.0`, and the old check passed). It
// is pinned structurally by dockerEngineRowFloor instead.
var operatorFloors = []struct {
	label  string
	matrix string
	page   string
	alt    string
}{
	{"Docker Compose", "Compose V2", "Compose V2", "Compose v2"},
	{"Browser floor (Chrome/Edge/Firefox)", "111", "111", ">=111"},
	{"Browser floor (Safari/iOS)", "16.4", "16.4", ""},
	{"Android minimum", "minSdk 26", "minSdk 26", "API 26"},
	{"Host architecture (arm)", "arm64", "arm64", ""},
	{"Host architecture (x86)", "x86_64", "x86_64", "amd64"},
	{"Host OS", "Linux", "Linux", ""},
	{"Storage constraint (network filesystems)", "network filesystems", "network filesystem", "network file system"},
	{"Storage constraint (local only)", "local filesystem", "local filesystem", "local disk"},
}

func checkSupportedVersionsDrift(root string) []string {
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	matrix, err := os.ReadFile(filepath.Join(root, "docs", "development", "supported-runtime-matrix.md"))
	if err != nil {
		return []string{"supported-versions drift: cannot read docs/development/supported-runtime-matrix.md: " + err.Error()}
	}
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	page, err := os.ReadFile(filepath.Join(root, "docs", "supported-versions.md"))
	if err != nil {
		return []string{"supported-versions drift: operator page docs/supported-versions.md missing: " + err.Error()}
	}
	mt := string(matrix)
	pt := string(page)
	var findings []string
	for _, f := range operatorFloors {
		if !strings.Contains(mt, f.matrix) {
			findings = append(findings, fmt.Sprintf("supported-versions drift: engineering matrix no longer states %q (%s) — update the matrix or drop the row", f.matrix, f.label))
		}
		inPage := strings.Contains(pt, f.page) || (f.alt != "" && strings.Contains(pt, f.alt))
		if !inPage {
			findings = append(findings, fmt.Sprintf("supported-versions drift: docs/supported-versions.md does not state %q (%s), which the engineering matrix does — update the operator page (issue #486)", f.page, f.label))
		}
	}
	findings = append(findings, checkDockerEngineFloor(mt, pt)...)
	return findings
}

// Docker Engine is the one floor that is a `>=`-prefixed semver, so it can be
// pinned structurally: read the `>=<version>` token out of the **Docker
// Engine** row of each document and require them to agree, and require the
// operator row to state exactly one. A plain token Contains (the operatorFloors
// mechanism) is not enough — it is satisfied by any mention of the number
// anywhere on the page, which is exactly how issue #941 slipped through.
var (
	dockerEngineRowRe  = mustCompile(`(?m)^.*\*\*Docker Engine\*\*.*$`)
	dockerFloorTokenRe = mustCompile(`>=\s*(\d+(?:\.\d+)*)`)
)

// dockerEngineRowFloor returns the floor tokens stated with `>=` in the
// **Docker Engine** table row of text, or nil when there is no such row.
func dockerEngineRowFloor(text string) []string {
	row := dockerEngineRowRe.FindString(text)
	if row == "" {
		return nil
	}
	var floors []string
	for _, m := range dockerFloorTokenRe.FindAllStringSubmatch(row, -1) {
		floors = append(floors, m[1])
	}
	return floors
}

// checkDockerEngineFloor pins the operator page's Docker Engine row to the
// engineering matrix's (issue #941). It flags a missing row on either side, a
// row stating more than one competing `>= NN.N` floor, and a mismatch between
// the two.
func checkDockerEngineFloor(matrix, page string) []string {
	matrixFloors := dockerEngineRowFloor(matrix)
	pageFloors := dockerEngineRowFloor(page)
	var findings []string
	if len(matrixFloors) == 0 {
		findings = append(findings, "supported-versions drift: engineering matrix has no **Docker Engine** row stating a `>= <version>` floor — update the matrix (issue #486)")
	}
	if len(pageFloors) == 0 {
		findings = append(findings, "supported-versions drift: docs/supported-versions.md has no **Docker Engine** row stating a `>= <version>` floor — update the operator page (issue #486)")
	}
	if len(pageFloors) > 1 {
		findings = append(findings, fmt.Sprintf("supported-versions drift: docs/supported-versions.md Docker Engine row states more than one competing floor (%s) — state exactly one (issue #941)", strings.Join(pageFloors, ", ")))
	}
	if len(matrixFloors) > 1 {
		findings = append(findings, fmt.Sprintf("supported-versions drift: engineering matrix Docker Engine row states more than one competing floor (%s) — state exactly one (issue #941)", strings.Join(matrixFloors, ", ")))
	}
	if len(matrixFloors) >= 1 && len(pageFloors) >= 1 && pageFloors[0] != matrixFloors[0] {
		findings = append(findings, fmt.Sprintf("supported-versions drift: docs/supported-versions.md Docker Engine row does not state the matrix's floor %q (it states %q) — update the operator page (issue #486)", matrixFloors[0], pageFloors[0]))
	}
	return findings
}

// ---------------------------------------------------------------------------
// 5. DOC-03 (issue #488): every registered integration is documented
// ---------------------------------------------------------------------------

func checkIntegrationOwnership(root string) []string {
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	doc, err := os.ReadFile(filepath.Join(root, "docs", "integration-ownership.md"))
	if err != nil {
		return []string{"integration-ownership: operator page docs/integration-ownership.md missing (issue #488): " + err.Error()}
	}
	text := string(doc)
	var findings []string
	for _, entry := range integrations.Registry() {
		anchor := `<a id="` + entry.ID + `"></a>`
		if !strings.Contains(text, anchor) {
			findings = append(findings, fmt.Sprintf("integration-ownership: integration %q (%s) has no operator section in docs/integration-ownership.md — add one anchored %s (issue #488)", entry.ID, entry.Name, anchor))
		}
	}
	return findings
}

// ---------------------------------------------------------------------------
// 6. CI follows the documented commands
// ---------------------------------------------------------------------------

func checkCICommandBinding(root string) []string {
	var findings []string
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	gettingStarted, err := os.ReadFile(filepath.Join(root, "docs", "getting-started.md"))
	if err == nil && !strings.Contains(string(gettingStarted), "docker compose up -d --build") {
		findings = append(findings, "docs/getting-started.md no longer contains the bring-up command `docker compose up -d --build` that deploy-smoke CI runs")
	}
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	deployment, err := os.ReadFile(filepath.Join(root, "docs", "deployment.md"))
	if err == nil {
		dt := string(deployment)
		if !strings.Contains(dt, "docker compose pull") || !strings.Contains(dt, "docker compose up -d") {
			findings = append(findings, "docs/deployment.md no longer documents the upgrade command pair `docker compose pull` + `docker compose up -d`")
		}
	}
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "deploy-smoke.yml"))
	if err == nil && !strings.Contains(string(workflow), "docker compose up -d --build") {
		findings = append(findings, "deploy-smoke.yml no longer runs the bring-up command `docker compose up -d --build` documented in docs/getting-started.md (issue #489)")
	}
	return findings
}
