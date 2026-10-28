package reasoncode_test

import (
	"bytes"
	"strings"
	"testing"

	"mycorrhizal/internal/lint/reasoncode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), reasoncode.Analyzer, "a", "mycorrhizal/errors")
}

func TestParseAllowlist(t *testing.T) {
	got, err := reasoncode.ParseAllowlist(strings.NewReader("# c\n\ncontrollers/a.go:3\nb.go:1\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"controllers/a.go": 3, "b.go": 1}, got)

	for _, bad := range []string{"nocolon", "a.go:x", "a.go:0", "a.go:1\na.go:2"} {
		_, err := reasoncode.ParseAllowlist(strings.NewReader(bad))
		assert.Error(t, err, bad)
	}
}

func TestCompare(t *testing.T) {
	allow := map[string]int{"a.go": 2, "b.go": 3, "c.go": 1}
	t.Run("exact passes", func(t *testing.T) {
		assert.Empty(t, reasoncode.Compare(map[string]int{"a.go": 2, "b.go": 3, "c.go": 1}, allow))
	})
	t.Run("over count fails", func(t *testing.T) {
		p := reasoncode.Compare(map[string]int{"a.go": 3, "b.go": 3, "c.go": 1}, allow)
		require.Len(t, p, 1)
		assert.Contains(t, p[0], "a.go")
		assert.Contains(t, p[0], "above")
	})
	t.Run("file missing from list fails", func(t *testing.T) {
		p := reasoncode.Compare(map[string]int{"a.go": 2, "b.go": 3, "c.go": 1, "new.go": 1}, allow)
		require.Len(t, p, 1)
		assert.Contains(t, p[0], "new.go")
	})
	t.Run("under count says lower it", func(t *testing.T) {
		p := reasoncode.Compare(map[string]int{"a.go": 1, "b.go": 3, "c.go": 1}, allow)
		require.Len(t, p, 1)
		assert.Contains(t, p[0], "lower it")
	})
	t.Run("fully migrated file says lower it", func(t *testing.T) {
		p := reasoncode.Compare(map[string]int{"a.go": 2, "b.go": 3}, allow)
		require.Len(t, p, 1)
		assert.Contains(t, p[0], "c.go")
		assert.Contains(t, p[0], "lower it")
	})
}

func TestFormatAllowlistRoundTrips(t *testing.T) {
	counts := map[string]int{"z.go": 1, "a/b.go": 4}
	var buf bytes.Buffer
	require.NoError(t, reasoncode.WriteAllowlist(&buf, counts))
	got, err := reasoncode.ParseAllowlist(&buf)
	require.NoError(t, err)
	assert.Equal(t, counts, got)
}
