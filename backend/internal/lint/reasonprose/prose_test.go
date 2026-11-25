package reasonprose

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	apperrors "mycorrhizal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestParseAllowlist(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		allow, err := parseAllowlist("# count: 2\n*/status\thttp status\nfoo_bar/reason\tparser detail\n")
		require.NoError(t, err)
		assert.Len(t, allow.reasons, 2)
		assert.Equal(t, 2, allow.count)
		assert.Equal(t, "parser detail", allow.reasons["foo_bar/reason"])
	})

	t.Run("comments and blank lines are ignored", func(t *testing.T) {
		allow, err := parseAllowlist("# a comment\n\n# count: 1\n\n*/status\tdata, not a sentence\n")
		require.NoError(t, err)
		assert.Equal(t, 1, allow.count)
		assert.Contains(t, allow.reasons, "*/status")
	})

	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"missing count", "*/status\thttp status\n", "missing # count"},
		{"count mismatch", "# count: 2\n*/status\thttp status\n", "count header says 2"},
		{"bad count header", "# count: many\n*/status\thttp status\n", "bad count header"},
		{"blank reason", "# count: 1\n*/status\t\n", "has no reason"},
		{"missing tab", "# count: 1\n*/status http status\n", "want code/key"},
		{"bad key", "# count: 1\nstatus\thttp status\n", "must be code/key"},
		{"empty code", "# count: 1\n/status\thttp status\n", "must be code/key"},
		{"empty param", "# count: 1\nfoo/\tparser detail\n", "must be code/key"},
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
			name:   "three words is prose",
			code:   "some_code",
			params: map[string]any{"detail": "Instance not reachable"},
			want:   []string{"detail"},
		},
		{
			name:   "two words is a value",
			code:   "some_code",
			params: map[string]any{"detail": "relationship edges"},
		},
		{
			name:   "whitespace-only is not prose",
			code:   "some_code",
			params: map[string]any{"detail": "   \t\n "},
		},
		{
			name:   "multi-line and extra spacing still count words",
			code:   "some_code",
			params: map[string]any{"detail": "one\n  two   three\tfour"},
			want:   []string{"detail"},
		},
		{
			name:   "non-string values are never prose",
			code:   "some_code",
			params: map[string]any{"count": 4, "ok": true},
		},
		{
			name:   "nil params are ignored",
			code:   "some_code",
			params: nil,
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
			name:   "allowlisted code with a different key is prose",
			code:   "meerkat_api_error",
			params: map[string]any{"detail": "one two three four"},
			want:   []string{"detail"},
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

func TestCheckProblemFields(t *testing.T) {
	problems := Check("some_code", map[string]any{"detail": "one two three four"})
	require.Len(t, problems, 1)
	p := problems[0]
	assert.Equal(t, "some_code", p.Code)
	assert.Equal(t, "detail", p.Key)
	assert.Equal(t, "one two three four", p.Value)
	assert.Contains(t, p.Why, "3 or more words")
	assert.Contains(t, p.Why, "allowlist entry")
}

// TestCheckMalformedAllowlistFailsClosed exercises the load-error branch. The
// embedded allowlist loads once through a sync.Once, so swap the parsed result
// after forcing that load and restore it when the test ends.
func TestCheckMalformedAllowlistFailsClosed(t *testing.T) {
	_, _ = loadAllowlist()
	prevLoaded, prevErr := loaded, loadError
	t.Cleanup(func() { loaded, loadError = prevLoaded, prevErr })
	loaded = allowlist{}
	loadError = errors.New("boom")

	problems := Check("some_code", map[string]any{"detail": "one two three four"})
	require.Len(t, problems, 2)
	assert.Equal(t, "some_code", problems[0].Code)
	assert.Empty(t, problems[0].Key)
	assert.Contains(t, problems[0].Why, "allowlist is malformed: boom")
	assert.Equal(t, "detail", problems[1].Key)
}

func TestSortProblemsOrdersByKeyThenValue(t *testing.T) {
	problems := []Problem{
		{Key: "b", Value: "a"},
		{Key: "a", Value: "z"},
		{Key: "a", Value: "a"},
	}
	sortProblems(problems)
	assert.Equal(t, []Problem{
		{Key: "a", Value: "a"},
		{Key: "a", Value: "z"},
		{Key: "b", Value: "a"},
	}, problems)
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

// --- Static half (issue #2920 follow-up) -------------------------------------

func TestStaticAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), StaticAnalyzer, "a")
}

