package patchcov

import (
	"strings"
	"testing"

	"mycorrhizal/internal/coverageratchet"
)

func TestParseUnifiedDiff(t *testing.T) {
	diff := `diff --git a/backend/services/foo.go b/backend/services/foo.go
index 111..222 100644
--- a/backend/services/foo.go
+++ b/backend/services/foo.go
@@ -10,0 +11,3 @@ func Foo() {
+	a := 1
+	b := 2
+	_ = a + b
@@ -20 +24 @@ func Bar() {
-	old
+	new
diff --git a/backend/deleted.go b/backend/deleted.go
deleted file mode 100644
--- a/backend/deleted.go
+++ /dev/null
@@ -1 +0,0 @@
-gone
diff --git a/frontend/src/App.tsx b/frontend/src/App.tsx
--- a/frontend/src/App.tsx
+++ b/frontend/src/App.tsx
@@ -1 +1 @@
-x
+y
`
	got, err := ParseUnifiedDiff(strings.NewReader(diff))
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if lines := got["backend/services/foo.go"]; len(lines) != 4 || lines[0] != 11 || lines[3] != 24 {
		t.Errorf("foo.go lines = %v, want [11 12 13 24]", lines)
	}
	if _, ok := got["backend/deleted.go"]; ok {
		t.Error("deleted file should not contribute new-side lines")
	}
	if lines := got["frontend/src/App.tsx"]; len(lines) != 1 || lines[0] != 1 {
		t.Errorf("App.tsx lines = %v, want [1]", lines)
	}
}

func TestParseUnifiedDiffRejectsMalformedHunk(t *testing.T) {
	_, err := ParseUnifiedDiff(strings.NewReader("+++ b/x.go\n@@ not a hunk @@\n"))
	if err == nil {
		t.Fatal("expected an error for a malformed hunk header")
	}
}

func TestChangedLineCoverage(t *testing.T) {
	blocks := []coverageratchet.Block{
		{File: "mycorrhizal/services/foo.go", StartLine: 10, EndLine: 12, NumStmt: 2, Count: 1},
		{File: "mycorrhizal/services/foo.go", StartLine: 20, EndLine: 20, NumStmt: 1, Count: 0},
		{File: "mycorrhizal/services/bar.go", StartLine: 5, EndLine: 5, NumStmt: 1, Count: 3},
	}
	changed := map[string][]int{
		"backend/services/foo.go": {11, 20, 30},
		"backend/services/bar.go": {5},
		"backend/services/baz.go": {1},
	}
	got := ChangedLineCoverage(blocks, changed, "mycorrhizal/", "backend")
	if len(got) != 2 {
		t.Fatalf("results = %v, want two files with executable lines", got)
	}
	if got[0].File != "backend/services/bar.go" || got[0].Covered != 1 || got[0].Executable != 1 {
		t.Errorf("bar result = %+v", got[0])
	}
	if got[1].File != "backend/services/foo.go" || got[1].Covered != 1 || got[1].Executable != 2 {
		t.Errorf("foo result = %+v", got[1])
	}
	if pct := Percent(got); pct < 66 || pct > 67 {
		t.Errorf("Percent = %v, want ~66.7", pct)
	}
	if exec, cov := Totals(got); exec != 3 || cov != 2 {
		t.Errorf("Totals = (%d, %d), want (3, 2)", exec, cov)
	}
}
