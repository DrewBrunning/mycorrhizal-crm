// Command changelogfragmentcheck validates the changelog.d/ fragments that
// carry [Unreleased] entries (docs/changelog-policy.md, REL-05). Each fragment
// is one file named <section>-<issue>[-<n>].md containing exactly one
// Keep-a-Changelog bullet; the issue number in the filename must appear in an
// issues/<n> URL in the bullet. README.md and non-Markdown files are ignored.
//
// It runs in CI and the pre-commit hook, so a malformed fragment fails before
// it reaches the release-time assembly step.
//
// Exit status 0: clean. 1: findings, one "path:line: message" each.
// 2: bad usage or an unreadable/missing directory.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/internal/changelog"
)

// osExit is os.Exit through a seam so tests can drive main() itself without
// killing the test process.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("changelogfragmentcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "../changelog.d", "path to the changelog.d directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_, findings, err := changelog.LoadDir(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "changelogfragmentcheck:", err)
		return 2
	}
	for _, finding := range findings {
		fmt.Fprintln(stdout, finding.String())
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}
