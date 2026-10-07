package models

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoProcessGlobalAuditRecorder is the #1493 guard against the singleton
// coming back. It scans every non-test Go file in the backend module and
// fails on:
//
//   - a reference to the removed global API (RegisterAuditDB, AuditFlush, the
//     package var auditRecorder), and
//   - a package-level variable (outside the recorder's own file) typed or
//     initialised as a recorder (AuditRecorder / auditLogger).
//
// The recorder must be reached through a *gorm.DB (AuditRecorderFor) or owned
// by whoever built it (NewAuditRecorder's return value). It runs in
// "Backend (Go)" via go test, so it is merge-blocking without a new CI job.
func TestNoProcessGlobalAuditRecorder(t *testing.T) {
	root := ".." // backend/
	fset := token.NewFileSet()
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "testdata" || n == "node_modules" || strings.HasPrefix(n, ".") && n != "." && n != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		violations = append(violations, auditGlobalViolations(fset, f, filepath.Base(path) == "audit_recorder.go" && filepath.Base(filepath.Dir(path)) == "models")...)
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, violations, "the audit recorder must not be a process global (issue #1493)")
}

// auditGlobalViolations reports the guard's findings in one parsed file.
// ownFile exempts the recorder implementation file's package-level mutex.
func auditGlobalViolations(fset *token.FileSet, f *ast.File, ownFile bool) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			switch id.Name {
			case "RegisterAuditDB", "AuditFlush", "auditRecorder":
				out = append(out, fset.Position(id.Pos()).String()+": "+id.Name)
			}
		}
		return true
	})
	if ownFile {
		return out
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if mentionsRecorder(vs.Type) {
				out = append(out, fset.Position(vs.Pos()).String()+": package-level recorder var")
				continue
			}
			for _, v := range vs.Values {
				if mentionsRecorder(v) {
					out = append(out, fset.Position(vs.Pos()).String()+": package-level recorder var")
				}
			}
		}
	}
	return out
}

func mentionsRecorder(n ast.Node) bool {
	if n == nil {
		return false
	}
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		if id, ok := x.(*ast.Ident); ok && (id.Name == "AuditRecorder" || id.Name == "auditLogger") {
			found = true
		}
		return !found
	})
	return found
}

// TestAuditGlobalGuard_DetectsViolations proves the guard can fail: it is fed
// source that reintroduces each forbidden shape.
func TestAuditGlobalGuard_DetectsViolations(t *testing.T) {
	cases := map[string]string{
		"register":  "package p\nfunc f() { RegisterAuditDB(nil) }\n",
		"flush":     "package p\nfunc f() { AuditFlush() }\n",
		"oldGlobal": "package p\nfunc f() { auditRecorder.record() }\n",
		"typedVar":  "package p\nvar r AuditRecorder\n",
		"initVar":   "package p\nvar r = &auditLogger{}\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "x.go", src, 0)
			require.NoError(t, err)
			assert.NotEmpty(t, auditGlobalViolations(fset, f, false))
		})
	}
	t.Run("clean", func(t *testing.T) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", "package p\nvar n = 1\nconst c = 2\nfunc f() { _ = n }\n", 0)
		require.NoError(t, err)
		assert.Empty(t, auditGlobalViolations(fset, f, false))
	})
	t.Run("ownFileExemptFromVarCheck", func(t *testing.T) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", "package p\nvar r AuditRecorder\n", 0)
		require.NoError(t, err)
		assert.Empty(t, auditGlobalViolations(fset, f, true))
	})
}