// TestNoUnallowlistedProseSourcesInProduction is the static gate: every
// production WithReason param that is a definite prose source (long constant,
// err.Error(), Sprintf) must be a reasoned allowlist entry, whether or not any
// test reaches that path. The runtime guard (RunGuarded) cannot see untested
// paths; #2920's review found 23 err.Error() passthroughs it never reached.
func TestNoUnallowlistedProseSourcesInProduction(t *testing.T) {
	findings, err := ScanPackages("../../..", "./...")
	require.NoError(t, err)
	require.NotEmpty(t, findings, "the scan found no WithReason prose sources at all; the analyzer is probably not matching the real errors package")
	for _, f := range findings {
		assert.True(t, Allowed(f.Key), "%s: %s -- use a localizable reason code/template, or add a reasoned entry to prose_allowlist.txt", f.Position, f.Message)
	}
}

// TestProseReasonEntriesAreDeclaredPassthroughs ties this allowlist to the web
// passthrough ratchet: an allowlisted prose `reason` param is only acceptable
// while the client deliberately renders it through a {{reason}}-only template
// (compatci's reason_passthrough_allowlist.txt). Localizing the template
// without dropping the prose param, or the reverse, fails here.
func TestProseReasonEntriesAreDeclaredPassthroughs(t *testing.T) {
	raw, err := os.ReadFile("../../compatci/reason_passthrough_allowlist.txt")
	require.NoError(t, err)
	passthrough := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _ := strings.Cut(line, "\t")
		passthrough[strings.TrimSpace(key)] = true
	}
	allow, err := allowlistForTest()
	require.NoError(t, err)
	for key := range allow.reasons {
		code, param, _ := strings.Cut(key, "/")
		if param != "reason" {
			continue
		}
		assert.True(t, passthrough[code], "%s is allowlisted as prose but errors.reason.%s is not a declared {{reason}} passthrough", key, code)
	}
}

type fakeRunner struct{ run func() int }

func (f fakeRunner) Run() int { return f.run() }

// TestRunGuarded proves the whole-package guard sees a reason attached
// anywhere during m.Run, fails a green run, and keeps a red run red.
func TestRunGuarded(t *testing.T) {
	prose := func() int {
		apperrors.ErrInvalidInput("x", "y").WithReason(apperrors.ReasonMeerkatUpstreamError, map[string]any{"reason": "The Meerkat instance could not be reached at all"})
		return 0
	}
	var out strings.Builder
	assert.Equal(t, 1, runGuarded(fakeRunner{prose}, &out))
	assert.Contains(t, out.String(), `reason_params["reason"] for meerkat_upstream_error is free prose`)

	out.Reset()
	assert.Equal(t, 3, runGuarded(fakeRunner{func() int { prose(); return 3 }}, &out), "a failing run keeps its own exit code")

	out.Reset()
	clean := func() int {
		apperrors.ErrInvalidInput("x", "y").WithReason(apperrors.ReasonMeerkatRemoteTooManyContacts, map[string]any{"total": 9, "max": 5})
		return 0
	}
	assert.Equal(t, 0, runGuarded(fakeRunner{clean}, &out))
	assert.Empty(t, out.String())

	// Per-field validation reasons are observed too.
	out.Reset()
	fieldProse := func() int {
		apperrors.ErrValidationFields([]apperrors.ValidationError{{
			Field: "name", Message: "m", ReasonCode: apperrors.ReasonMeerkatUpstreamError,
			ReasonParams: map[string]any{"reason": "this field value could not be parsed"},
		}})
		return 0
	}
	assert.Equal(t, 1, runGuarded(fakeRunner{fieldProse}, &out))

	// The observer is removed afterwards.
	out.Reset()
	prose()
	assert.Empty(t, out.String())
}

// TestAllowedMatchesExactAndWildcardEntriesAndFailsClosed pins Allowed's three
// outcomes against a swapped-in allowlist (see TestCheckMalformedAllowlistFailsClosed).
func TestAllowedMatchesExactAndWildcardEntriesAndFailsClosed(t *testing.T) {
	_, _ = loadAllowlist()
	prevLoaded, prevErr := loaded, loadError
	t.Cleanup(func() { loaded, loadError = prevLoaded, prevErr })

	allow, err := parseAllowlist("# count: 2\nfoo_bar/reason\tparser detail\n*/status\thttp status\n")
	require.NoError(t, err)
	loaded, loadError = allow, nil
	assert.True(t, Allowed("foo_bar/reason"), "exact entry")
	assert.True(t, Allowed("any_code/status"), "*/param entry")
	assert.False(t, Allowed("foo_bar/detail"), "same code, other param")
	assert.False(t, Allowed("other_code/reason"), "same param, other code")

	loadError = errors.New("boom")
	assert.False(t, Allowed("foo_bar/reason"), "a malformed allowlist allows nothing")
}

func TestScanPackagesReportsLoadErrors(t *testing.T) {
	_, err := ScanPackages(t.TempDir(), "./does-not-exist/...")
	require.Error(t, err)

	_, err = ScanPackages(filepath.Join(t.TempDir(), "missing-dir"), "./...")
	require.Error(t, err)
}
