// Command changelogcheck lints the [Unreleased] section of CHANGELOG.md
// against docs/changelog-policy.md (issue #2242): known section names in
// policy order, a bold lead phrase per bullet, no sub-bullets, a word limit,
// full-URL issue links, and a cap on Internal. Released sections are not
// checked.
//
// Exit status 0: clean. 1: findings, one "CHANGELOG.md:line: message" each.
// 2: bad usage or unreadable file.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"mycorrhizal/internal/changelog"
)

// maxInternal is the hard limit behind "roughly three" Internal entries.
const maxInternal = 5

func main() { // # pragma: no cover — thin os.Exit wrapper over run
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("changelogcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "../CHANGELOG.md", "path to CHANGELOG.md")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "changelogcheck:", err)
		return 2
	}
	findings := check(string(data))
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

type bullet struct {
	line int
	text string
}

// check returns the findings for the [Unreleased] section of doc.
func check(doc string) []string {
	var out []string
	add := func(line int, format string, a ...any) {
		out = append(out, fmt.Sprintf("CHANGELOG.md:%d: %s", line, fmt.Sprintf(format, a...)))
	}

	in, found := false, false
	section, order := "", 0
	var bullets []bullet
	flush := func() {
		for _, b := range bullets {
			checkBullet(b, add)
		}
		if section == "Internal" && len(bullets) > maxInternal {
			add(bullets[maxInternal].line, "Internal has %d entries; keep it to about three (limit %d)", len(bullets), maxInternal)
		}
		bullets = nil
	}
	for i, raw := range strings.Split(doc, "\n") {
		line := strings.TrimRight(raw, "\r")
		n := i + 1
		switch {
		case strings.HasPrefix(line, "## "):
			if in {
				flush()
				return out
			}
			if strings.HasPrefix(line, "## [Unreleased]") {
				in, found = true, true
			}
		case !in:
		case strings.HasPrefix(line, "### "):
			flush()
			section = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			pos, ok := changelog.SectionPosition(section)
			switch {
			case !ok:
				add(n, "unknown section %q (use %s)", section, changelog.SectionNameList())
			case pos <= order:
				add(n, "section %q is out of order or repeated (order: %s)", section, changelog.SectionNameList())
			default:
				order = pos
			}
		case strings.HasPrefix(line, "- "):
			bullets = append(bullets, bullet{n, line})
		case strings.HasPrefix(strings.TrimLeft(line, " "), "- ") || strings.HasPrefix(strings.TrimLeft(line, " "), "* "):
			add(n, "no sub-bullets; one bullet per change")
		case strings.HasPrefix(line, "  ") && len(bullets) > 0 && strings.TrimSpace(line) != "":
			bullets[len(bullets)-1].text += " " + strings.TrimSpace(line)
		}
	}
	if !found {
		return []string{"CHANGELOG.md:1: no ## [Unreleased] section"}
	}
	flush()
	return out
}

func checkBullet(b bullet, add func(int, string, ...any)) {
	for _, message := range changelog.ValidateBullet(b.text) {
		add(b.line, "%s", message)
	}
}
