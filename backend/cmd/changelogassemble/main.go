// Command changelogassemble folds changelog.d/ fragments into the
// [Unreleased] section of CHANGELOG.md (docs/changelog-policy.md, REL-05).
//
// By default it is a check: every fragment's exact bullet must already be in
// [Unreleased]. release.yml and promote-rc.yml run it before the
// changelogsection gate, so a release cannot ship unassembled notes. -write
// inserts missing bullets under their canonical section in policy order,
// preserving the rest of [Unreleased] (including the > **Upgrade** blockquote)
// and the existing bullets' order, then deletes the folded fragment files
// unless -keep. A second -write is a no-op.
//
// Exit status 0: clean or assembled. 1: findings or unassembled fragments.
// 2: bad usage, unreadable file, or no [Unreleased] section.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"mycorrhizal/internal/changelog"
)

// osExit is os.Exit through a seam so tests can drive main() itself without
// killing the test process.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("changelogassemble", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "../CHANGELOG.md", "path to CHANGELOG.md")
	dir := fs.String("dir", "../changelog.d", "path to the changelog.d directory")
	write := fs.Bool("write", false, "insert missing fragment bullets and delete the folded fragments")
	keep := fs.Bool("keep", false, "keep fragment files after -write")
	check := fs.Bool("check", false, "check mode (the default): every fragment must already be assembled")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch {
	case *check && *write:
		fmt.Fprintln(stderr, "changelogassemble: -check and -write are mutually exclusive")
		return 2
	case *keep && !*write:
		fmt.Fprintln(stderr, "changelogassemble: -keep requires -write")
		return 2
	}

	fragments, findings, err := changelog.LoadDir(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "changelogassemble:", err)
		return 2
	}
	if len(findings) > 0 {
		for _, finding := range findings {
			fmt.Fprintln(stdout, finding.String())
		}
		return 1
	}

	data, err := os.ReadFile(*file) // #nosec G304 -- file is the operator's -file flag
	if err != nil {
		fmt.Fprintln(stderr, "changelogassemble:", err)
		return 2
	}
	lines, eol := splitLines(string(data))
	missing, err := missingFragments(lines, fragments)
	if err != nil {
		fmt.Fprintln(stderr, "changelogassemble:", err)
		return 2
	}

	if !*write {
		if len(missing) > 0 {
			for _, fragment := range missing {
				fmt.Fprintf(stdout, "%s: bullet is missing from CHANGELOG.md [Unreleased]\n", fragment.Path)
			}
			fmt.Fprintln(stdout, "run 'cd backend && go run ./cmd/changelogassemble -write' and commit CHANGELOG.md with the deleted fragments")
			return 1
		}
		return 0
	}

	if len(missing) > 0 {
		var inserted int
		lines, inserted, err = fold(lines, missing)
		if err != nil {
			fmt.Fprintln(stderr, "changelogassemble:", err)
			return 2
		}
		doc := strings.Join(lines, eol)
		if doc != string(data) {
			if err := writeFile(*file, doc); err != nil {
				fmt.Fprintln(stderr, "changelogassemble:", err)
				return 2
			}
		}
		fmt.Fprintf(stdout, "changelogassemble: inserted %d bullet(s) into CHANGELOG.md [Unreleased]\n", inserted)
	}

	if *keep {
		return 0
	}
	for _, fragment := range fragments {
		if err := os.Remove(fragment.Path); err != nil {
			fmt.Fprintln(stderr, "changelogassemble:", err)
			return 2
		}
	}
	fmt.Fprintf(stdout, "changelogassemble: deleted %d fragment(s)\n", len(fragments))
	return 0
}

// splitLines normalises CRLF to LF for editing and reports the line ending to
// write back, so a CRLF checkout stays CRLF.
func splitLines(doc string) ([]string, string) {
	eol := "\n"
	if strings.Contains(doc, "\r\n") {
		eol = "\r\n"
	}
	parts := strings.Split(doc, "\n")
	for i := range parts {
		parts[i] = strings.TrimRight(parts[i], "\r")
	}
	return parts, eol
}

func writeFile(path, doc string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, []byte(doc), mode) // #nosec G304 G703 -- path is the operator's -file flag or its sibling
}

// unreleased returns [start, end) line indices of the ## [Unreleased] section.
func unreleased(lines []string) (int, int, error) {
	start := -1
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		if start >= 0 {
			return start, i, nil
		}
		if strings.HasPrefix(line, "## [Unreleased]") {
			start = i
		}
	}
	if start < 0 {
		return 0, 0, changelog.ErrMissingUnreleased
	}
	return start, len(lines), nil
}

