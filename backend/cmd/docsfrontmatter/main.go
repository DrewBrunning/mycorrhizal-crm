// Command docsfrontmatter lets a released version of the docs site render the
// pages it shipped without YAML front matter (issue #2244).
//
// scripts/build-docs-versions.sh builds each release from its own tag. Up to
// v1.7.0, about a dozen operator and import pages (disaster recovery, the
// configuration reference, the migration-recovery runbook, ...) had no front
// matter, so Jekyll copied them as raw Markdown and every link to their .html
// address was a 404. Their text was right; only the front matter was missing,
// and main has it. For each page in the release tree (-src) that has none
// while the same page in the current tree (-current) has some, this prepends
// main's front matter to the release's own text, adapted to the release's nav:
//
//   - parent is kept when a page with that title exists in the release tree
//     (counting the pages being backfilled), and dropped otherwise, as is a
//     grand_parent the release does not have;
//   - a page whose parent was dropped joins the release's top-level nav after
//     its own entries, ordered by main's (parent, nav_order, path), so it does
//     not collide with a release page's nav_order.
//
// Pages that already have front matter, pages main does not publish, and pages
// main no longer has are left alone. The page text is never changed: fixing a
// release's content still needs a patch release. A second run is a no-op.
//
// Usage (from backend/): go run ./cmd/docsfrontmatter -src DIR -current DIR
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit ends the process; tests drive run()
}

// frontMatterRe matches a leading YAML front matter block, its closing
// delimiter line, and the blank lines that separate it from the body.
var frontMatterRe = regexp.MustCompile(`(?m)\A---\r?\n((?s:.*?))^---[ \t]*\r?\n((?:[ \t]*\r?\n)*)`)

type page struct {
	rel   string         // slash path relative to the docs root
	lines []string       // main's front matter lines
	sep   string         // main's separator between front matter and body
	meta  map[string]any // main's parsed front matter
	body  []byte         // the release's own text
}

func run(args []string, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("docsfrontmatter", flag.ContinueOnError)
	fset.SetOutput(stderr)
	src := fset.String("src", "", "docs/ tree of the release being built (rewritten in place)")
	current := fset.String("current", "", "docs/ tree of the current main")
	if err := fset.Parse(args); err != nil {
		return 2
	}
	if *src == "" || *current == "" {
		fmt.Fprintln(stderr, "docsfrontmatter: -src and -current are required")
		return 2
	}
	n, err := backfill(*src, *current)
	if err != nil {
		fmt.Fprintln(stderr, "docsfrontmatter:", err)
		return 2
	}
	fmt.Fprintf(stdout, "docsfrontmatter: backfilled front matter on %d page(s) in %s\n", n, *src)
	return 0
}

func backfill(src, current string) (int, error) {
	root, err := os.OpenRoot(src)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()

	titles := map[string]bool{} // titles of pages the release will render
	maxTop := 0                 // the release's highest top-level nav_order
	var todo []page
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(rel, ".md") {
			return err
		}
		body, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		if m := frontMatterRe.FindSubmatch(body); m != nil {
			var meta map[string]any
			if yaml.Unmarshal(m[1], &meta) == nil { // a release page we cannot parse simply has no title to offer
				titles[str(meta["title"])] = true
				if meta["parent"] == nil {
					maxTop = max(maxTop, num(meta["nav_order"]))
				}
			}
			return nil
		}
		cur, err := os.ReadFile(filepath.Join(current, filepath.FromSlash(rel))) // #nosec G304 -- build-time CLI; rel comes from walking -src
		if errors.Is(err, fs.ErrNotExist) {
			return nil // main no longer has this page
		}
		if err != nil {
			return err
		}
		m := frontMatterRe.FindSubmatch(cur)
		if m == nil {
			return nil
		}
		var meta map[string]any
		if err := yaml.Unmarshal(m[1], &meta); err != nil {
			return fmt.Errorf("%s: front matter on main does not parse: %w", rel, err)
		}
		titles[str(meta["title"])] = true
		todo = append(todo, page{rel: rel, lines: strings.SplitAfter(string(m[1]), "\n"), sep: string(m[2]), meta: meta, body: body})
		return nil
	})
	if err != nil {
		return 0, err
	}

	var orphans []*page
	drop := map[*page]map[string]bool{}
	for i := range todo {
		p := &todo[i]
		drop[p] = map[string]bool{}
		if !titles[str(p.meta["parent"])] {
			drop[p]["parent"], drop[p]["grand_parent"] = true, true
			orphans = append(orphans, p)
		} else if gp := p.meta["grand_parent"]; gp != nil && !titles[str(gp)] {
			drop[p]["grand_parent"] = true
		}
	}
	sort.SliceStable(orphans, func(i, j int) bool {
		a, b := orphans[i], orphans[j]
		if pa, pb := str(a.meta["parent"]), str(b.meta["parent"]); pa != pb {
			return pa < pb
		}
		if na, nb := num(a.meta["nav_order"]), num(b.meta["nav_order"]); na != nb {
			return na < nb
		}
		return a.rel < b.rel
	})
	order := map[*page]int{}
	for i, p := range orphans {
		order[p] = maxTop + 1 + i
	}

	for i := range todo {
		p := &todo[i]
		fm := frontMatter(p.lines, drop[p], order[p])
		if err := root.WriteFile(p.rel, []byte("---\n"+fm+"---\n"+p.sep+string(p.body)), 0o644); err != nil { // #nosec G306 -- a public docs page in the build tree
			return 0, err
		}
	}
	return len(todo), nil
}

// frontMatter rebuilds main's front matter lines without the dropped keys and,
// when navOrder is set, with that nav_order.
func frontMatter(lines []string, drop map[string]bool, navOrder int) string {
	var b strings.Builder
	for _, line := range lines {
		key, _, _ := strings.Cut(line, ":")
		switch {
		case line == "":
		case drop[key]:
		case key == "nav_order" && navOrder > 0:
			fmt.Fprintf(&b, "nav_order: %d\n", navOrder)
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func num(v any) int {
	n, _ := v.(int)
	return n
}
