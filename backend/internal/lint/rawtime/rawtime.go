// Package rawtime is a go/analysis pass for issue #1494: time-dependent
// business logic must read "now" from an injected clock.Clock
// (backend/internal/clock), not from the wall clock, so a test can pin or
// advance time instead of sleeping and day-boundary / DST / expiry-edge
// behaviour can be asserted at exact instants.
//
// It flags every use of time.Now, time.Since and time.Until (Since/Until call
// Now internally, so they are the same bypass) in non-test files of the
// controllers, services and middleware packages, whether called
// (`time.Now()`) or passed as a value (`newCache(time.Now)`).
//
// A use is permitted only when it is
//
//   - on a line carrying (or directly under a line carrying) a
//     `// rawtime:allow <reason>` comment — a non-empty reason is required, the
//     same discipline as `# pragma: no cover`; or
//   - in a file listed in Allowlist, up to that entry's Count, with a
//     recorded Reason. The list is a ratchet, not a free pass: the count is
//     exact (TestAllowlistIsExact fails when a site is migrated and the count
//     is not lowered, as well as when one is added), so the list can only
//     shrink as areas are migrated to the injected clock.
//
// Test files are skipped; the clock package itself is outside the scoped
// packages.
package rawtime

import (
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// scopedPackages are the package directories the rule applies to.
var scopedPackages = map[string]bool{
	"controllers": true,
	"services":    true,
	"middleware":  true,
}

// rawFuncs are the time-package functions that read the wall clock.
var rawFuncs = map[string]bool{"Now": true, "Since": true, "Until": true}

const allowMarker = "rawtime:allow"

// Analyzer reports raw wall-clock reads in controllers, services and
// middleware that are not on the allowlist.
var Analyzer = &analysis.Analyzer{
	Name: "rawtime",
	Doc:  "report time.Now/Since/Until in controllers, services and middleware outside the reason-bearing allowlist: inject a clock.Clock instead (issue #1494)",
	Run:  run,
}

// Use is one raw wall-clock read found by Scan.
type Use struct {
	Pos  token.Pos
	Name string // "Now", "Since" or "Until"
}

// Scan returns the raw time uses in file that are not covered by an inline
// `rawtime:allow` marker, plus the positions of markers that lack a reason.
// It is purely syntactic (the `time` import is resolved by its local name),
// so the analyzer and the allowlist-exactness test share one definition.
func Scan(fset *token.FileSet, file *ast.File) (uses []Use, badMarkers []token.Pos) {
	timeName := ""
	for _, imp := range file.Imports {
		if imp.Path.Value != `"time"` {
			continue
		}
		timeName = "time"
		if imp.Name != nil {
			timeName = imp.Name.Name
		}
	}
	if timeName == "" || timeName == "_" || timeName == "." {
		return nil, nil
	}

	// line -> reason presence, for lines carrying a marker.
	markerLines := map[int]bool{}
	for _, cg := range file.Comments {
		for _, cm := range cg.List {
			idx := strings.Index(cm.Text, allowMarker)
			if idx < 0 {
				continue
			}
			reason := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cm.Text[idx+len(allowMarker):]), "*/"))
			line := fset.Position(cm.Pos()).Line
			if reason == "" {
				badMarkers = append(badMarkers, cm.Pos())
				continue
			}
			markerLines[line] = true
			// A marker on a comment-only line covers the next line.
			markerLines[fset.Position(cm.End()).Line+1] = true
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != timeName || id.Obj != nil || !rawFuncs[sel.Sel.Name] {
			return true
		}
		if markerLines[fset.Position(sel.Pos()).Line] {
			return true
		}
		uses = append(uses, Use{Pos: sel.Pos(), Name: sel.Sel.Name})
		return true
	})
	sort.Slice(uses, func(i, j int) bool { return uses[i].Pos < uses[j].Pos })
	return uses, badMarkers
}

// Key is the Allowlist key of filename: "<package dir>/<file>".
func Key(filename string) string {
	return path.Base(filepath.ToSlash(filepath.Dir(filename))) + "/" + filepath.Base(filename)
}

func run(pass *analysis.Pass) (any, error) {
	if !scopedPackages[path.Base(pass.Pkg.Path())] {
		return nil, nil
	}
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		uses, bad := Scan(pass.Fset, file)
		for _, p := range bad {
			pass.Reportf(p, "rawtime:allow needs a reason after the marker (issue #1494)")
		}
		allowed := Allowlist[Key(filename)].Count
		for i, u := range uses {
			if i < allowed {
				continue
			}
			pass.Reportf(u.Pos, "raw time.%s() in %s: read the time from an injected clock.Clock (clock.FromContext(c) / services.Now()), or justify it with a `// rawtime:allow <reason>` comment (issue #1494)", u.Name, path.Base(pass.Pkg.Path()))
		}
	}
	return nil, nil
}
