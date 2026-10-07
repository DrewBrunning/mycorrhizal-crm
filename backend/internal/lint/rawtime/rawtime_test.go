package rawtime_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/lint/rawtime"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer runs the pass over testdata/src: services/a.go holds the
// positives (call, value reference, aliased import, a reason-less marker) and
// the negatives (inline-allowed, non-clock time functions); a_test.go and the
// out-of-scope package `other` must produce nothing.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), rawtime.Analyzer, "services", "other")
}

// TestAnalyzer_FileAllowlist: a file-level Allowlist entry tolerates exactly
// Count uses (in source order) and reports the rest. middleware/m.go has
// three uses; with Count 2 only the third carries a `want`.
func TestAnalyzer_FileAllowlist(t *testing.T) {
	rawtime.Allowlist["middleware/m.go"] = rawtime.Entry{Count: 2, Reason: "test fixture"}
	defer delete(rawtime.Allowlist, "middleware/m.go")
	analysistest.Run(t, analysistest.TestData(), rawtime.Analyzer, "middleware")
}

func TestKey(t *testing.T) {
	if got := rawtime.Key("/x/backend/services/foo.go"); got != "services/foo.go" {
		t.Errorf("Key = %q", got)
	}
}

// TestAllowlistIsExact re-scans the real controllers/services/middleware
// sources with the analyzer's own syntactic scan and requires every
// Allowlist entry to match its file's raw-use count exactly, carry a Reason,
// and name an existing file — so the list can only shrink and a migrated
// site cannot leave a stale budget behind. It also re-asserts that no
// un-allowlisted file carries a raw use, so plain `go test` catches a
// regression even without the lint step.
func TestAllowlistIsExact(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	counts := map[string]int{}
	for _, dir := range []string{"controllers", "services", "middleware"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(root, dir, name), nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			uses, bad := rawtime.Scan(fset, f)
			counts[dir+"/"+name] = len(uses) + len(bad)
		}
	}
	for key, entry := range rawtime.Allowlist {
		got, exists := counts[key]
		if !exists {
			t.Errorf("allowlist entry %q: no such file", key)
			continue
		}
		if strings.TrimSpace(entry.Reason) == "" {
			t.Errorf("allowlist entry %q: Reason is required", key)
		}
		if got != entry.Count {
			t.Errorf("allowlist entry %q: Count = %d but the file has %d raw time uses (lower or raise it to match)", key, entry.Count, got)
		}
	}
	for key, n := range counts {
		if n > 0 && rawtime.Allowlist[key].Count == 0 {
			t.Errorf("%s has %d raw time.Now/Since/Until use(s) with no `// rawtime:allow <reason>` marker or allowlist entry", key, n)
		}
	}
}

// TestScan_ShadowedAndNoImport: a file that does not import "time" (or
// imports it under `_`/`.`) has nothing to scan.
func TestScan_NoTimeImport(t *testing.T) {
	for _, src := range []string{
		"package p\nfunc f() {}\n",
		"package p\nimport _ \"time\"\nfunc f() {}\n",
		"package p\nimport . \"time\"\nfunc f() { _ = Now() }\n",
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if uses, bad := rawtime.Scan(fset, f); len(uses)+len(bad) != 0 {
			t.Errorf("expected nothing for %q, got %v %v", src, uses, bad)
		}
	}
}
