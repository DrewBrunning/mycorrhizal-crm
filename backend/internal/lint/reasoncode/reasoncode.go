// Package reasoncode is a go/analysis pass for the error-localization ratchet
// (issue #2098, ADR 0038): a user-facing apperrors constructor must be chained
// with .WithReason(...) so clients can translate it from reason_code.
//
// It reports:
//   - a call to one of the constrained mycorrhizal/errors constructors whose
//     result is not part of a call chain containing .WithReason, and
//   - a ReasonCode("...") conversion outside mycorrhizal/errors (codes must be
//     the registered constants).
//
// Constructors that get default codes inside the constructor itself (#2099) or
// are 5xx-internal are deliberately absent. Test files are skipped. The
// counted allowlist (allowlist.go) turns the diagnostics into a ratchet.
package reasoncode

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const errorsPkgPath = "mycorrhizal/errors"

var constrained = map[string]bool{
	"ErrUnauthorized":        true,
	"ErrForbidden":           true,
	"ErrNotFoundMessage":     true,
	"ErrConflict":            true,
	"ErrGone":                true,
	"ErrPreconditionFailed":  true,
	"ErrValidation":          true,
	"ErrInvalidInput":        true,
	"ErrBusinessLogic":       true,
	"ErrOperationFailed":     true,
	"ErrExternal":            true,
	"ErrInsufficientStorage": true,
	"ErrPayloadTooLarge":     true,
}

// Analyzer reports user-facing apperrors constructors without a reason code.
var Analyzer = &analysis.Analyzer{
	Name:     "reasoncode",
	Doc:      "report user-facing apperrors constructors not chained with .WithReason, and ReasonCode(\"...\") conversions outside the errors package",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Path() == errorsPkgPath {
		return nil, nil
	}
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	covered := map[*ast.CallExpr]bool{}
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if strings.HasSuffix(pass.Fset.Position(call.Pos()).Filename, "_test.go") {
			return
		}
		// Preorder visits an outer call before the calls in its receiver
		// chain, so a WithReason marks the whole chain beneath it first.
		if isWithReason(pass, call) {
			markChain(call, covered)
			return
		}
		if isReasonCodeConversion(pass, call) {
			pass.Reportf(call.Pos(), `ReasonCode("...") conversion outside the errors package: use a registered Reason constant (ADR 0038)`)
			return
		}
		if name := constrainedName(pass, call); name != "" && !covered[call] {
			pass.Reportf(call.Pos(), "%s without .WithReason: attach a reason code so clients can localize it (issue #2098, ADR 0038)", name)
		}
	})
	return nil, nil
}

func isWithReason(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WithReason" {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == errorsPkgPath
}

// markChain marks every call in call's receiver chain as covered.
func markChain(call *ast.CallExpr, covered map[*ast.CallExpr]bool) {
	var cur ast.Expr = call
	for {
		c, ok := ast.Unparen(cur).(*ast.CallExpr)
		if !ok {
			return
		}
		covered[c] = true
		sel, ok := ast.Unparen(c.Fun).(*ast.SelectorExpr)
		if !ok {
			return
		}
		cur = sel.X
	}
}

func isReasonCodeConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return false
	}
	named, ok := types.Unalias(tv.Type).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "ReasonCode" && obj.Pkg() != nil && obj.Pkg().Path() == errorsPkgPath
}

func constrainedName(pass *analysis.Pass, call *ast.CallExpr) string {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return ""
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != errorsPkgPath || !constrained[fn.Name()] {
		return ""
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return ""
	}
	return fn.Name()
}
