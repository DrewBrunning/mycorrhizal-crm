// Package mdref resolves markdown links, anchors, and repo-file references
// against a repository checkout.
//
// It is the shared resolver for the docs structural test (cmd/docscheck) and
// the agent-context validator (cmd/contextcheck). The two callers iterate
// their own file sets and supply the display name used in findings; the
// resolution rules live here once.
package mdref

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Finding is one unresolved reference. Line is zero for repo-file references,
// which are not line-anchored.
type Finding struct {
	File    string
	Line    int
	Message string
}

// String renders the finding as "file:line: message", or "file: message" when
// Line is zero.
func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Message)
	}
	return fmt.Sprintf("%s: %s", f.File, f.Message)
}

// Link is one markdown link occurrence with its raw destination and start line.
type Link struct {
	Dest string
	Line int
}

// File is one markdown document to resolve references in.
type File struct {
	// Path is the document's path on disk. Relative links resolve against its
	// directory and anchors are read from it.
	Path string
	// Display is the name used in findings (usually repo- or docs-relative).
	Display string
	// Text is the document body.
	Text string
}

// Resolver resolves references against a repository checkout.
type Resolver struct {
	// Root is the repository root.
	Root string
}

// Links checks every markdown link in f: local file targets, anchors, the
// rendered .html form, absolute /path links, and github.com blob/tree links
// mapped back onto the checkout. External, mailto:, tel:, and bare # links are
// ignored.
func (r Resolver) Links(f File) []Finding {
	var findings []Finding
	for _, ref := range ExtractLinks(f.Text) {
		dest := ref.Dest
		if dest == "" {
			continue
		}
		if strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://") {
			if blobPath := GitHubBlobRepoPath(dest); blobPath != "" && !PathExists(r.Root, blobPath) {
				findings = append(findings, Finding{f.Display, ref.Line,
					fmt.Sprintf("github link target missing: %s (no local %s)", dest, blobPath)})
			}
			continue
		}
		if strings.HasPrefix(dest, "mailto:") || strings.HasPrefix(dest, "tel:") || dest == "#" {
			continue
		}
		pathPart, frag := SplitFragment(dest)
		if pathPart == "" {
			if frag != "" && !HeadingIDs(f.Path)[frag] {
				findings = append(findings, Finding{f.Display, ref.Line,
					fmt.Sprintf("anchor not found in this file: #%s", frag)})
			}
			continue
		}
		if strings.HasPrefix(pathPart, "/") {
			cand := filepath.Join(r.Root, filepath.FromSlash(strings.TrimPrefix(pathPart, "/")))
			if !PathExists(r.Root, cand) {
				findings = append(findings, Finding{f.Display, ref.Line,
					fmt.Sprintf("absolute link target missing: %s", dest)})
			}
			continue
		}
		target := filepath.Clean(filepath.Join(filepath.Dir(f.Path), filepath.FromSlash(pathPart)))
		probe := target
		if strings.HasSuffix(probe, ".html") {
			probe = strings.TrimSuffix(probe, ".html") + ".md"
		}
		if !PathExists(r.Root, probe) && !PathExists(r.Root, target) {
			findings = append(findings, Finding{f.Display, ref.Line,
				fmt.Sprintf("link target missing: %s", dest)})
			continue
		}
		if frag != "" && strings.HasSuffix(probe, ".md") && !HeadingIDs(probe)[frag] {
			findings = append(findings, Finding{f.Display, ref.Line,
				fmt.Sprintf("anchor not found: %s", dest)})
		}
	}
	return findings
}

var repoPathRe = mustCompile(`(?m)(?:^|[^A-Za-z0-9_./*-])((?:backend|frontend|android|docs|cmd|\.github|testdata)/[A-Za-z0-9_./{}\-*?]+)`)

