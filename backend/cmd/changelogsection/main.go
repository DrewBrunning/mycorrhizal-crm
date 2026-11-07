// Command changelogsection prints one release section of CHANGELOG.md
// (docs/changelog-policy.md, issue #2242). docker-publish.yml's create-release
// job and promote-rc.yml copy the output into the GitHub Release body;
// release.yml and promote-rc.yml run it before tagging so a release cannot be
// cut without a written section.
//
// Usage: changelogsection -section 1.6.0 [-fallback Unreleased] [-link-base URL] [-file ../CHANGELOG.md]
// The section name is the text inside the brackets of its "## [name]" heading.
// -fallback names the section to print when -section is absent (a release
// candidate prints "Unreleased" until the maintainer renames it). -link-base
// turns relative Markdown links, which resolve only inside the repository,
// into absolute ones for the Release body. The heading line is not printed.
//
// Exit status 0: section written. 2: bad usage, unreadable file, or the
// section is missing or empty.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("changelogsection", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "../CHANGELOG.md", "path to CHANGELOG.md")
	name := fs.String("section", "", "section name, e.g. 1.6.0 or Unreleased")
	fallback := fs.String("fallback", "", "section to print when -section is absent")
	linkBase := fs.String("link-base", "", "prefix for relative link targets, e.g. https://github.com/o/r/blob/v1.6.0/")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *name == "" {
		fmt.Fprintln(stderr, "changelogsection: -section is required")
		return 2
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "changelogsection:", err)
		return 2
	}
	body, err := section(string(data), *name)
	if errors.Is(err, errMissing) && *fallback != "" {
		body, err = section(string(data), *fallback)
	}
	if err != nil {
		fmt.Fprintln(stderr, "changelogsection:", err)
		return 2
	}
	if *linkBase != "" {
		body = absoluteLinks(body, *linkBase)
	}
	if _, err := fmt.Fprintln(stdout, body); err != nil {
		fmt.Fprintln(stderr, "changelogsection: write:", err)
		return 2
	}
	return 0
}

var errMissing = errors.New("no section")

// section returns the body of the "## [name]" section, trimmed, without its
// heading. It errors when the section is absent (wrapping errMissing) or has
// no content beyond blank lines and "###" sub-headings.
func section(changelog, name string) (string, error) {
	prefix := "## [" + name + "]"
	var body []string
	in, found := false, false
	for _, line := range strings.Split(changelog, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "## [") {
			if in {
				break
			}
			if line == prefix || strings.HasPrefix(line, prefix+" ") {
				in, found = true, true
			}
			continue
		}
		if in {
			body = append(body, line)
		}
	}
	if !found {
		return "", fmt.Errorf("%w %q in changelog", errMissing, name)
	}
	content := false
	for _, l := range body {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			content = true
			break
		}
	}
	if !content {
		return "", fmt.Errorf("section %q is empty", name)
	}
	return strings.TrimSpace(strings.Join(body, "\n")), nil
}

var (
	linkRe   = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// absoluteLinks prefixes every relative inline link target in md with base.
// CHANGELOG.md sits at the repository root, so a relative target is a
// repository path and a bare "#anchor" points into CHANGELOG.md itself.
func absoluteLinks(md, base string) string {
	return linkRe.ReplaceAllStringFunc(md, func(m string) string {
		target := linkRe.FindStringSubmatch(m)[1]
		switch {
		case schemeRe.MatchString(target):
			return m
		case strings.HasPrefix(target, "#"):
			target = "CHANGELOG.md" + target
		}
		return "](" + base + strings.TrimPrefix(target, "/") + ")"
	})
}