func bulletPresent(lines []string, bullet string) bool {
	start, end, err := unreleased(lines)
	if err != nil {
		return false
	}
	want := strings.TrimSpace(bullet)
	for _, raw := range lines[start+1 : end] {
		if strings.TrimSpace(strings.TrimRight(raw, "\r")) == want {
			return true
		}
	}
	return false
}

// missingFragments returns the fragments whose exact bullet is not in
// [Unreleased], in deterministic fragment order.
func missingFragments(lines []string, fragments []changelog.Fragment) ([]changelog.Fragment, error) {
	if _, _, err := unreleased(lines); err != nil {
		return nil, err
	}
	var missing []changelog.Fragment
	for _, fragment := range fragments {
		if !bulletPresent(lines, fragment.Bullet) {
			missing = append(missing, fragment)
		}
	}
	return missing, nil
}

// fold inserts every missing bullet under its canonical section, creating the
// section in policy order when absent. Duplicate bullet text is inserted once;
// the returned count is the number of bullets actually inserted.
func fold(lines []string, missing []changelog.Fragment) ([]string, int, error) {
	seen := make(map[string]bool, len(missing))
	inserted := 0
	var err error
	for _, section := range changelog.Sections {
		var bullets []string
		for _, fragment := range missing {
			if fragment.Section.Key != section.Key || seen[fragment.Bullet] {
				continue
			}
			bullets = append(bullets, fragment.Bullet)
			seen[fragment.Bullet] = true
			inserted++
		}
		if len(bullets) == 0 {
			continue
		}
		lines, err = insertIntoSection(lines, section, bullets)
		if err != nil {
			return nil, 0, err
		}
	}
	return lines, inserted, nil
}

type sectionRange struct {
	section      changelog.Section
	heading      int
	contentStart int
	contentEnd   int
}

// insertIntoSection adds bullets to section, creating the heading when it is
// absent. Only lines inside [Unreleased] are touched.
func insertIntoSection(lines []string, section changelog.Section, bullets []string) ([]string, error) {
	start, end, err := unreleased(lines)
	if err != nil {
		return nil, err
	}

	var ranges []sectionRange
	for i := start + 1; i < end; i++ {
		line := strings.TrimRight(lines[i], "\r")
		if !strings.HasPrefix(line, "### ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "### "))
		sec, ok := changelog.SectionForName(name)
		if !ok {
			sec = changelog.Section{Name: name}
		}
		if len(ranges) > 0 {
			ranges[len(ranges)-1].contentEnd = i
		}
		ranges = append(ranges, sectionRange{section: sec, heading: i, contentStart: i + 1})
	}
	if len(ranges) > 0 {
		ranges[len(ranges)-1].contentEnd = end
	}

	for _, r := range ranges {
		if r.section.Key == section.Key && r.section.Name == section.Name {
			at := lastNonBlank(lines, r.contentStart, r.contentEnd)
			block := make([]string, 0, len(bullets)+2)
			if at == r.contentStart {
				block = append(block, "")
			}
			block = append(block, bullets...)
			if at == r.contentEnd && at < len(lines) && strings.TrimSpace(lines[at]) != "" {
				block = append(block, "")
			}
			out := make([]string, 0, len(lines)+len(block))
			out = append(out, lines[:at]...)
			out = append(out, block...)
			out = append(out, lines[at:]...)
			return out, nil
		}
	}

	target := changelog.SectionIndex(section)
	insertAt := -1
	for _, r := range ranges {
		if changelog.SectionIndex(r.section) > target {
			insertAt = r.heading
			break
		}
	}
	block := []string{"### " + section.Name, ""}
	block = append(block, bullets...)

	if insertAt >= 0 {
		block = append(block, "")
		out := make([]string, 0, len(lines)+len(block))
		out = append(out, lines[:insertAt]...)
		out = append(out, block...)
		out = append(out, lines[insertAt:]...)
		return out, nil
	}

	at := lastNonBlank(lines, start+1, end)
	block = append([]string{""}, block...)
	if at < len(lines) && strings.TrimSpace(lines[at]) != "" {
		block = append(block, "")
	}
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:at]...)
	out = append(out, block...)
	out = append(out, lines[at:]...)
	return out, nil
}

// lastNonBlank returns the index just after the last non-blank line in
// [start, end), or start when the range is empty.
func lastNonBlank(lines []string, start, end int) int {
	at := start
	for i := start; i < end; i++ {
		if strings.TrimSpace(strings.TrimRight(lines[i], "\r")) != "" {
			at = i + 1
		}
	}
	return at
}
