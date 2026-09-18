// Command securitycadence is the calendar backstop for the recurring
// security-stewardship obligations that have no event trigger (issues #955,
// #956).
//
// Root secrets have rotation procedures (docs/security/incident-response.md)
// but, before this, no stated interval and no age visibility — a self-hosted
// deployment could keep the same JWT secret, at-rest master key, release App
// key, and Android signing keystore indefinitely with nothing to notice. The
// sensitive-resource access list and the support-window/EOL policy have the
// same shape: real, written down, but reviewed only when something else makes
// someone think of them.
//
// docs/security/security-cadence.md is the register: a human-maintained table
// of {id, obligation, interval_days, last_done}. This command parses that table
// and:
//
//  1. reports every entry and its age,
//  2. exits 1 when any entry is past its interval (so a scheduled job can
//     alarm), and
//  3. exits 2 when the register is malformed or a required obligation is
//     missing — a broken register must fail loudly rather than silently stop
//     covering a secret.
//
// It is deliberately a *reminder*, not a rotator: no automation can know when
// an operator last rotated a key, and the command never touches a secret. The
// discipline is updating the last_done cell when the work is done.
//
// Exit status 0 means every obligation is current; 1 means at least one is
// overdue; 2 means the check itself could not run (missing/unparseable
// register, or a missing required obligation).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// registerFile is the cadence register this command reads, repo-root-relative.
const registerFile = "docs/security/security-cadence.md"

// The machine-readable block markers. Everything between them is the register
// table; the rest of the page is prose.
const (
	beginMarker = "<!-- security-cadence:begin -->"
	endMarker   = "<!-- security-cadence:end -->"
)

// dateLayout is the register's date format.
const dateLayout = "2006-01-02"

// requiredObligations are the ids the register must always carry. They map to
// the classes the adversarial review named (#955's four root secrets and
// #956's two reviews). Dropping one is a deliberate policy decision that must
// change this list in the same PR — not a silent loss of coverage.
var requiredObligations = []string{
	"jwt_secret_key",
	"data_encryption_key",
	"release_app_key",
	"android_signing_key",
	"access_list_review",
	"support_window_review",
}

// Entry is one parsed register row.
type Entry struct {
	ID           string
	Obligation   string
	IntervalDays int
	LastDone     time.Time
	Action       string
}

// Due returns the date the obligation next comes due.
func (e Entry) Due() time.Time { return e.LastDone.AddDate(0, 0, e.IntervalDays) }

// Overdue reports whether now is past the entry's due date.
func (e Entry) Overdue(now time.Time) bool { return now.After(e.Due()) }

// config is the parsed command configuration.
type config struct {
	registerPath string
	now          time.Time
}

func main() {
	os.Exit(mainExit(os.Args[1:], os.Stdout, time.Now().UTC())) // # pragma: no cover — os.Exit terminates the process; tests exercise run()
}

// mainExit parses flags and delegates to run. It is separated from main so the
// flag surface can be exercised without spawning a process.
func mainExit(args []string, w io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("securitycadence", flag.ContinueOnError)
	fs.SetOutput(w)
	register := fs.String("register", registerFile, "register to check, repo-root-relative")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root, err := findRepoRoot(mustGetwd())
	if err != nil {
		fmt.Fprintln(w, "securitycadence:", err)
		return 2
	}
	return run(w, root, config{registerPath: *register, now: now})
}