// RepoReferences checks path-shaped references to repo files. The leading
// boundary keeps "cmd/actionlint" inside a Go module path from matching, and
// "backend/" inside ".../patch/backend/..." from matching.
func (r Resolver) RepoReferences(f File) []Finding {
	var findings []Finding
	clean := StripFences(f.Text)
	seen := map[string]bool{}
	for _, m := range repoPathRe.FindAllStringSubmatch(clean, -1) {
		cand := strings.TrimRight(m[1], ".,;:])}")
		if strings.ContainsAny(cand, "{}*?") || strings.Contains(cand, "...") {
			continue // a glob, a template, or an intentionally-abbreviated path
		}
		cand = trimLineColSuffix(cand)
		if seen[cand] {
			continue
		}
		seen[cand] = true
		resolved := cand
		if strings.HasPrefix(cand, "cmd/") {
			resolved = "backend/" + cand
		}
		if strings.HasPrefix(cand, "testdata/") {
			continue // ambiguous: backend/testdata, internal/*/testdata, ...
		}
		if FileReferenceResolves(r.Root, resolved) {
			continue
		}
		// Only flag clearly file-like references. Area enumerations in prose
		// ("backend/frontend/android") and a wrapped reference truncated at a
		// line break have no file extension and would be noise.
		if !strings.Contains(filepath.Base(cand), ".") {
			continue
		}
		findings = append(findings, Finding{f.Display, 0,
			fmt.Sprintf("file reference missing: %s", cand)})
	}
	return findings
}

// PathExists reports whether p exists, resolving a relative p against root.
func PathExists(root, p string) bool {
	if p == "" {
		return false
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	_, err := os.Stat(abs)
	return err == nil
}

// GitHubBlobRepoPath maps a github.com/DrewBrunning/mycorrhizal-crm
// blob/tree link to a repo-relative path, or "" if the URL is not one of ours.
func GitHubBlobRepoPath(u string) string {
	const prefix = "https://github.com/DrewBrunning/mycorrhizal-crm/"
	for _, kind := range []string{"blob/main/", "tree/main/", "blob/HEAD/", "tree/HEAD/"} {
		if idx := strings.Index(u, prefix+kind); idx >= 0 {
			return u[idx+len(prefix+kind):]
		}
	}
	return ""
}

// SplitFragment separates a link destination into its path and #fragment.
func SplitFragment(dest string) (pathPart, frag string) {
	if idx := strings.Index(dest, "#"); idx >= 0 {
		return dest[:idx], dest[idx+1:]
	}
	return dest, ""
}

// ExtractLinks returns every markdown link destination in text, ignoring links
// inside fenced code blocks, HTML comments, and inline code spans. Link text
// and destinations may span lines.
func ExtractLinks(text string) []Link {
	cleaned := scrub(text)
	runes := []rune(cleaned)
	n := len(runes)
	var refs []Link
	i := 0
	for i < n {
		if runes[i] != '[' {
			i++
			continue
		}
		labelEnd, ok := matchBracket(runes, i, '[', ']')
		if !ok {
			i++
			continue
		}
		if labelEnd+1 < n && runes[labelEnd+1] == '(' {
			destEnd, ok := matchBracket(runes, labelEnd+1, '(', ')')
			if ok {
				dest := string(runes[labelEnd+2 : destEnd])
				line := 1 + strings.Count(string(runes[:i]), "\n")
				refs = append(refs, Link{Dest: strings.TrimSpace(dest), Line: line})
				i = destEnd + 1
				continue
			}
		}
		i = labelEnd + 1
	}
	return refs
}

// matchBracket finds the index of the matching close of an open rune at open
// index, honoring nesting. Returns the close index and whether one was found.
func matchBracket(runes []rune, open int, o, c rune) (int, bool) {
	depth := 0
	for j := open; j < len(runes); j++ {
		switch runes[j] {
		case o:
			depth++
		case c:
			depth--
			if depth == 0 {
				return j, true
			}
		}
	}
	return 0, false
}

// scrub removes fenced code blocks, HTML comments, and inline code spans from
// the text, preserving newlines so reported line numbers stay accurate.
func scrub(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, len(lines))
	copy(out, lines)
	inFence := false
	inComment := false
	fenceMarker := ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(t, fenceMarker) {
				inFence = false
			}
			out[i] = ""
			continue
		}
		if strings.HasPrefix(t, "```") {
			inFence = true
			fenceMarker = "```"
			out[i] = ""
			continue
		}
		if strings.HasPrefix(t, "~~~") {
			inFence = true
			fenceMarker = "~~~"
			out[i] = ""
			continue
		}
		if strings.Contains(line, "<!--") && !inComment {
			before := line[:strings.Index(line, "<!--")]
			if afterIdx := strings.Index(line, "-->"); afterIdx >= 0 {
				out[i] = before + line[afterIdx+3:]
				continue
			}
			inComment = true
			out[i] = before
			continue
		}
		if inComment {
			if strings.Contains(line, "-->") {
				out[i] = line[strings.Index(line, "-->")+3:]
				inComment = false
			} else {
				out[i] = ""
			}
			continue
		}
	}
	return blankInlineCode(strings.Join(out, "\n"))
}

