package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const skipXML = `<testsuite><testcase name="m" classname="p.Q"><skipped/></testcase><testcase name="ok" classname="p.Q"/></testsuite>`

// fixture builds a fake repo root with an allowlist, a Kotlin source and a
// results dir.
func fixture(t *testing.T, allowlist, xml string) (root, results string) {
	t.Helper()
	root = t.TempDir()
	must := func(p, body string) {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must("android/e2e-expected-skips.txt", allowlist)
	must("android/app/src/androidTest/kotlin/p/Q.kt", "class Q { fun m() {} }")
	must("results/nested/TEST-a.xml", xml)
	must("results/ignored.txt", "not xml")
	return root, filepath.Join(root, "results")
}

func runCmd(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestResultsModeMatchesAllowlist(t *testing.T) {
	root, results := fixture(t, "p.Q#m | arm64\n", skipXML)
	summary := filepath.Join(t.TempDir(), "summary.md")
	code, out, _ := runCmd("-root", root, "-results", results, "-summary", summary)
	if code != 0 || !strings.Contains(out, "androidskipcheck OK: 2 test(s), 1 skipped") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	b, err := os.ReadFile(summary)
	if err != nil || !strings.Contains(string(b), "Skips match the allowlist exactly") {
		t.Fatalf("summary = %q err=%v", b, err)
	}
}

func TestResultsModeFailsOnUnexpectedSkip(t *testing.T) {
	root, results := fixture(t, "# nothing expected\n", skipXML)
	code, out, _ := runCmd("-root", root, "-results", results)
	if code != 1 || !strings.Contains(out, "UNEXPECTED SKIP: p.Q#m") || !strings.Contains(out, "1 android-skip-gate finding") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestResultsModeFailsWhenSkipDisappears(t *testing.T) {
	passing := `<testsuite><testcase name="m" classname="p.Q"/></testsuite>`
	root, results := fixture(t, "p.Q#m | arm64\n", passing)
	code, out, _ := runCmd("-root", root, "-results", results)
	if code != 1 || !strings.Contains(out, "UNEXPECTED RUN: p.Q#m") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestResultsModeMalformedAllowlistIsAFinding(t *testing.T) {
	root, results := fixture(t, "p.Q#m\n", skipXML)
	code, out, _ := runCmd("-root", root, "-results", results)
	if code != 1 || !strings.Contains(out, "has no `| reason`") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestResultsModeErrorsAreExit2(t *testing.T) {
	root, results := fixture(t, "p.Q#m | r\n", skipXML)
	if err := os.WriteFile(filepath.Join(results, "bad.xml"), []byte("<html/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errb := runCmd("-root", root, "-results", results); code != 2 || !strings.Contains(errb, "not JUnit XML") {
		t.Fatalf("bad xml: code=%d err=%s", code, errb)
	}
	if code, _, errb := runCmd("-root", root, "-results", filepath.Join(root, "nope")); code != 2 || errb == "" {
		t.Fatalf("missing dir: code=%d err=%s", code, errb)
	}
	// summary path whose parent does not exist cannot be opened.
	_, good := fixture(t, "p.Q#m | r\n", skipXML)
	if code, _, errb := runCmd("-root", root, "-results", good, "-summary", filepath.Join(root, "no", "dir", "s.md")); code != 2 || errb == "" {
		t.Fatalf("bad summary: code=%d err=%s", code, errb)
	}
}

func TestEmptyResultsDirFails(t *testing.T) {
	root, _ := fixture(t, "p.Q#m | r\n", skipXML)
	empty := t.TempDir()
	code, out, _ := runCmd("-root", root, "-results", empty)
	if code != 1 || !strings.Contains(out, "executed nothing") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestMissingAllowlistIsExit2(t *testing.T) {
	root := t.TempDir()
	if code, _, errb := runCmd("-root", root, "-static"); code != 2 || errb == "" {
		t.Fatalf("code=%d err=%s", code, errb)
	}
}

func TestStaticMode(t *testing.T) {
	root, _ := fixture(t, "p.Q#m | r\n", skipXML)
	if code, out, _ := runCmd("-root", root, "-static"); code != 0 || !strings.Contains(out, "-static OK: 1 expected") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	root2, _ := fixture(t, "p.Q#renamed | r\n", skipXML)
	if code, out, _ := runCmd("-root", root2, "-static"); code != 1 || !strings.Contains(out, "declares no `fun renamed(`") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestFlagValidation(t *testing.T) {
	for _, args := range [][]string{{}, {"-static", "-results", "x"}, {"-bogus"}} {
		if code, _, _ := runCmd(args...); code != 2 {
			t.Errorf("args %v: code=%d, want 2", args, code)
		}
	}
}

// The real committed allowlist must be internally valid and resolve to real
// tests -- this is the same check the docs-citations CI step runs, kept here
// so `go test ./...` also fails on a broken allowlist.
func TestRealAllowlistResolves(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if code, out, errb := runCmd("-root", root, "-static"); code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out, errb)
	}
}

func TestFindRepoRootFailsOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := findRepoRoot(); err == nil {
		t.Fatal("want error outside a repo")
	}
}

func TestDefaultRootDiscovery(t *testing.T) {
	// run() without -root finds the repo from the working directory (the
	// package dir under backend/).
	if code, out, errb := runCmd("-static"); code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out, errb)
	}
}
