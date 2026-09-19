// Command residualrisk emits the release "what are we shipping with" statement
// (issue #953).
//
// release-metadata.json records which gates passed; this command supplies the
// other half — the open accept-with-reason items across the project's
// justified ignore lists, the dependency-advisory exceptions that are still
// open (and how soon each expires), and the current ASVS/MASVS
// documented-exception counts. docker-publish.yml's create-release runs it
// while assembling release-metadata.json and merges the JSON under a
// `residual_risk` key, so a reader of a release artifact has the accepted
// residual in one place (ADR 0021, issue #1163 -- moved there from release.yml).
//
// It is a pure reader: it adds no policy and owns no number the repository
// does not already record.
//
// Exit status 0: JSON written. 2: a declared source is missing or malformed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"mycorrhizal/internal/residualrisk"
)

func main() {
	os.Exit(mainExit(os.Args[1:], os.Stdout, time.Now().UTC())) // # pragma: no cover — os.Exit terminates the process; tests exercise run()
}

// mainExit parses flags and delegates to run. It is separated from main so the
// flag surface can be exercised without spawning a process.
func mainExit(args []string, w io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("residualrisk", flag.ContinueOnError)
	fs.SetOutput(w)
	out := fs.String("out", "", "write JSON here instead of stdout")
	asOf := fs.String("now", "", "evaluate exception expiry as of this date (YYYY-MM-DD; default: now)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *asOf != "" {
		d, err := time.Parse("2006-01-02", *asOf)
		if err != nil {
			fmt.Fprintln(w, "residualrisk: -now must be YYYY-MM-DD:", err)
			return 2
		}
		now = d
	}
	root, err := findRepoRoot(mustGetwd())
	if err != nil {
		fmt.Fprintln(w, "residualrisk:", err)
		return 2
	}
	return run(w, root, *out, now)
}

// run writes the summary as indented JSON to outPath, or to w when outPath is
// empty. Tests drive it directly with a fixture tree.
func run(w io.Writer, root, outPath string, now time.Time) int {
	summary, err := residualrisk.Summarize(root, now)
	if err != nil {
		fmt.Fprintln(w, "residualrisk:", err)
		return 2
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintln(w, "residualrisk: encode:", err) // # pragma: no cover — MarshalIndent of a plain struct cannot fail
		return 2                                      // # pragma: no cover
	}
	if outPath == "" {
		fmt.Fprintln(w, string(encoded))
		return 0
	}
	if err := os.WriteFile(outPath, append(encoded, '\n'), 0o600); err != nil {
		fmt.Fprintln(w, "residualrisk: write:", err)
		return 2
	}
	return 0
}

// findRepoRoot walks up from start until it finds the dated attestation the
// command reads alongside its other sources, so it works from backend/ (go run)
// and from backend/cmd/residualrisk/ (go test) alike.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, residualrisk.ReportFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for %s)", start, residualrisk.ReportFile)
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
		// # pragma: no cover — Getwd fails only when the cwd has been deleted.
		return "."
	}
	return wd
}
