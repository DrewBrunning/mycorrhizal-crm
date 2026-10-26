package agenteval

import (
	"strings"
	"testing"
)

func TestDetectIntegrityViolations(t *testing.T) {
	patch := strings.Join([]string{
		"diff --git a/backend/a_test.go b/backend/a_test.go",
		"--- a/backend/a_test.go",
		"+++ b/backend/a_test.go",
		"@@ -1,2 +1,4 @@",
		"-func TestOld(t *testing.T) {}",
		"+func TestX(t *testing.T) { t.Skip(\"flaky\") }",
		"+//nolint:gosec // silence the scanner",
		"+// # pragma: no cover -- not reachable in this test",
		"diff --git a/frontend/src/x.test.ts b/frontend/src/x.test.ts",
		"--- a/frontend/src/x.test.ts",
		"+++ b/frontend/src/x.test.ts",
		"@@ -1,1 +1,2 @@",
		"+it.skip(\"x\", () => {})",
		"+// @ts-expect-error",
	}, "\n")

	violations, warnings := DetectIntegrityViolations(patch, []string{"backend/legacy_test.go"}, []RenamedFile{{Old: "backend/old_test.go", New: "backend/old.go"}})
	joinedV := strings.Join(violations, "\n")
	joinedW := strings.Join(warnings, "\n")
	for _, want := range []string{
		"removed test declaration: backend/a_test.go",
		"added test skip: backend/a_test.go",
		"added test skip: frontend/src/x.test.ts",
		"deleted test file: backend/legacy_test.go",
		"test file renamed away: backend/old_test.go -> backend/old.go",
	} {
		if !strings.Contains(joinedV, want) {
			t.Errorf("violations missing %q:\n%s", want, joinedV)
		}
	}
	for _, want := range []string{
		"added lint suppression: backend/a_test.go",
		"added coverage suppression: backend/a_test.go",
		"added lint suppression: frontend/src/x.test.ts",
	} {
		if !strings.Contains(joinedW, want) {
			t.Errorf("warnings missing %q:\n%s", want, joinedW)
		}
	}
}

