package carddav

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ADR 0041 test-plan item 10: every contacts query in this package must go
// through the scope helper, so a new DAV path cannot silently skip the filter.
// The check is deliberately source-level: a behavioural test can only cover
// the paths it knows about, while this fails when a future function adds a
// contact query of its own.
func TestCardDAVContactQueriesUseScopeHelper(t *testing.T) {
	const path = "backend.go"
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Body.Pos()).Offset
		end := fset.Position(fn.Body.End()).Offset
		body := string(src[start:end])
		if !strings.Contains(body, "models.Contact") || !strings.Contains(body, "db.") {
			continue
		}
		require.Truef(t,
			strings.Contains(body, "ApplyCardDAVSyncScope") || strings.Contains(body, "ContactInCardDAVSyncScope"),
			"%s queries contacts without the CardDAV sync-scope helper; every read/write path must apply the scope (ADR 0041 decision 4/5)", fn.Name.Name)
	}
}
