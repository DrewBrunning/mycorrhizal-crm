// Package gormerr is a go/analysis pass for CLAUDE.md backend trap #4
// ("check .Error on every db.Updates/db.Save").
//
// GORM's finisher methods (Create, Save, Updates, Delete, First, Find, Exec,
// ...) report failure through the returned *gorm.DB's Error field rather than
// an error result, so errcheck cannot see a dropped error: a bare
//
//	db.Save(&contact)
//
// compiles, passes errcheck, and silently loses a failed write. This pass
// flags a finisher call whose *gorm.DB result is discarded:
//
//   - as an expression statement (`db.Save(&x)`),
//   - assigned only to blank identifiers (`_ = db.Save(&x)`),
//   - in a defer or go statement (`defer db.Delete(&x)`).
//
// Reading the result in any way (`.Error`, `.RowsAffected`, passing it on,
// assigning it to a variable) is not flagged. An explicit `_ = db.X().Error`
// is a visible, reviewable decision to drop the error and is also not
// flagged. Test files are skipped.
package gormerr

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const gormPkgPath = "gorm.io/gorm"

// finishers are the *gorm.DB methods that execute SQL and return *gorm.DB,
// so their error lives only in the returned value's Error field. Chain
// builders (Where, Model, Select, Raw, ...) are deliberately absent: they run
// nothing, and a discarded builder is not an error-handling bug. Rollback /
// RollbackTo are absent too — they run on a path that is already failing and
// whose own error has nothing left to protect. Methods returning a plain
// error (Transaction, ScanRows, Rows, Association.*) are errcheck's job.
var finishers = map[string]bool{
	"Commit":          true,
	"Count":           true,
	"Create":          true,
	"CreateInBatches": true,
	"Delete":          true,
	"Exec":            true,
	"Find":            true,
	"FindInBatches":   true,
	"First":           true,
	"FirstOrCreate":   true,
	"FirstOrInit":     true,
	"Last":            true,
	"Pluck":           true,
	"Save":            true,
	"Scan":            true,
	"Take":            true,
	"Update":          true,
	"UpdateColumn":    true,
	"UpdateColumns":   true,
	"Updates":         true,
}

// Analyzer reports GORM finisher calls whose *gorm.DB result (and so its
// Error) is discarded.
var Analyzer = &analysis.Analyzer{
	Name:     "gormerr",
	Doc:      "report GORM finisher calls (Create, Save, Updates, Delete, First, Find, Exec, ...) whose *gorm.DB result — and so its .Error — is discarded",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.ExprStmt)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.DeferStmt)(nil),
		(*ast.GoStmt)(nil),
	}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		if strings.HasSuffix(pass.Fset.Position(n.Pos()).Filename, "_test.go") {
			return
		}
		switch stmt := n.(type) {
		case *ast.ExprStmt:
			check(pass, stmt.X, "")
		case *ast.DeferStmt:
			check(pass, stmt.Call, "deferred ")
		case *ast.GoStmt:
			check(pass, stmt.Call, "go ")
		case *ast.AssignStmt:
			if len(stmt.Rhs) != 1 {
				return
			}
			for _, lhs := range stmt.Lhs {
				if id, ok := lhs.(*ast.Ident); !ok || id.Name != "_" {
					return
				}
			}
			check(pass, stmt.Rhs[0], "blank-assigned ")
		}
	})
	return nil, nil
}

func check(pass *analysis.Pass, expr ast.Expr, kind string) {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return
	}
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || !finishers[fn.Name()] || !isGormDBMethod(fn) {
		return
	}
	pass.Reportf(call.Pos(), "%sresult of (*gorm.DB).%s is discarded: its .Error is never checked (CLAUDE.md backend trap #4)", kind, fn.Name())
}

// isGormDBMethod reports whether fn is a method of *gorm.DB returning *gorm.DB.
func isGormDBMethod(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	if !isGormDBPtr(sig.Recv().Type()) {
		return false
	}
	return sig.Results().Len() == 1 && isGormDBPtr(sig.Results().At(0).Type())
}

func isGormDBPtr(t types.Type) bool {
	ptr, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "DB" && obj.Pkg() != nil && obj.Pkg().Path() == gormPkgPath
}