func TestDetectIntegrityIgnoresNonTestsCommentsAndSupportedSkips(t *testing.T) {
	patch := strings.Join([]string{
		"--- a/backend/app.go",
		"+++ b/backend/app.go",
		"@@ -1,1 +1,2 @@",
		"+t.Skip(\"not a test file\")",
		"--- a/backend/a_test.go",
		"+++ b/backend/a_test.go",
		"@@ -1,1 +1,4 @@",
		"+// t.Skip is documented here",
		"+citest.SkipOrRequire(t, \"go\")",
		"+func TestNew(t *testing.T) {}",
	}, "\n")
	violations, warnings := DetectIntegrityViolations(patch, nil, nil)
	if len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDetectIntegrityFlagsBuildConstraintsAsWarnings(t *testing.T) {
	patch := strings.Join([]string{
		"--- a/backend/a_test.go",
		"+++ b/backend/a_test.go",
		"@@ -1,0 +1,1 @@",
		"+//go:build ignore",
	}, "\n")
	violations, warnings := DetectIntegrityViolations(patch, nil, nil)
	if len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "build constraint") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDetectIntegrityViolationsDeduplicates(t *testing.T) {
	patch := strings.Join([]string{
		"--- a/backend/a_test.go",
		"+++ b/backend/a_test.go",
		"@@ -1,1 +1,3 @@",
		"+t.Skip(\"x\")",
		"+t.Skip(\"y\")",
	}, "\n")
	violations, _ := DetectIntegrityViolations(patch, []string{"backend/a_test.go"}, nil)
	count := 0
	for _, finding := range violations {
		if finding == "added test skip: backend/a_test.go" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate findings = %v", violations)
	}
}

func TestDetectIntegrityDeletedTestInPatch(t *testing.T) {
	tests := []struct {
		name          string
		patch         string
		wantViolation string
	}{
		{
			name: "deleted test declaration through dev null",
			patch: strings.Join([]string{
				"--- a/backend/a_test.go",
				"+++ /dev/null",
				"@@ -1,1 +0,0 @@",
				"-func TestGone(t *testing.T) {}",
			}, "\n"),
			wantViolation: "removed test declaration: backend/a_test.go",
		},
		{
			name: "removed comment in a test file is ignored",
			patch: strings.Join([]string{
				"--- a/backend/a_test.go",
				"+++ b/backend/a_test.go",
				"@@ -1,1 +1,1 @@",
				"-// func TestGone(t *testing.T) {}",
				"+// still documented",
			}, "\n"),
		},
		{
			name: "removed declaration outside a test file is ignored",
			patch: strings.Join([]string{
				"--- a/backend/app.go",
				"+++ b/backend/app.go",
				"@@ -1,1 +1,1 @@",
				"-func TestGone(t *testing.T) {}",
				"+var x = 1",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, warnings := DetectIntegrityViolations(tt.patch, nil, nil)
			if tt.wantViolation == "" {
				if len(violations) != 0 {
					t.Fatalf("violations = %v, want none", violations)
				}
			} else if !contains(violations, tt.wantViolation) {
				t.Fatalf("violations = %v, want %q", violations, tt.wantViolation)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
		})
	}
}

func TestDetectIntegrityDeletedListSkipsBlankPaths(t *testing.T) {
	violations, warnings := DetectIntegrityViolations("", []string{"", "  ", "backend/a_test.go"}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(violations) != 1 || violations[0] != "deleted test file: backend/a_test.go" {
		t.Fatalf("violations = %v", violations)
	}
}

func TestDetectIntegrityDetectsPatterns(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  string
	}{
		{"go skip", addedLinePatch("backend/a_test.go", `func TestA(t *testing.T) { t.Skip("x") }`), "added test skip: backend/a_test.go"},
		{"go benchmark skip", addedLinePatch("backend/a_test.go", `func BenchmarkA(b *testing.B) { b.SkipNow() }`), "added test skip: backend/a_test.go"},
		{"js describe skip", addedLinePatch("frontend/src/a.test.ts", `describe.skip("x", () => {})`), "added test skip: frontend/src/a.test.ts"},
		{"js xit", addedLinePatch("frontend/src/a.test.ts", `xit("x", () => {})`), "added test skip: frontend/src/a.test.ts"},
		{"pytest mark skip", addedLinePatch("backend/a_test.py", `@pytest.mark.skip(reason="x")`), "added test skip: backend/a_test.py"},
		{"junit ignore", addedLinePatch("backend/A_Test.kt", `@Ignore`), "added test skip: backend/A_Test.kt"},
		{"junit disabled", addedLinePatch("backend/A_Test.kt", `@Disabled`), "added test skip: backend/A_Test.kt"},
		{"nolint", addedLinePatch("backend/a_test.go", `//nolint:gosec // reason`), "added lint suppression: backend/a_test.go"},
		{"nosec", addedLinePatch("backend/a_test.go", `// #nosec G101`), "added lint suppression: backend/a_test.go"},
		{"lint ignore", addedLinePatch("backend/a_test.go", `//lint:ignore SA1000 reason`), "added lint suppression: backend/a_test.go"},
		{"ts ignore", addedLinePatch("frontend/src/a.test.ts", `// @ts-ignore`), "added lint suppression: frontend/src/a.test.ts"},
		{"ts expect error", addedLinePatch("frontend/src/a.test.ts", `// @ts-expect-error`), "added lint suppression: frontend/src/a.test.ts"},
		{"eslint disable", addedLinePatch("frontend/src/a.test.ts", `/* eslint-disable */`), "added lint suppression: frontend/src/a.test.ts"},
		{"biome ignore", addedLinePatch("frontend/src/a.test.ts", `// biome-ignore lint`), "added lint suppression: frontend/src/a.test.ts"},
		{"prettier ignore", addedLinePatch("frontend/src/a.test.ts", `// prettier-ignore`), "added lint suppression: frontend/src/a.test.ts"},
		{"go coverage pragma", addedLinePatch("backend/a_test.go", `// # pragma: no cover -- covered test`), "added coverage suppression: backend/a_test.go"},
		{"istanbul ignore", addedLinePatch("frontend/src/a.test.ts", `/* istanbul ignore next */`), "added coverage suppression: frontend/src/a.test.ts"},
		{"stryker disable", addedLinePatch("frontend/src/a.test.ts", `// stryker disable all`), "added coverage suppression: frontend/src/a.test.ts"},
		{"coverage ignore", addedLinePatch("frontend/src/a.test.ts", `// coverage: ignore`), "added coverage suppression: frontend/src/a.test.ts"},
		{"removed go test", removedLinePatch("backend/a_test.go", `func TestGone(t *testing.T) {}`), "removed test declaration: backend/a_test.go"},
		{"removed junit test", removedLinePatch("backend/A_Test.kt", `@Test`), "removed test declaration: backend/A_Test.kt"},
		{"removed pytest", removedLinePatch("backend/a_test.py", `def test_gone():`), "removed test declaration: backend/a_test.py"},
		{"removed js describe", removedLinePatch("frontend/src/a.test.ts", `describe("x", () => {})`), "removed test declaration: frontend/src/a.test.ts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, warnings := DetectIntegrityViolations(tt.patch, nil, nil)
			findings := append(append([]string{}, violations...), warnings...)
			if !contains(findings, tt.want) {
				t.Fatalf("findings = %v, want %q", findings, tt.want)
			}
		})
	}
}

func addedLinePatch(file, line string) string {
	return strings.Join([]string{
		"--- a/" + file,
		"+++ b/" + file,
		"@@ -1,0 +1,1 @@",
		"+" + line,
	}, "\n")
}

func removedLinePatch(file, line string) string {
	return strings.Join([]string{
		"--- a/" + file,
		"+++ b/" + file,
		"@@ -1,1 +0,0 @@",
		"-" + line,
	}, "\n")
}
