// Command jacoco2lcov converts the Android aggregated JaCoCo XML report to
// lcov for the Codecov `android` flag, and prints a JaCoCo-vs-lcov
// reconciliation so the conversion's effect on the numbers is measured on
// every run (internal/jacocolcov has the why and the conversion rules).
//
// Normal workflow (CI runs exactly this in android-tests.yml's `test` job):
//
//	cd android && ./gradlew jacocoTestReportAggregated
//	cd backend && go run ./cmd/jacoco2lcov
//
// The reconciliation goes to stdout, and is also appended as Markdown to
// -summary (CI passes $GITHUB_STEP_SUMMARY). Ambiguous files are printed as
// GitHub `::warning::` annotations.
//
// Exit status 0: lcov written. 1: the lcov output would hold no files, which
// means the report or the source index is broken (an empty upload is exactly
// what Codecov rejects). 2: the command could not run.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/jacocolcov"
)

const (
	defaultReportRel = "android/build/reports/jacoco/jacocoTestReportAggregated/jacocoTestReportAggregated.xml"
	defaultOutRel    = "android/build/reports/jacoco/jacocoTestReportAggregated/jacocoTestReportAggregated.lcov"
	sourceDir        = "android"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jacoco2lcov", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "repository root (default: found by walking up to backend/go.mod)")
	in := fs.String("in", "", "aggregated JaCoCo XML (default: <root>/"+defaultReportRel+")")
	out := fs.String("out", "", "lcov output path (default: <root>/"+defaultOutRel+")")
	summary := fs.String("summary", "", "append the Markdown reconciliation to this file (e.g. $GITHUB_STEP_SUMMARY)")
	if err := fs.Parse(args); err != nil {
		return 2 // flag.ContinueOnError already printed the usage error itself
	}

	repoRoot := *root
	if repoRoot == "" {
		r, err := findRepoRoot()
		if err != nil {
			fmt.Fprintln(stderr, "jacoco2lcov:", err) // # pragma: no cover — only reachable outside the repository
			return 2                                  // # pragma: no cover
		}
		repoRoot = r
	}
	inPath, outPath := *in, *out
	if inPath == "" {
		inPath = filepath.Join(repoRoot, defaultReportRel)
	}
	if outPath == "" {
		outPath = filepath.Join(repoRoot, defaultOutRel)
	}

	f, err := os.Open(inPath) // #nosec G304 -- a CLI flag pointing at this run's own report, not user input
	if err != nil {
		fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err)
		return 2
	}
	files, err := jacocolcov.ParseReport(f)
	_ = f.Close() // read-only handle; a close error cannot lose data
	if err != nil {
		fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err)
		return 2
	}

	index, err := jacocolcov.IndexSources(os.DirFS(repoRoot), sourceDir)
	if err != nil {
		fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err)
		return 2
	}

	var lcov bytes.Buffer
	res, err := jacocolcov.Convert(files, index, &lcov)
	if err != nil {
		fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err) // # pragma: no cover — Convert only fails on a write error, and a bytes.Buffer never returns one
		return 2                                      // # pragma: no cover
	}

	fmt.Fprint(stdout, res.Summary())
	for _, key := range res.Ambiguous {
		fmt.Fprintf(stdout, "::warning::jacoco2lcov: %s matches more than one file under %s/; dropped from the lcov upload\n", key, sourceDir)
	}
	if *summary != "" {
		if err := appendFile(*summary, "### Android coverage: JaCoCo → lcov\n\n"+res.Summary()+"\n"); err != nil {
			fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err)
			return 2
		}
	}

	if res.LCOV.Files == 0 {
		fmt.Fprintf(stderr, "jacoco2lcov: FAIL -- no source file in %s resolved to a file under %s/; refusing to write an empty lcov report\n", inPath, sourceDir)
		return 1
	}
	if err := os.WriteFile(outPath, lcov.Bytes(), 0o600); err != nil {
		fmt.Fprintf(stderr, "jacoco2lcov: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "jacoco2lcov: wrote %s (%d files)\n", outPath, res.LCOV.Files)
	return 0
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- CI's own step-summary path
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// findRepoRoot walks up from the working directory to backend/go.mod, the
// sentinel the other cmd/* tools use.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err // # pragma: no cover — Getwd fails only when the cwd has been deleted
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "backend", "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("backend/go.mod not found above the working directory") // # pragma: no cover — only reachable outside the repository
		}
		dir = parent
	}
}