// blankInlineCode blanks the contents of inline code spans (backtick runs),
// preserving newlines.
func blankInlineCode(text string) string {
	runes := []rune(text)
	out := append([]rune(nil), runes...)
	n := len(runes)
	i := 0
	for i < n {
		if runes[i] != '`' {
			i++
			continue
		}
		runLen := 0
		for i+runLen < n && runes[i+runLen] == '`' {
			runLen++
		}
		closeAt := -1
		j := i + runLen
		for j < n {
			if runes[j] == '`' {
				k := 0
				for j+k < n && runes[j+k] == '`' {
					k++
				}
				if k == runLen {
					closeAt = j
					break
				}
				j += k
				continue
			}
			j++
		}
		end := n
		if closeAt >= 0 {
			end = closeAt + runLen
		}
		for k := i; k < end; k++ {
			if runes[k] != '\n' {
				out[k] = ' '
			}
		}
		if closeAt < 0 {
			break
		}
		i = end
	}
	return string(out)
}

// HeadingIDs returns every anchor id a rendered .md page will carry: a
// kramdown auto-id for each heading plus any literal <a id="..."> markers.
func HeadingIDs(path string) map[string]bool {
	// #nosec G304 -- path is built by the caller from its own file walk or a
	// resolved link target, never request input.
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	ids := map[string]bool{}
	counts := map[string]int{}
	for _, line := range strings.Split(string(body), "\n") {
		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			base := kramdownID(removeBackticks(m[2]))
			if base == "" {
				base = "section"
			}
			counts[base]++
			if counts[base] == 1 {
				ids[base] = true
			} else {
				ids[fmt.Sprintf("%s-%d", base, counts[base]-1)] = true
			}
		}
		if am := anchorIDRe.FindAllStringSubmatch(line, -1); am != nil {
			for _, m := range am {
				ids[m[1]] = true
			}
		}
	}
	return ids
}

// removeBackticks strips the inline-code delimiters from a heading but keeps
// the code's text — that is the text kramdown computes an auto-id from ("The
// `/api/v1` promise" -> "the-apiv1-promise").
func removeBackticks(s string) string {
	return strings.ReplaceAll(s, "`", "")
}

// kramdownID reproduces kramdown's basic_generate_id: keep word characters
// (letters, digits, underscore), hyphen and space; delete everything else;
// spaces become hyphens. Empirically verified against ids this site's own
// cross-links use (e.g. "Rolling back a bad release (N+1 → N)" ->
// rolling-back-a-bad-release-n1--n).
func kramdownID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if isWordRune(r) || r == '-' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(strings.ReplaceAll(b.String(), " ", "-"))
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// StripFences removes fenced code blocks from text.
func StripFences(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	inFence := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// FileReferenceResolves reports whether the path exists, allowing a trailing
// Go package-qualified symbol (backend/internal/fsguard.NetworkFilesystemWarning
// resolves because backend/internal/fsguard is a package).
func FileReferenceResolves(root, p string) bool {
	if PathExists(root, p) {
		return true
	}
	for {
		base := filepath.Base(p)
		dot := strings.LastIndex(base, ".")
		if dot <= 0 {
			return false
		}
		p = p[:len(p)-len(base)+dot]
		if PathExists(root, p) {
			return true
		}
		if !strings.Contains(p, "/") {
			return false
		}
	}
}

// trimLineColSuffix removes a trailing ":<line>" or ":<line>-<col>" that cites
// code, keeping the file path.
func trimLineColSuffix(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx <= 0 {
		return s
	}
	suffix := s[idx+1:]
	if suffix == "" {
		return s
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && r != '-' {
			return s
		}
	}
	return s[:idx]
}

func mustCompile(pat string) *regexp.Regexp { return regexp.MustCompile(pat) }

var (
	mdHeadingRe = mustCompile(`^(#{1,6})\s+(.*)$`)
	anchorIDRe  = mustCompile(`<a\s+id="([^"]+)"`)
)
