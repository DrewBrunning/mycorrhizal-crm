package errors

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	reasonCodeShape = regexp.MustCompile(`^[a-z]+(_[a-z0-9]+)+$`)
	placeholderRe   = regexp.MustCompile(`\{([^{}]*)\}`)
)

func TestReasonRegistryWellFormed(t *testing.T) {
	areas := map[string]bool{}
	for _, a := range reasonAreas {
		areas[a] = true
	}
	require.NotEmpty(t, reasonRegistry)
	for code, spec := range reasonRegistry {
		c := string(code)
		assert.Regexp(t, reasonCodeShape, c, "code shape: %s", c)
		area, _, _ := strings.Cut(c, "_")
		assert.True(t, areas[area], "%s: area %q is not in reasonAreas", c, area)

		declared := map[string]bool{}
		for _, p := range spec.Params {
			assert.False(t, declared[p], "%s: duplicate param %q", c, p)
			declared[p] = true
		}
		used := map[string]bool{}
		for _, m := range placeholderRe.FindAllStringSubmatch(spec.Template, -1) {
			used[m[1]] = true
		}
		assert.Equal(t, declared, used, "%s: template placeholders must equal Params", c)
	}
}

func TestEveryReasonConstantRegistered(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "reasons.go", nil, 0)
	require.NoError(t, err)

	consts := map[string]bool{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "ReasonCode" {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				require.True(t, ok, "ReasonCode constants must be string literals")
				consts[strings.Trim(lit.Value, `"`)] = true
			}
		}
	}
	for c := range consts {
		_, ok := reasonRegistry[ReasonCode(c)]
		assert.True(t, ok, "constant %q is not in reasonRegistry", c)
	}
	for code := range reasonRegistry {
		assert.True(t, consts[string(code)], "registry key %q has no ReasonCode constant", code)
	}
}

func TestWithReasonRendersEnglish(t *testing.T) {
	err := ErrInvalidInput("n", "x").WithReason(ReasonCommonTestOnly, map[string]any{"n": 3})
	assert.Equal(t, "Test 3", err.Message)
	assert.Equal(t, "Test 3", err.Details["reason"])
	assert.Equal(t, ReasonCommonTestOnly, ReasonCode(err.ReasonCode))
	assert.Equal(t, "common_test_only", err.ReasonCode)
	assert.Equal(t, map[string]any{"n": 3}, err.ReasonParams)
}

func TestWithReasonParamMismatchFailsSafe(t *testing.T) {
	for name, params := range map[string]map[string]any{
		"wrong key":   {"m": 3},
		"extra key":   {"n": 3, "m": 4},
		"missing key": {},
		"nil":         nil,
	} {
		t.Run(name, func(t *testing.T) {
			orig := ErrInvalidInput("n", "x")
			msg := orig.Message
			err := orig.WithReason(ReasonCommonTestOnly, params)
			assert.Empty(t, err.ReasonCode)
			assert.Nil(t, err.ReasonParams)
			assert.Equal(t, msg, err.Message)
			assert.Equal(t, "x", err.Details["reason"])
		})
	}
}

func TestWithReasonUnknownCodeFailsSafe(t *testing.T) {
	err := ErrInvalidInput("n", "x").WithReason("common_nope", nil)
	assert.Empty(t, err.ReasonCode)
	assert.Equal(t, "Invalid value for field 'n'", err.Message)
}

func TestReasonCodesSortedAndSpecFor(t *testing.T) {
	codes := ReasonCodes()
	require.NotEmpty(t, codes)
	assert.True(t, sort.SliceIsSorted(codes, func(i, j int) bool { return codes[i] < codes[j] }))
	tpl, params, ok := ReasonSpecFor(ReasonCommonTestOnly)
	assert.True(t, ok)
	assert.Equal(t, "Test {n}", tpl)
	assert.Equal(t, []string{"n"}, params)
	_, _, ok = ReasonSpecFor("nope_nope")
	assert.False(t, ok)
}
