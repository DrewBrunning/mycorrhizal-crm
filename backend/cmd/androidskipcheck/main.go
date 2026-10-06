// Command androidskipcheck is the skip allowlist gate for the Android
// instrumented E2E suite (issue #1483).
//
// Two modes, both comparing against android/e2e-expected-skips.txt:
//
//	androidskipcheck -results <dir> [-summary <file>]
//	    parse every JUnit XML under <dir> (AGP's
//	    app/build/outputs/androidTest-results/connected) and fail unless the set
//	    of skipped tests equals the allowlist exactly. Run by the
//	    `android-e2e` jobs in android-tests.yml.
//	androidskipcheck -static
//	    verify every allowlist entry resolves to a real test in
//	    android/app/src/androidTest. Runs on every PR (docs-citations job),
//	    since the emulator suite itself does not.
//
// Exit 0: clean. Exit 1: at least one finding. Exit 2: could not run.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/androidskips"
)

const (
	allowlistRel   = "android/e2e-expected-skips.txt"
	androidTestRel = "android/app/src/androidTest"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("androidskipcheck", flag.ContinueOnError)
	fl.SetOutput(stderr)
	results := fl.String("results", "", "directory of JUnit XML results to check against the allowlist")
	summary := fl.String("summary", "", "file to append the markdown skip report to (e.g. $GITHUB_STEP_SUMMARY)")
	static := fl.Bool("static", false, "check that every allowlist entry resolves to a real androidTest method")
	root := fl.String("root", "", "repository root (default: found by walking up to backend/go.mod)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if (*results == "") == !*static {
		fmt.Fprintln(stderr, "androidskipcheck: pass exactly one of -results <dir> or -static")
		return 2
	}

	repo := *root
	if repo == "" {
		var err error
		if repo, err = findRepoRoot(); err != nil {
			fmt.Fprintln(stderr, "androidskipcheck:", err) // # pragma: no cover — only reachable outside the repository
			return 2                                       // # pragma: no cover
		}
	}

	// #nosec G304 -- fixed repo-relative path
	f, err := os.Open(filepath.Join(repo, allowlistRel))
	if err != nil {
		fmt.Fprintln(stderr, "androidskipcheck:", err)
		return 2
	}
	defer func() { _ = f.Close() }() // read-only handle
	allow, findings := androidskips.ParseAllowlist(f)

	if *static {
		findings = append(findings, androidskips.CheckSources(allow, filepath.Join(repo, androidTestRel))...)
		return report(stdout, findings, fmt.Sprintf("androidskipcheck -static OK: %d expected skip(s) all resolve to real tests", len(allow)))
	}

	docs, err := readResults(*results)
	if err != nil {
		fmt.Fprintln(stderr, "androidskipcheck:", err)
		return 2
	}
	res, err := androidskips.ParseResults(docs)
	if err != nil {
		fmt.Fprintln(stderr, "androidskipcheck:", err)
		return 2
	}
	findings = append(findings, androidskips.Check(allow, res)...)
	if *summary != "" {
		if err := appendFile(*summary, androidskips.Summary(allow, res, findings)); err != nil {
			fmt.Fprintln(stderr, "androidskipcheck:", err)
			return 2
		}
	}
	return report(stdout, findings, fmt.Sprintf("androidskipcheck OK: %d test(s), %d skipped, all expected", res.Total, len(res.Skipped)))
}

func report(w io.Writer, findings []string, ok string) int {
	if len(findings) == 0 {
		fmt.Fprintln(w, ok)
		return 0
	}
	for _, f := range findings {
		fmt.Fprintln(w, f)
	}
	fmt.Fprintf(w, "\n%d android-skip-gate finding(s).\n", len(findings))
	return 1
}

func readResults(dir string) (map[string][]byte, error) {
	docs := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".xml") {
			return nil
		}
		// #nosec G304 G122 -- p is a file found by walking the caller-supplied results directory
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr // # pragma: no cover — a just-listed file becoming unreadable is a filesystem race
		}
		docs[p] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func appendFile(path, text string) error {
	// #nosec G302 G304 -- path is the CI-provided step-summary file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close() // # pragma: no cover — write error on an open regular file
		return err    // # pragma: no cover
	}
	return f.Close()
}

// findRepoRoot walks up from the working directory to backend/go.mod, the
// sentinel the other cmd/*check tools use.
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
			return "", fmt.Errorf("backend/go.mod not found above the working directory")
		}
		dir = parent
	}
}
