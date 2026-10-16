package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const reportXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="test">
  <package name="com/kit">
    <sourcefile name="Foo.kt">
      <line nr="1" mi="0" ci="2"/>
      <line nr="2" mi="1" ci="0"/>
    </sourcefile>
    <sourcefile name="Dup.kt">
      <line nr="1" mi="0" ci="1"/>
    </sourcefile>
  </package>
</report>`

// repo lays out a fake repository root holding the report and the sources.
func repo(t *testing.T, xml string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, defaultReportRel), xml)
	writeFile(t, filepath.Join(root, "android/core/src/main/kotlin/com/kit/Foo.kt"), "")
	writeFile(t, filepath.Join(root, "android/a/src/main/kotlin/com/kit/Dup.kt"), "")
	writeFile(t, filepath.Join(root, "android/b/src/main/kotlin/com/kit/Dup.kt"), "")
	return root
}

func TestRunWritesLcovAndSummary(t *testing.T) {
	root := repo(t, reportXML)
	summary := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-root", root, "-summary", summary}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	lcov, err := os.ReadFile(filepath.Join(root, defaultOutRel))
	if err != nil {
		t.Fatal(err)
	}
	want := "SF:android/core/src/main/kotlin/com/kit/Foo.kt\nDA:1,1\nDA:2,0\nLF:2\nLH:1\nend_of_record\n"
	if string(lcov) != want {
		t.Errorf("lcov = %q, want %q", lcov, want)
	}
	out := stdout.String()
	if !strings.Contains(out, "::warning::jacoco2lcov: com/kit/Dup.kt matches more than one file") {
		t.Errorf("ambiguous file not annotated:\n%s", out)
	}
	if !strings.Contains(out, "wrote ") || !strings.Contains(out, "(1 files)") {
		t.Errorf("missing write confirmation:\n%s", out)
	}
	md, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(md), "### Android coverage: JaCoCo → lcov") || !strings.Contains(string(md), "| lcov output | 1 |") {
		t.Errorf("summary file:\n%s", md)
	}
}

func TestRunExplicitPaths(t *testing.T) {
	root := repo(t, reportXML)
	in := writeFile(t, filepath.Join(t.TempDir(), "r.xml"), reportXML)
	out := filepath.Join(t.TempDir(), "out.lcov")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-root", root, "-in", in, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("explicit -out not written: %v", err)
	}
}

func TestRunRefusesEmptyOutput(t *testing.T) {
	root := repo(t, `<report name="t"><package name="com/gen"><sourcefile name="X_Factory.java"><line nr="1" mi="0" ci="1"/></sourcefile></package></report>`)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-root", root}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing to write an empty lcov report") {
		t.Errorf("stderr: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, defaultOutRel)); !os.IsNotExist(err) {
		t.Errorf("an empty lcov file must not be written (stat err = %v)", err)
	}
}

func TestRunErrors(t *testing.T) {
	root := repo(t, reportXML)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bad flag", []string{"-nope"}, "flag provided but not defined"},
		{"missing report", []string{"-root", root, "-in", filepath.Join(root, "missing.xml")}, "missing.xml"},
		{"malformed report", []string{"-root", root, "-in", writeFile(t, filepath.Join(t.TempDir(), "bad.xml"), "<report><package")}, "parsing JaCoCo XML"},
		{"no android dir", []string{"-root", t.TempDir(), "-in", filepath.Join(root, defaultReportRel)}, "indexing android"},
		{"unwritable summary", []string{"-root", root, "-summary", filepath.Join(root, "no-such-dir", "s.md")}, "no-such-dir"},
		{"unwritable out", []string{"-root", root, "-out", filepath.Join(root, "no-such-dir", "o.lcov")}, "no-such-dir"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr %q does not mention %q", stderr.String(), tc.want)
			}
		})
	}
}

func TestRunFindsRootFromWorkingDirectory(t *testing.T) {
	root := repo(t, reportXML)
	writeFile(t, filepath.Join(root, "backend/go.mod"), "module x\n")
	t.Chdir(filepath.Join(root, "backend"))
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
}
