// Command depexceptions is the freshness gate for the dependency-advisory
// exception ledger (COMPAT-03, issue #474).
//
// docs/dependency-upgrade-policy.md's "Security fixes" tier makes a specific
// promise: an advisory against a shipped dependency that cannot be fixed
// right now gets recorded in docs/security/dependency-exceptions.ignore with
// an expiry date, not left silent. That promise decays exactly the way the
// citation checklists do (see cmd/citecheck) — not when someone edits the
// file, but when time passes and nobody looks at it again. A `.ignore` file
// with a stale date in it is worse than no ledger at all, because it still
// reads as "this is being tracked."
//
// This command re-verifies both structural and temporal correctness of that
// ledger on every run:
//
//  1. Every non-comment line parses as the seven-field format the ledger's
//     own header documents (advisory, ecosystem, package, opened, expires,
//     owner, reason), with valid dates and a known ecosystem.
//  2. `opened` is not after `expires`, and the window between them is at most
//     90 days (the deprecation-policy default period, MAINT-01, issue #490).
//  3. `expires` has not already passed relative to when the check runs — the
//     mechanical half of "something surfaces it when the expiry passes".
//
// Exit status 0 means every entry is well-formed and unexpired (including
// zero entries — nothing to report is not a failure); 1 means at least one
// entry is malformed or expired; 2 means the check itself could not run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// exceptionsFile is the ledger this command checks, repo-root-relative.
const exceptionsFile = "docs/security/dependency-exceptions.ignore"

// dateLayout is the ledger's date format for both `opened` and `expires`.
const dateLayout = "2006-01-02"

// maxWindowDays bounds how far out an exception may set its own `expires`
// date, measured from `opened`. 90 days matches the deprecation policy's
// default review period (MAINT-01, issue #490) — one project-wide number for
// "revisit this," not a second one invented here.
const maxWindowDays = 90

// validEcosystems are the Dependabot ecosystems this repo actually tracks
// (.github/dependabot.yml): Go modules, npm/Yarn, Gradle, Docker base
// images, and GitHub Actions. An ecosystem outside this set is a typo, not a
// real dependency surface.
var validEcosystems = map[string]bool{
	"go":             true,
	"npm":            true,
	"gradle":         true,
	"docker":         true,
	"github-actions": true,
}

func main() {
	os.Exit(run(os.Stdout, time.Now().UTC())) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// exception is one parsed, structurally valid ledger entry.
type exception struct {
	line      int
	advisory  string
	ecosystem string
	pkg       string
	opened    time.Time
	expires   time.Time
	owner     string
	reason    string
}

// finding is one problem, reported as `line N: message`. Every finding this
// command produces traces to a specific ledger line — a malformed entry or
// an expired one — so there is no file-level (line 0) variant to carry.
type finding struct {
	line int
	msg  string
}

// run performs the whole check against the real repository and writes a
// summary to w. Split out of main, and root/now injected, so every exit path
// is covered by tests without touching the process environment or the clock.
func run(w io.Writer, now time.Time) int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(w, "depexceptions:", err)
		return 2
	}
	code, err := check(w, root, now)
	if err != nil {
		fmt.Fprintln(w, "depexceptions:", err)
		return 2
	}
	return code
}

// check reads and validates the ledger at root/exceptionsFile.
func check(w io.Writer, root string, now time.Time) (int, error) {
	// #nosec G304 -- exceptionsFile is a fixed internal constant, never
	// request input, joined with root from findRepoRoot walking up from the
	// process's own working directory. Same posture as cmd/citecheck's
	// readRepoFile and cmd/genapibaseline's committed-baseline read path.
	body, err := os.ReadFile(filepath.Join(root, exceptionsFile))
	if os.IsNotExist(err) {
		fmt.Fprintf(w, "%s does not exist — treating as zero recorded exceptions.\n", exceptionsFile)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	entries, parseFindings := parseExceptions(string(body))
	findings := append([]finding{}, parseFindings...)
	findings = append(findings, validate(entries, now)...)

	if len(findings) == 0 {
		fmt.Fprintf(w, "%s: %d recorded exception(s), none expired.\n", exceptionsFile, len(entries))
		return 0, nil
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].line < findings[j].line })
	fmt.Fprintf(w, "\n%d problem(s) in %s:\n", len(findings), exceptionsFile)
	for _, f := range findings {
		fmt.Fprintf(w, "  line %d: %s\n", f.line, f.msg)
	}
	return 1, nil
}

