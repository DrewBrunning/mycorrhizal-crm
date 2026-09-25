// Package omitemptyguard is the structural gate for CLAUDE.md's frontend
// trap #8: a Go struct field that is a slice or map, tagged `omitempty`, on
// a type that is actually serialized as an HTTP response, is a crash
// waiting to happen. `db.Find` (or an unpopulated Preload) leaves a slice
// nil when nothing matches; `omitempty` then drops the key entirely; and a
// frontend TypeScript type with the field declared required (not `field?:
// T[]`) reads that as `undefined` and breaks on `.length`. This shipped on
// the contact prep view for any contact with no history before it was
// fixed.
//
// Not every slice/map `omitempty` is this bug — a request-only DTO, a field
// deliberately absent unless requested (`includes=`), a single record's own
// optional JSON-blob column (not a has-many association GORM's Find/Preload
// can leave nil), and RFC-shaped Card JSON (contactmodel's own types, which
// are out of this WP's scope) are all legitimate. AllowlistReasons records
// those with a reason instead of silently exempting them, so removing a
// field's legitimate exemption (e.g. turning a request-only DTO into a
// response type) is a reviewable diff, not a silent gap.
package omitemptyguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Finding is one struct field that needs a reason (not yet allowlisted) or
// is allowlisted with no reason recorded.
type Finding struct {
	Path      string
	Line      int
	StructKey string // "StructName.FieldName", the allowlist key
	JSONTag   string
}

// jsonOmitempty extracts the field name from a `json:"name,omitempty"` tag
// and reports whether `omitempty` is one of its options.
func jsonOmitempty(tag string) (name string, has bool) {
	v := extractTagValue(tag, "json")
	if v == "" {
		return "", false
	}
	parts := strings.Split(v, ",")
	for _, p := range parts[1:] {
		if p == "omitempty" {
			return parts[0], true
		}
	}
	return parts[0], false
}

// extractTagValue pulls the value of a `key:"value"` entry out of a raw Go
// struct tag string (the string still carries its surrounding backticks).
func extractTagValue(rawTag, key string) string {
	tag := strings.Trim(rawTag, "`")
	prefix := key + `:"`
	idx := strings.Index(tag, prefix)
	if idx == -1 {
		return ""
	}
	rest := tag[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// isSliceOrMap reports whether expr is a slice or map type expression
// (`[]T` or `map[K]V`), including through a named type is NOT resolved here
// (this is a syntactic AST scan, not a type-checked one) — a field typed as
// a named slice/map alias is out of this scan's reach, same tradeoff every
// lightweight structural gate in this repo makes (e.g. cmd/pragmacheck's
// regex-based scan versus a full parse).
func isSliceOrMap(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.ArrayType:
		return t.Len == nil // nil Len => slice, not a fixed-size array
	case *ast.MapType:
		return true
	default:
		return false
	}
}

// ScanFile parses a single Go source file's content for exported struct
// fields that are slices/maps tagged `omitempty`, and returns every one not
// present (with a non-empty reason) in allowlist. allowlist keys are
// "StructName.FieldName".
func ScanFile(path, content string, allowlist map[string]string) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil || !isSliceOrMap(field.Type) {
				continue
			}
			name, hasOmitempty := jsonOmitempty(field.Tag.Value)
			if name == "-" || !hasOmitempty {
				continue
			}
			for _, fname := range field.Names {
				key := ts.Name.Name + "." + fname.Name
				if reason, ok := allowlist[key]; ok && strings.TrimSpace(reason) != "" {
					continue
				}
				pos := fset.Position(field.Pos())
				findings = append(findings, Finding{
					Path:      path,
					Line:      pos.Line,
					StructKey: key,
					JSONTag:   name,
				})
			}
		}
		return true
	})
	return findings, nil
}

// ScanDir scans every non-test .go file directly under dir (not recursive —
// models and controllers are each a single flat package directory in this
// repo) and returns every finding, sorted by path then line.
func ScanDir(dir string, allowlist map[string]string) ([]Finding, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		// #nosec G304 -- path comes from os.ReadDir over a caller-supplied directory, not request input
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, err := ScanFile(path, string(b), allowlist)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// UnusedAllowlistEntries returns every allowlist key that ScanDir's findings
// never actually matched against — a stale entry for a field that was since
// fixed, renamed, or removed. Pass the allowlist and the struct keys
// actually observed while scanning (independent of whether they were
// allowlisted) so a drift test can fail on dead entries the same way
// docs/security/citation-drift.ignore's checker does.
func UnusedAllowlistEntries(allowlist map[string]string, observed map[string]bool) []string {
	var unused []string
	for key := range allowlist {
		if !observed[key] {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	return unused
}

// ScanDirObserved is ScanDir's sibling: it returns every omitempty
// slice/map struct field key it saw, allowlisted or not, so a drift test
// can find stale allowlist entries via UnusedAllowlistEntries.
func ScanDirObserved(dir string) (map[string]bool, error) {
	observed, err := ScanDir(dir, nil)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(observed))
	for _, f := range observed {
		out[f.StructKey] = true
	}
	return out, nil
}

// FormatFinding renders one finding as a one-line report entry.
func FormatFinding(f Finding) string {
	return fmt.Sprintf("%s:%d: %s (json %s) is a slice/map field with `omitempty` on a struct that may be an HTTP response — allowlist it in internal/omitemptyguard with a reason, or drop omitempty and initialize it to a non-nil empty value (CLAUDE.md frontend trap #8)", f.Path, f.Line, f.StructKey, strconv.Quote(f.JSONTag))
}
