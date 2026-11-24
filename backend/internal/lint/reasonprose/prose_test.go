package reasonprose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	apperrors "mycorrhizal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAllowlist(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		allow, err := parseAllowlist("# count: 2\n*/status\thttp status\nfoo_bar/reason\tparser detail\n")
		require.NoError(t, err)
		assert.Len(t, allow.reasons, 2)
		assert.Equal(t, 2, allow.count)
		assert.Equal(t, "parser detail", allow.reasons["foo_bar/reason"])
	})

	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"missing count", "*/status\thttp status\n", "missing # count"},
		{"count mismatch", "# count: 2\n*/status\thttp status\n", "count header says 2"},
		{"blank reason", "# count: 1\n*/status\t\n", "has no reason"},
		{"missing tab", "# count: 1\n*/status http status\n", "want code/key"},
		{"bad key", "# count: 1\nstatus\thttp status\n", "must be code/key"},
		{"duplicate", "# count: 2\n*/status\ta\n*/status\tb\n", "duplicate entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAllowlist(tc.text)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   string
		params map[string]any
		want   []string
	}{
		{
			name:   "four words is prose",
			code:   "some_code",
			params: map[string]any{"detail": "one two three four"},
			want:   []string{"detail"},
		},
		{
			name:   "three words is a value",
			code:   "some_code",
			params: map[string]any{"detail": "one two three"},
		},
		{
			name:   "non-string values are never prose",
			code:   "some_code",
			params: map[string]any{"count": 4, "ok": true},
		},
		{
			name:   "allowed key is not prose",
			code:   "immich_upstream_error",
			params: map[string]any{"status": "500 Internal Server Error"},
		},
		{
			name:   "allowed code and key is not prose",
			code:   "meerkat_api_error",
			params: map[string]any{"reason": "The Meerkat URL is invalid"},
		},
		{
			name:   "unallowed reason is prose",
			code:   "some_code",
			params: map[string]any{"reason": "one two three four"},
			want:   []string{"reason"},
		},
		{
			name:   "problems are sorted by key",
			code:   "some_code",
			params: map[string]any{"z": "one two three four", "a": "one two three four"},
			want:   []string{"a", "z"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := Check(tc.code, tc.params)
			got := make([]string, 0, len(problems))
			for _, p := range problems {
				got = append(got, p.Key)
			}
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestEmbeddedAllowlistIsWellFormed(t *testing.T) {
	allow, err := allowlistForTest()
	require.NoError(t, err)
	require.Equal(t, allow.count, len(allow.reasons))
	for key, reason := range allow.reasons {
		assert.NotEmpty(t, reason, "%s needs a one-line reason", key)
		code, param, _ := strings.Cut(key, "/")
		if code == "*" {
			assert.True(t, paramDeclaredAnywhere(param), "%s is not a declared param on any reason code", key)
			continue
		}
		_, params, ok := apperrors.ReasonSpecFor(apperrors.ReasonCode(code))
		require.True(t, ok, "%s names an unregistered reason code", key)
		assert.Contains(t, params, param, "%s is not a declared param of %s", key, code)
	}
}

func paramDeclaredAnywhere(param string) bool {
	for _, code := range apperrors.ReasonCodes() {
		_, params, _ := apperrors.ReasonSpecFor(code)
		for _, p := range params {
			if p == param {
				return true
			}
		}
	}
	return false
}

// TestAllowlistEntriesAreUsedByProductionCallSites is the prose allowlist's
// stale-entry check: an entry whose WithReason call site is gone must be
// removed. This is how #2927 shrinking the meerkat prose site forces this
// allowlist to shrink in the same rebase.
func TestAllowlistEntriesAreUsedByProductionCallSites(t *testing.T) {
	allow, err := allowlistForTest()
	require.NoError(t, err)
	used := productionReasonParamKeys(t, "../../..")
	for key := range allow.reasons {
		assert.True(t, used[key], "allowlist entry %q no longer appears in a WithReason call site; remove the stale entry", key)
	}
}

// productionReasonParamKeys parses the backend module's non-test Go files and
// returns the "code/key" and "*/key" pairs passed to .WithReason with a map
// literal. Calls whose code or params are variables are deliberately skipped:
// the scan is an allowlist-rot check, not a complete census.
func productionReasonParamKeys(t *testing.T, root string) map[string]bool {
	t.Helper()
	constants := reasonConstants(t, filepath.Join(root, "errors", "reasons.go"))
	used := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WithReason" || len(call.Args) < 2 {
				return true
			}
			code := reasonCodeArg(call.Args[0], constants)
			if code == "" {
				return true
			}
			lit, ok := call.Args[1].(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				keyLit, ok := kv.Key.(*ast.BasicLit)
				if !ok || keyLit.Kind != token.STRING {
					continue
				}
				key, err := strconv.Unquote(keyLit.Value)
				if err != nil {
					continue
				}
				used[code+"/"+key] = true
				used["*/"+key] = true
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	return used
}

func reasonCodeArg(expr ast.Expr, constants map[string]string) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return constants[e.Sel.Name]
	case *ast.Ident:
		return constants[e.Name]
	case *ast.CallExpr:
		if len(e.Args) == 1 {
			if lit, ok := e.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				code, err := strconv.Unquote(lit.Value)
				if err == nil {
					return code
				}
			}
		}
	}
	return ""
}

func reasonConstants(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != "ReasonCode" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if code, err := strconv.Unquote(lit.Value); err == nil {
					out[name.Name] = code
				}
			}
		}
	}
	return out
}
