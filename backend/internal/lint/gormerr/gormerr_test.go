package gormerr_test

import (
	"testing"

	"mycorrhizal/internal/lint/gormerr"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer runs the pass over testdata/src/a, whose `// want` comments
// are the positive cases; every unannotated line (negatives, a_test.go) must
// produce no diagnostic. analysistest.Run is itself the assertion — it fails
// unless the analyzer's diagnostics match those `// want` comments.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), gormerr.Analyzer, "a")
}
