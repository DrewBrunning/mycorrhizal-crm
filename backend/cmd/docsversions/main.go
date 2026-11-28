// Command docsversions decides which release tags get a published copy of the
// documentation site (issue #2244).
//
// Reads release tags, one per line, on stdin and prints a JSON array on stdout:
// one entry per released minor (its newest patch, release candidates and
// anything that is not a vX.Y.Z tag skipped, nothing older than -floor), newest
// first, plus the unreleased docs of main. The newest release is served at the
// site root ("path": ""); every other version lives under its own path ("1.7/"),
// and main's docs live under "next/", so a reader can see what is coming before
// it ships. Before the first release at or above -floor, main is the whole site.
// scripts/build-docs-versions.sh turns this list into one Jekyll build per entry
// and docs/_includes/header_custom.html reads the same list as /versions.json.
//
// Usage: git tag -l 'v*' | go run ./cmd/docsversions [-floor v1.7.0]
//
// Exit status 0: list written. 2: bad usage.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// Version is one published copy of the docs.
type Version struct {
	Tag        string `json:"tag"`
	Label      string `json:"label"`
	Path       string `json:"path"`
	Latest     bool   `json:"latest"`
	Unreleased bool   `json:"unreleased,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("docsversions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	floor := fs.String("floor", "v1.7.0", "oldest release whose docs are published")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !semver.IsValid(*floor) {
		fmt.Fprintf(stderr, "docsversions: -floor %q is not a semantic version like v1.7.0\n", *floor)
		return 2
	}
	var tags []string
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		tags = append(tags, strings.TrimSpace(sc.Text()))
	}
	versions := Select(tags, *floor)
	next := Version{Tag: "main", Label: "next", Path: "next/", Unreleased: true}
	if len(versions) == 0 {
		next.Path, next.Latest = "", true
	}
	versions = append(versions, next)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(versions); err != nil {
		fmt.Fprintln(stderr, "docsversions: write:", err) // # pragma: no cover — stdout write failure
		return 2                                          // # pragma: no cover
	}
	return 0
}

// Select returns the newest patch of every released minor at or above floor,
// newest first, the first marked Latest and served at the site root.
func Select(tags []string, floor string) []Version {
	newest := map[string]string{} // MajorMinor -> newest vX.Y.Z
	for _, t := range tags {
		if !semver.IsValid(t) || semver.Prerelease(t) != "" || semver.Build(t) != "" || strings.Count(t, ".") != 2 {
			continue
		}
		if semver.Compare(t, floor) < 0 {
			continue
		}
		mm := semver.MajorMinor(t)
		if cur, ok := newest[mm]; !ok || semver.Compare(t, cur) > 0 {
			newest[mm] = t
		}
	}
	out := make([]Version, 0, len(newest))
	for _, t := range newest {
		out = append(out, Version{Tag: t, Label: strings.TrimPrefix(semver.MajorMinor(t), "v")})
	}
	sort.Slice(out, func(i, j int) bool { return semver.Compare(out[i].Tag, out[j].Tag) > 0 })
	for i := range out {
		if i == 0 {
			out[i].Latest = true
			continue
		}
		out[i].Path = out[i].Label + "/"
	}
	return out
}
