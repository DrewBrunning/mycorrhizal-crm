package pragmacheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckGoSource_BareMarkerIsAFinding(t *testing.T) {
	src := "package x\n\nfunc f() {\n\tif err != nil {\n\t\treturn err // # pragma: no cover\n\t}\n}\n"
	findings := CheckGoSource("x.go", src)
	require.Len(t, findings, 1)
	assert.Equal(t, 5, findings[0].Line)
}

func TestCheckGoSource_SameLineReasonPasses(t *testing.T) {
	src := "package x\n\nfunc f() {\n\tif err != nil {\n\t\treturn err // # pragma: no cover — os.Exit fails only on a closed fd\n\t}\n}\n"
	assert.Empty(t, CheckGoSource("x.go", src))
}

func TestCheckGoSource_PunctuationOnlyReasonIsBare(t *testing.T) {
	// A reason of only dashes/colons has no actual content once punctuation
	// is stripped, so it must not satisfy the rule.
	src := "package x\n\nfunc f() {\n\treturn err // # pragma: no cover — --\n}\n"
	findings := CheckGoSource("x.go", src)
	assert.Len(t, findings, 1)
}

func TestCheckGoSource_InheritedReasonFromGuardingIfPasses(t *testing.T) {
	// The common repo pattern: the reason sits on the `if` line, and every
	// bare marker inside the block inherits it.
	src := "package x\n\nfunc f() {\n\tif err != nil { // # pragma: no cover — defensive\n\t\tlog(err) // # pragma: no cover\n\t\treturn err // # pragma: no cover\n\t}\n}\n"
	assert.Empty(t, CheckGoSource("x.go", src))
}

func TestCheckGoSource_InheritedReasonFromCommentLineAbovePasses(t *testing.T) {
	src := "package x\n\nfunc f() {\n\t// a template build failure is an OS/invariant break\n\treturn err // # pragma: no cover\n}\n"
	assert.Empty(t, CheckGoSource("x.go", src))
}

func TestCheckGoSource_NoInheritanceAcrossBlankLine(t *testing.T) {
	// A reason two statements up (separated by an unrelated line) must not
	// leak onto an unrelated bare marker.
	src := "package x\n\nfunc f() {\n\t// unrelated setup, not a reason for the next line\n\tx := 1\n\treturn x // # pragma: no cover\n}\n"
	findings := CheckGoSource("x.go", src)
	assert.Len(t, findings, 1)
}

func TestCheckGoSource_ProseDiscussingTheConventionIsNotAMarker(t *testing.T) {
	// A doc comment that merely quotes the marker syntax while explaining
	// the convention (controllers/user_delete_cascade.go's real shape) must
	// not itself be treated as an applied, unreasoned marker.
	src := "package x\n\n// Every per-table `return err` below is deliberately `# pragma: no cover`\n// because of reasons explained at length elsewhere in this comment block.\nfunc f() {}\n"
	assert.Empty(t, CheckGoSource("x.go", src))
}

func TestCheckGoSource_MarkerOnFirstLineHasNoPrecedingComment(t *testing.T) {
	// precedingComment must not walk off the start of the file: a marker on
	// line 1 (or preceded only by blank lines back to the start) has no
	// earlier line to inherit a reason from.
	src := "return // # pragma: no cover\n"
	findings := CheckGoSource("x.go", src)
	assert.Len(t, findings, 1)
}

func TestCheckGoSource_MarkerAfterOnlyBlankLinesHasNoPrecedingComment(t *testing.T) {
	src := "\n\n\treturn // # pragma: no cover\n"
	findings := CheckGoSource("x.go", src)
	assert.Len(t, findings, 1)
}

func TestCheckGoSource_NoMarkersIsClean(t *testing.T) {
	assert.Empty(t, CheckGoSource("x.go", "package x\n\nfunc f() {}\n"))
}

func TestCheckTSSource_BareV8IgnoreIsAFinding(t *testing.T) {
	src := "export function f() {\n  /* v8 ignore next */\n  return 1;\n}\n"
	findings := CheckTSSource("x.ts", src)
	require.Len(t, findings, 1)
	assert.Equal(t, 2, findings[0].Line)
}

func TestCheckTSSource_ReasonedV8IgnorePasses(t *testing.T) {
	src := "export function f() {\n  /* v8 ignore next -- unreachable defensive branch */\n  return 1;\n}\n"
	assert.Empty(t, CheckTSSource("x.ts", src))
}

func TestCheckTSSource_InheritedReasonFromLineAbovePasses(t *testing.T) {
	src := "export function f() {\n  // structurally unreachable: the caller already validated this\n  /* v8 ignore next */\n  return 1;\n}\n"
	assert.Empty(t, CheckTSSource("x.ts", src))
}

func TestCheckTSSource_NoMarkersIsClean(t *testing.T) {
	assert.Empty(t, CheckTSSource("x.ts", "export function f() { return 1; }\n"))
}
