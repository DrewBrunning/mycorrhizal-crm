package reasonprose

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

// StaticAnalyzer is the source-level half of the #2920 item 2 guard. The
// runtime guard (RunGuarded) only sees the reasons a test suite reaches; this
// reports every production `.WithReason(code, map[string]any{...})` whose
// param value is a definite prose source, reached by a test or not:
//
//   - a constant string of proseWordThreshold or more words,
//   - err.Error() (an English sentence from the error chain), or
//   - fmt.Sprintf/fmt.Sprint/fmt.Errorf(...).Error() with a format of
//     proseWordThreshold or more words, or
//   - a string concatenation with such a constant or call in it.
//
// A plain variable, a field, a number or a short constant is data the template
// interpolates and is not reported. Diagnostics are prefixed `code/key: ` so
// they key into the same allowlist as Check.
var StaticAnalyzer = &analysis.Analyzer{
	Name:     "reasonprose",
	Doc:      "report WithReason params whose value is a prose source (long constant, err.Error(), Sprintf)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runStatic,
}

const errorsPkgPath = "mycorrhizal/errors"

func runStatic(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if strings.HasSuffix(pass.Fset.Position(call.Pos()).Filename, "_test.go") {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WithReason" || len(call.Args) != 2 {
			return
		}
		if fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func); !ok || fn.Pkg() == nil || fn.Pkg().Path() != errorsPkgPath {
			return
		}
		code := constString(pass, call.Args[0])
		if code == "" {
			return
		}
		lit, ok := ast.Unparen(call.Args[1]).(*ast.CompositeLit)
		if !ok {
			return
		}
		for _, elt := range lit.Elts {
			// The params argument is a map[string]any, so every element of a
			// literal for it is a key: value pair.
			kv := elt.(*ast.KeyValueExpr)
			key := constString(pass, kv.Key)
			if key == "" {
				continue
			}
			if why := proseSource(pass, kv.Value); why != "" {
				pass.Reportf(kv.Value.Pos(), "%s/%s: %s", code, key, why)
			}
		}
	})
	return nil, nil
}

func constString(pass *analysis.Pass, e ast.Expr) string {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(tv.Value)
}

func wordCount(s string) int { return len(strings.Fields(s)) }

// proseSource explains why e yields prose, or returns "".
func proseSource(pass *analysis.Pass, e ast.Expr) string {
	e = ast.Unparen(e)
	if s := constString(pass, e); s != "" {
		if wordCount(s) >= proseWordThreshold {
			return fmt.Sprintf("constant %q is prose", s)
		}
		return ""
	}
	// name + " could not be reached": prose with a value spliced in.
	if bin, ok := e.(*ast.BinaryExpr); ok && bin.Op == token.ADD {
		for _, operand := range []ast.Expr{bin.X, bin.Y} {
			if s := constString(pass, ast.Unparen(operand)); wordCount(s) >= proseWordThreshold {
				return fmt.Sprintf("concatenation with %q builds prose", s)
			}
			if why := proseSource(pass, operand); why != "" {
				return why
			}
		}
		return ""
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		// x.Error() where x is an error: the English text of the error chain.
		if sel.Sel.Name == "Error" && len(call.Args) == 0 {
			if t := pass.TypesInfo.TypeOf(sel.X); t != nil && types.Implements(t, errorInterface) {
				return "err.Error() passes the error chain's English text through"
			}
		}
		if fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == "fmt" {
			switch fn.Name() {
			case "Sprintf", "Sprint", "Sprintln":
				if len(call.Args) > 0 {
					if format := constString(pass, call.Args[0]); wordCount(format) >= proseWordThreshold {
						return fmt.Sprintf("fmt.%s(%s, ...) builds prose", fn.Name(), strconv.Quote(format))
					}
				}
			}
		}
	}
	return ""
}

var errorInterface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// StaticFinding is one StaticAnalyzer diagnostic.
type StaticFinding struct {
	Key      string // "code/param"
	Position string // file:line
	Message  string
}

// ScanPackages runs StaticAnalyzer over the package patterns (relative to dir)
// and returns its findings sorted by key, then position.
func ScanPackages(dir string, patterns ...string) ([]StaticFinding, error) {
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir, Tests: false}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		return nil, fmt.Errorf("%d package load error(s)", n)
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{StaticAnalyzer}, pkgs, nil)
	if err != nil {
		return nil, err
	}
	var out []StaticFinding
	for act := range graph.All() {
		if act.Analyzer != StaticAnalyzer {
			continue
		}
		for _, d := range act.Diagnostics {
			key, _, _ := strings.Cut(d.Message, ": ")
			pos := act.Package.Fset.Position(d.Pos)
			out = append(out, StaticFinding{Key: key, Position: fmt.Sprintf("%s:%d", pos.Filename, pos.Line), Message: d.Message})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Position < out[j].Position
	})
	return out, nil
}

// Allowed reports whether key ("code/param") is covered by the embedded
// allowlist, either exactly or through a "*/param" entry.
// A malformed allowlist allows nothing (fail closed).
func Allowed(key string) bool {
	allow, err := loadAllowlist()
	return err == nil && allow.allows(key)
}