// parseExceptions reads every non-blank, non-comment line of the ledger as a
// seven-field pipe-delimited entry. A line that fails to parse is reported as
// a finding rather than skipped, so a typo cannot silently disappear.
func parseExceptions(body string) ([]exception, []finding) {
	var entries []exception
	var findings []finding
	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lineNo := i + 1
		fields := strings.Split(line, "|")
		for j := range fields {
			fields[j] = strings.TrimSpace(fields[j])
		}
		if len(fields) != 7 {
			findings = append(findings, finding{lineNo, fmt.Sprintf(
				"expected 7 pipe-delimited fields (advisory | ecosystem | package | opened | expires | owner | reason), got %d", len(fields))})
			continue
		}
		e := exception{
			line:      lineNo,
			advisory:  fields[0],
			ecosystem: fields[1],
			pkg:       fields[2],
			owner:     fields[5],
			reason:    fields[6],
		}
		var bad []string
		if e.advisory == "" {
			bad = append(bad, "advisory is empty")
		}
		if !validEcosystems[e.ecosystem] {
			bad = append(bad, fmt.Sprintf("ecosystem %q is not one of go/npm/gradle/docker/github-actions", e.ecosystem))
		}
		if e.pkg == "" {
			bad = append(bad, "package is empty")
		}
		if e.owner == "" {
			bad = append(bad, "owner is empty")
		}
		if e.reason == "" {
			bad = append(bad, "reason is empty")
		}
		opened, err := time.Parse(dateLayout, fields[3])
		if err != nil {
			bad = append(bad, fmt.Sprintf("opened %q is not a YYYY-MM-DD date", fields[3]))
		} else {
			e.opened = opened
		}
		expires, err := time.Parse(dateLayout, fields[4])
		if err != nil {
			bad = append(bad, fmt.Sprintf("expires %q is not a YYYY-MM-DD date", fields[4]))
		} else {
			e.expires = expires
		}
		if len(bad) > 0 {
			findings = append(findings, finding{lineNo, strings.Join(bad, "; ")})
			continue
		}
		entries = append(entries, e)
	}
	return entries, findings
}

// validate checks the temporal rules that need both dates parsed
// successfully: the window is ordered and bounded, and expiry hasn't passed.
func validate(entries []exception, now time.Time) []finding {
	var out []finding
	for _, e := range entries {
		if e.expires.Before(e.opened) {
			out = append(out, finding{e.line, fmt.Sprintf(
				"expires (%s) is before opened (%s)", e.expires.Format(dateLayout), e.opened.Format(dateLayout))})
			continue
		}
		if e.expires.After(e.opened.AddDate(0, 0, maxWindowDays)) {
			out = append(out, finding{e.line, fmt.Sprintf(
				"expires (%s) is more than %d days after opened (%s) — shorten the window or re-record it closer to today",
				e.expires.Format(dateLayout), maxWindowDays, e.opened.Format(dateLayout))})
			continue
		}
		if e.expires.Before(now) {
			out = append(out, finding{e.line, fmt.Sprintf(
				"%s (%s/%s) expired on %s — apply the fix, or renew with a fresh dated entry and an updated reason",
				e.advisory, e.ecosystem, e.pkg, e.expires.Format(dateLayout))})
		}
	}
	return out
}

// findRepoRoot walks up from the working directory until it finds
// backend/go.mod, so the command works from backend/ (the documented `go
// run` invocation) and from backend/cmd/depexceptions/ (`go test`) alike.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been
		// deleted, which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/go.mod)", dir)
		}
		dir = parent
	}
}
