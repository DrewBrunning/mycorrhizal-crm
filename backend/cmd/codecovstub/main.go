// Command codecovstub decides whether a PR's changed files trigger any
// coverage-uploading suite, using the default branch's .github/filters.yaml.
//
// It backs .github/workflows/codecov-patch-stub-workflow-run.yml, which posts
// the no-op codecov/patch/* statuses for PRs whose own pull_request run has
// a read-only token (fork and Dependabot PRs) and so cannot post them from
// unit-tests.yml's codecov-patch-stub job.
//
// Usage: codecovstub <path-to-filters.yaml> < changed-files (one per line)
//
// Prints "no-upload" or "upload" and exits 0. Any problem (missing
// argument, unreadable or incomplete filters file) exits 2 with no verdict
// on stdout, so the caller can never mistake a failure for "no-upload".
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"mycorrhizal/internal/covgate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: codecovstub <filters.yaml> < changed-files")
		return 2
	}
	filters, err := os.ReadFile(args[0]) // #nosec G304 G703 -- CI-supplied path to the checked-out default branch filters file
	if err != nil {
		fmt.Fprintln(stderr, "codecovstub:", err)
		return 2
	}
	var files []string
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			files = append(files, line)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(stderr, "codecovstub:", err)
		return 2 // # pragma: no cover — see above
	}
	upload, err := covgate.TriggersCoverageUpload(filters, files)
	if err != nil {
		fmt.Fprintln(stderr, "codecovstub:", err)
		return 2
	}
	if upload {
		fmt.Fprintln(stdout, "upload")
	} else {
		fmt.Fprintln(stdout, "no-upload")
	}
	return 0
}
