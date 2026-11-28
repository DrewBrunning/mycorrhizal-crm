// Command docssitelinks fixes the relative links in one built version of the
// docs site so none of them is dead (issue #2244).
//
// scripts/build-docs-versions.sh runs it on every version after its Jekyll
// build. The site builds with plain Jekyll, which leaves Markdown links as
// written, and each release is built from its own tag, so a release's pages
// keep whatever links shipped with it. For each relative href:
//
//   - a link that already resolves on the site (a rendered page, an asset, a
//     directory index) is kept;
//   - a .md link to a page the site rendered becomes its .html address (a
//     release built before docscheck required .html links);
//   - anything else is a repository file or an unpublished record (ADRs,
//     security records, specs, repo-root files) and points at its source on
//     GitHub at the ref the version was built from. An unpublished .md that
//     Jekyll copied verbatim goes there too: the copy would show raw Markdown.
//
// Usage (from backend/):
//
//	go run ./cmd/docssitelinks -site DIR -src DIR -ref REF
//
// -site is the built version, -src the docs/ tree it was built from, -ref the
// tag (or main) it was built from.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const repoBlobPrefix = "https://github.com/DrewBrunning/mycorrhizal-crm/blob/"

var (
	hrefRe   = regexp.MustCompile(`href="([^"]*)"`)
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	refRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

func run(args []string, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("docssitelinks", flag.ContinueOnError)
	fset.SetOutput(stderr)
	site := fset.String("site", "", "built site directory of one docs version")
	src := fset.String("src", "", "docs/ source directory that version was built from")
	ref := fset.String("ref", "", "git ref (tag or branch) the version was built from")
	if err := fset.Parse(args); err != nil {
		return 2
	}
	if *site == "" || *src == "" || !refRe.MatchString(*ref) || strings.Contains(*ref, "..") {
		fmt.Fprintln(stderr, "docssitelinks: -site, -src and a branch or tag -ref are required")
		return 2
	}
	r := rewriter{site: *site, src: *src, blobBase: repoBlobPrefix + *ref + "/"}
	// Reads and writes go through an os.Root so a symlink in the built tree
	// cannot point them outside it.
	root, err := os.OpenRoot(*site)
	if err != nil {
		fmt.Fprintln(stderr, "docssitelinks:", err)
		return 2
	}
	defer func() { _ = root.Close() }()
	pages, changed := 0, 0
	err = filepath.WalkDir(*site, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		rel, err := filepath.Rel(*site, p)
		if err != nil {
			return err // # pragma: no cover — p comes from walking site, so it is always under it
		}
		body, err := root.ReadFile(rel)
		if err != nil {
			return err // # pragma: no cover — the file was listed a moment ago; a racing deletion is not a normal state
		}
		out, n := r.rewrite(string(body), filepath.ToSlash(rel))
		pages++
		if n == 0 {
			return nil
		}
		changed += n
		return root.WriteFile(rel, []byte(out), 0o644) // #nosec G306 -- a public web page in the Pages artifact
	})
	if err != nil {
		fmt.Fprintln(stderr, "docssitelinks:", err)
		return 2
	}
	fmt.Fprintf(stdout, "docssitelinks: rewrote %d link(s) across %d page(s) in %s\n", changed, pages, *site)
	return 0
}

// rewriter resolves one built version's relative links.
type rewriter struct {
	site, src, blobBase string
}

// rewrite returns page (the HTML at pageRel, a slash path relative to the
// site root) with every dead relative href fixed, and how many it changed.
func (r rewriter) rewrite(page, pageRel string) (string, int) {
	n := 0
	out := hrefRe.ReplaceAllStringFunc(page, func(m string) string {
		href := hrefRe.FindStringSubmatch(m)[1]
		fixed := r.fix(href, path.Dir(pageRel))
		if fixed == href {
			return m
		}
		n++
		return `href="` + fixed + `"`
	})
	return out, n
}

func (r rewriter) fix(href, dir string) string {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "/") || schemeRe.MatchString(href) {
		return href
	}
	p, frag, hasFrag := strings.Cut(href, "#")
	p, _, _ = strings.Cut(p, "?")
	if p == "" {
		return href
	}
	suffix := ""
	if hasFrag {
		suffix = "#" + frag
	}
	target := path.Clean(path.Join(dir, p)) // relative to the docs root; may climb out of it
	switch {
	case strings.HasSuffix(target, ".md") && isFile(r.site, strings.TrimSuffix(target, ".md")+".html"):
		return strings.TrimSuffix(p, ".md") + ".html" + suffix
	case strings.HasSuffix(target, ".md"):
		// Not rendered: Jekyll's raw copy (if any) would show unrendered Markdown.
	case exists(r.site, target):
		return href
	case strings.HasSuffix(target, ".html") && isFile(r.src, strings.TrimSuffix(target, ".html")+".md"):
		target = strings.TrimSuffix(target, ".html") + ".md"
	}
	return r.blobBase + repoPath(target) + suffix
}

// repoPath turns a path relative to docs/ into one relative to the repository.
func repoPath(target string) string {
	return strings.TrimPrefix(path.Clean("docs/"+target), "/")
}

func exists(root, rel string) bool {
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return false
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func isFile(root, rel string) bool {
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return false
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}