// run decides the exit status against a known repository root. Tests drive it
// directly with a fixture tree.
func run(w io.Writer, root string, cfg config) int {
	body, err := readRepoFile(root, cfg.registerPath)
	if err != nil {
		fmt.Fprintf(w, "securitycadence: %v\n", err)
		return 2
	}
	entries, err := parseRegister(body)
	if err != nil {
		fmt.Fprintf(w, "securitycadence: %s: %v\n", cfg.registerPath, err)
		return 2
	}
	if missing := missingRequired(entries); len(missing) > 0 {
		fmt.Fprintf(w, "securitycadence: %s is missing required obligation(s): %s\n",
			cfg.registerPath, strings.Join(missing, ", "))
		return 2
	}

	overdue := 0
	for _, e := range entries {
		age := int(cfg.now.Sub(e.LastDone).Hours() / 24)
		if e.Overdue(cfg.now) {
			overdue++
			fmt.Fprintf(w, "OVERDUE  %-22s last done %s (%d days ago; interval %d days, due %s)\n",
				e.ID, e.LastDone.Format(dateLayout), age, e.IntervalDays, e.Due().Format(dateLayout))
			continue
		}
		fmt.Fprintf(w, "ok       %-22s last done %s (%d days ago; interval %d days, due %s)\n",
			e.ID, e.LastDone.Format(dateLayout), age, e.IntervalDays, e.Due().Format(dateLayout))
	}

	if overdue > 0 {
		fmt.Fprintf(w, "\n%d of %d security-stewardship obligation(s) are overdue.\nSee %s for the procedure and update its last_done cell when done.\n",
			overdue, len(entries), cfg.registerPath)
		return 1
	}
	fmt.Fprintf(w, "\nAll %d security-stewardship obligation(s) are current.\n", len(entries))
	return 0
}

// parseRegister extracts the entries from the table between the begin/end
// markers. It is deliberately strict: a row that does not parse fails the
// command rather than being skipped, because a silently-dropped row is a
// silently-unwatched secret.
func parseRegister(body []byte) ([]Entry, error) {
	text := string(body)
	start := strings.Index(text, beginMarker)
	end := strings.Index(text, endMarker)
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("missing %s / %s markers", beginMarker, endMarker)
	}
	block := text[start+len(beginMarker) : end]

	var entries []Entry
	sawHeader := false
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Strip the leading and trailing pipe, then split the cells.
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if len(cells) != 5 {
			return nil, fmt.Errorf("register row has %d cells, want 5 (id | obligation | interval_days | last_done | action): %q", len(cells), line)
		}
		if !sawHeader {
			if cells[0] == "id" {
				sawHeader = true
			}
			continue
		}
		if isSeparatorRow(cells) {
			continue
		}

		id := strings.Trim(cells[0], "`")
		if id == "" {
			return nil, fmt.Errorf("register row has an empty id: %q", line)
		}
		interval, err := strconv.Atoi(cells[2])
		if err != nil || interval <= 0 {
			return nil, fmt.Errorf("register row %q has a non-positive/non-integer interval_days %q", id, cells[2])
		}
		lastDone, err := time.Parse(dateLayout, cells[3])
		if err != nil {
			return nil, fmt.Errorf("register row %q has last_done %q, not %s", id, cells[3], dateLayout)
		}
		entries = append(entries, Entry{
			ID:           id,
			Obligation:   cells[1],
			IntervalDays: interval,
			LastDone:     lastDone,
			Action:       cells[4],
		})
	}
	if !sawHeader {
		return nil, fmt.Errorf("register has no header row")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("register has no entries")
	}
	return entries, nil
}

// isSeparatorRow reports whether every cell is a markdown rule (`---`, `:--:`).
func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, ":- ") != "" {
			return false
		}
	}
	return true
}

// missingRequired returns the required obligation ids not present in entries.
func missingRequired(entries []Entry) []string {
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.ID] = true
	}
	var missing []string
	for _, id := range requiredObligations {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// findRepoRoot walks up from start until it finds the register the command
// exists to verify, so it works from backend/ (go run) and from
// backend/cmd/securitycadence/ (go test) alike.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, registerFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for %s)", start, registerFile)
		}
		dir = parent
	}
}

// mustGetwd returns the working directory, falling back to "." so a resolution
// failure surfaces as the normal "no repository root" error rather than a
// panic.
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "."
	}
	return wd
}

// readRepoFile reads a repository-relative file, refusing anything that would
// escape root. Every read here comes from the fixed registerFile default or an
// operator-supplied `-register`, so the traversal check keeps that flag from
// becoming an arbitrary-file reader.
func readRepoFile(root, rel string) ([]byte, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("refusing to read %q — outside the repository root", rel)
	}
	// #nosec G304 -- clean is repo-relative and traversal-checked immediately
	// above; root comes from findRepoRoot walking up from the working
	// directory, never from request input. Same posture as cmd/asvsstale's
	// readRepoFile.
	return os.ReadFile(filepath.Join(root, clean))
}
