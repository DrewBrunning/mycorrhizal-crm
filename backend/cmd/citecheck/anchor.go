package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// Symbol anchors: the `path#Anchor` citation form.
//
// A `path:line` citation goes stale on any edit above the cited line, so every
// release branch and main drift apart and a docs-only merge conflicts for no
// reason anyone can resolve by reading (the v1.3.0-rc.4 promotion). An anchor
// names the *thing* — a Go declaration, a CI step or job, a Kotlin/TS
// declaration, or a quoted literal — so editing elsewhere in the file leaves
// the citation valid, while renaming or removing the thing fails the gate.
//
// Anchor kinds, chosen by the cited file's extension:
//
//	.go                  Func, Type, Var/Const, Type.Method, Type.Field
//	.yml / .yaml         a step `name:` value, or a mapping key (job id, etc.)
//	.kt .kts .ts .tsx    a declaration (`fun`, `class`, `val`, `function`, …)
//	any file, quoted     `path#"literal text"` — substring present in the file
//
// A file type with no declaration syntax (conf, md, sql, xml, json, sh, …)
// resolves an unquoted anchor as a literal substring too.

// resolveAnchor reports whether anchor names something in the file rel. A
// non-nil error means the file could not be read or parsed, not "not found".
func resolveAnchor(idx *treeIndex, rel, anchor string) (bool, error) {
	body, err := readRepoFile(idx.root, rel)
	if err != nil {
		return false, err
	}
	if lit, ok := quotedLiteral(anchor); ok {
		return strings.Contains(string(body), lit), nil
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		return goAnchorExists(rel, body, anchor)
	case ".yml", ".yaml":
		return yamlAnchorExists(body, anchor), nil
	case ".kt", ".kts", ".ts", ".tsx":
		return declAnchorExists(body, anchor), nil
	}
	return strings.Contains(string(body), anchor), nil
}

// quotedLiteral unwraps the `"literal text"` anchor form.
func quotedLiteral(anchor string) (string, bool) {
	if len(anchor) >= 3 && strings.HasPrefix(anchor, `"`) && strings.HasSuffix(anchor, `"`) {
		return anchor[1 : len(anchor)-1], true
	}
	return "", false
}

// goAnchorExists parses the file (declarations only) and looks for anchor
// among its top-level names.
func goAnchorExists(rel string, body []byte, anchor string) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), rel, body, parser.SkipObjectResolution)
	if err != nil {
		return false, fmt.Errorf("parsing %s: %w", rel, err)
	}
	return goDeclNames(f)[anchor], nil
}

// goDeclNames is every anchorable name in a Go file: top-level funcs, types,
// vars and consts, methods as `Recv.Method`, and struct fields / interface
// methods as `Type.Member`.
func goDeclNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil && len(d.Recv.List) == 1 {
				names[recvName(d.Recv.List[0].Type)+"."+d.Name.Name] = true
				continue
			}
			names[d.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
					addMembers(names, s)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

// addMembers records `Type.Field` for struct fields and `Type.Method` for
// interface methods.
func addMembers(names map[string]bool, s *ast.TypeSpec) {
	var fields *ast.FieldList
	switch t := s.Type.(type) {
	case *ast.StructType:
		fields = t.Fields
	case *ast.InterfaceType:
		fields = t.Methods
	}
	if fields == nil {
		return
	}
	for _, fld := range fields.List {
		for _, n := range fld.Names {
			names[s.Name.Name+"."+n.Name] = true
		}
	}
}

// recvName is the receiver's type name with the pointer and any type
// parameters stripped: `*Server`, `Server` and `*Cache[K, V]` → Server/Cache.
func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// yamlAnchorExists matches a step/job `name:` value or a mapping key. A
// line scan, not a YAML parse: workflow files carry `${{ }}` expressions and
// anchors that a strict parser handles no better than this does for what is
// only an existence check.
func yamlAnchorExists(body []byte, anchor string) bool {
	q := regexp.QuoteMeta(anchor)
	nameRe := regexp.MustCompile(`(?m)^\s*(?:-\s*)?name:\s*["']?` + q + `["']?\s*(?:#.*)?$`)
	keyRe := regexp.MustCompile(`(?m)^\s*(?:-\s*)?["']?` + q + `["']?\s*:`)
	return nameRe.Match(body) || keyRe.Match(body)
}

// declAnchorExists matches a Kotlin/TypeScript declaration of the anchor's last
// dotted segment (an extension receiver such as `Flow<T?>.name` is skipped;
// `Type.member` requires `Type` to appear as a word too, so
// a member anchor cannot be satisfied by an unrelated file-level name).
func declAnchorExists(body []byte, anchor string) bool {
	parts := strings.Split(anchor, ".")
	name := parts[len(parts)-1]
	if name == "" {
		return false
	}
	decl := regexp.MustCompile(`(?m)\b(?:fun|class|object|interface|val|var|const|function|type|enum|typealias)\s+(?:<[^>]*>\s*)?(?:[^\s(=:]+\.)?` + regexp.QuoteMeta(name) + `\b`)
	if !decl.Match(body) {
		return false
	}
	for _, owner := range parts[:len(parts)-1] {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(owner) + `\b`).Match(body) {
			return false
		}
	}
	return true
}
